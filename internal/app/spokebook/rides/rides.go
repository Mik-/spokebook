// Package rides imports rides and assigns them to bikes. Rides add to the
// computed odometer of their bike (see package odometer); after a change
// the installations that follow the odometer are refreshed.
package rides

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/odometer"
	"github.com/spokebook/spokebook/internal/app/spokebook/rides/parse"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// ErrBeforeService means the ride is dated before the bike went into
// service.
var ErrBeforeService = errors.New("rides: before the bike went into service")

// Status says whether a ride read from a file is new.
type Status int

const (
	New Status = iota
	// Duplicate overlaps in time with an existing ride or an earlier one
	// of the same import, e.g. the same ride from another device.
	Duplicate
	// Known was imported before (same source and external ID).
	Known
)

// String returns the status as ID for messages.
func (s Status) String() string {
	return [...]string{"new", "duplicate", "known"}[s]
}

// Candidate is a ride read from a file, checked against the stored rides.
type Candidate struct {
	parse.Ride
	Status Status
	// Existing is the known or overlapping ride; empty ID for an
	// overlap within the import.
	Existing store.Ride
}

// Gear is a bike of a source found in an import, with its mapping if
// there is one.
type Gear struct {
	Source  string
	Key     string
	Name    string
	Mapping *store.RideGear
}

// Preview checks the rides read from files: whether they are known or
// duplicates, and which bikes of the sources they name.
func Preview(ctx context.Context, st store.Stores, ownerID string, read []parse.Ride) ([]Candidate, []Gear, error) {
	var cands []Candidate
	var gears []Gear
	for _, r := range read {
		c := Candidate{Ride: r}
		known, err := st.Rides().FindRide(ctx, ownerID, r.Source, r.ExternalID)
		switch {
		case err == nil:
			c.Status, c.Existing = Known, known
		case !errors.Is(err, store.ErrNotFound):
			return nil, nil, err
		default:
			over, err := st.Rides().OverlappingRides(ctx, ownerID, r.StartedAt, r.EndedAt)
			if err != nil {
				return nil, nil, err
			}
			if len(over) > 0 {
				c.Status, c.Existing = Duplicate, over[0]
			} else if slices.ContainsFunc(cands, func(o Candidate) bool { return o.Status == New && overlaps(o.Ride, r) }) {
				c.Status = Duplicate
			}
		}
		cands = append(cands, c)
		if r.GearKey == "" || c.Status == Known || slices.ContainsFunc(gears, func(g Gear) bool {
			return g.Source == r.Source && g.Key == r.GearKey
		}) {
			continue
		}
		g := Gear{Source: r.Source, Key: r.GearKey, Name: r.GearName}
		m, err := st.Rides().FindRideGear(ctx, ownerID, r.Source, r.GearKey)
		switch {
		case err == nil:
			g.Mapping = &m
		case !errors.Is(err, store.ErrNotFound):
			return nil, nil, err
		}
		gears = append(gears, g)
	}
	return cands, gears, nil
}

// overlaps reports whether two rides overlap in time; rides without
// duration overlap if they start at the same time.
func overlaps(a, b parse.Ride) bool {
	if a.StartedAt.Equal(b.StartedAt) {
		return true
	}
	return a.StartedAt.Before(b.EndedAt) && b.StartedAt.Before(a.EndedAt)
}

// Item is a ride to import. With Auto, the bike comes from its gear (see
// GearChoice) or the default bike; otherwise BikeID was chosen (empty:
// none).
type Item struct {
	parse.Ride
	Auto   bool
	BikeID string
}

// GearChoice is the bike chosen for a source's bike in an import (empty:
// none). With Remember it is saved as mapping for later imports.
type GearChoice struct {
	Source   string
	Key      string
	Name     string
	BikeID   string
	Remember bool
}

// Result counts what an import did.
type Result struct {
	Imported   int
	Skipped    int
	Unassigned int
}

// Import saves the items. Rides already imported are skipped; a ride
// dated before its bike went into service is not assigned.
func Import(ctx context.Context, st store.Store, ownerID string, items []Item, gears []GearChoice, defaultBikeID string) (Result, error) {
	var res Result
	err := st.WithTx(ctx, func(s store.Stores) error {
		res = Result{}
		bikes, err := bikesByID(ctx, s, ownerID)
		if err != nil {
			return err
		}
		if defaultBikeID != "" && bikes[defaultBikeID].ID == "" {
			return store.ErrNotFound
		}
		for _, g := range gears {
			if g.Remember {
				if err := remember(ctx, s, ownerID, g, bikes); err != nil {
					return err
				}
			}
		}
		touched := map[string]bool{}
		now := time.Now().UTC()
		for _, it := range items {
			r := store.Ride{
				ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID, Source: it.Source, ExternalID: it.ExternalID,
				GearKey: it.GearKey, StartedAt: it.StartedAt, EndedAt: it.EndedAt, RiddenOn: it.RiddenOn,
				DistanceM: it.DistanceM, Name: it.Name, CreatedAt: now,
			}
			r.BikeID = it.BikeID
			if it.Auto {
				r.BikeID = defaultBikeID
				if i := slices.IndexFunc(gears, func(g GearChoice) bool {
					return g.Source == it.Source && g.Key == it.GearKey && it.GearKey != ""
				}); i >= 0 && gears[i].BikeID != "" {
					r.BikeID, r.BikeByGear = gears[i].BikeID, true
				}
			}
			if b, ok := bikes[r.BikeID]; r.BikeID != "" && (!ok || beforeService(b, r.RiddenOn)) {
				r.BikeID, r.BikeByGear = "", false
			}
			_, err := s.Rides().CreateRide(ctx, r)
			if errors.Is(err, store.ErrConflict) {
				res.Skipped++
				continue
			}
			if err != nil {
				return err
			}
			res.Imported++
			if r.BikeID == "" {
				res.Unassigned++
			}
			touched[r.BikeID] = true
		}
		return refresh(ctx, s, ownerID, touched)
	})
	return res, err
}

// ImportAuto saves rides without a preview, as a source fetched them: known
// rides, overlaps and rides in doubt (indoor, unclear sport) are skipped.
// The bike comes from the mapping; an unknown bike of the source gets a
// mapping without bike, so its rides stay unassigned until it is mapped.
func ImportAuto(ctx context.Context, st store.Store, ownerID string, read []parse.Ride) (Result, error) {
	cands, gears, err := Preview(ctx, st, ownerID, read)
	if err != nil {
		return Result{}, err
	}
	var items []Item
	skipped := 0
	for _, c := range cands {
		if c.Status != New || c.Doubt != "" {
			skipped++
			continue
		}
		items = append(items, Item{Ride: c.Ride, Auto: true})
	}
	choices := make([]GearChoice, len(gears))
	for i, g := range gears {
		choices[i] = GearChoice{Source: g.Source, Key: g.Key, Name: g.Name, Remember: g.Mapping == nil}
		if g.Mapping != nil {
			choices[i].BikeID = g.Mapping.BikeID
		}
	}
	res, err := Import(ctx, st, ownerID, items, choices, "")
	res.Skipped += skipped
	return res, err
}

// remember saves the bike of a source's bike as mapping.
func remember(ctx context.Context, s store.Stores, ownerID string, g GearChoice, bikes map[string]store.Bike) error {
	if g.BikeID != "" && bikes[g.BikeID].ID == "" {
		return store.ErrNotFound
	}
	m, err := s.Rides().FindRideGear(ctx, ownerID, g.Source, g.Key)
	switch {
	case err == nil:
		if m.BikeID == g.BikeID {
			return nil
		}
		return s.Rides().SetRideGearBike(ctx, ownerID, m.ID, g.BikeID)
	case errors.Is(err, store.ErrNotFound):
		_, err = s.Rides().CreateRideGear(ctx, store.RideGear{
			ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID, Source: g.Source, GearKey: g.Key,
			Name: g.Name, BikeID: g.BikeID, CreatedAt: time.Now().UTC(),
		})
	}
	return err
}

// SetBike assigns the ride to a bike by hand (empty: none).
func SetBike(ctx context.Context, st store.Store, ownerID, rideID, bikeID string) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		r, err := s.Rides().GetRide(ctx, ownerID, rideID)
		if err != nil {
			return err
		}
		if bikeID != "" {
			b, err := s.Bikes().GetBike(ctx, ownerID, bikeID)
			if err != nil {
				return err
			}
			if beforeService(b, r.RiddenOn) {
				return ErrBeforeService
			}
		}
		if err := s.Rides().SetRideBike(ctx, ownerID, r.ID, bikeID, false); err != nil {
			return err
		}
		return refresh(ctx, s, ownerID, map[string]bool{r.BikeID: true, bikeID: true})
	})
}

// SetDiscarded discards a ride, so it no longer counts, or restores it.
func SetDiscarded(ctx context.Context, st store.Store, ownerID, rideID string, discarded bool) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		r, err := s.Rides().GetRide(ctx, ownerID, rideID)
		if err != nil {
			return err
		}
		if err := s.Rides().SetRideDiscarded(ctx, ownerID, r.ID, discarded); err != nil {
			return err
		}
		return refresh(ctx, s, ownerID, map[string]bool{r.BikeID: true})
	})
}

// Delete deletes a ride. Unlike a discarded one, a new import adds it
// again.
func Delete(ctx context.Context, st store.Store, ownerID, rideID string) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		r, err := s.Rides().GetRide(ctx, ownerID, rideID)
		if err != nil {
			return err
		}
		if err := s.Rides().DeleteRide(ctx, ownerID, r.ID); err != nil {
			return err
		}
		return refresh(ctx, s, ownerID, map[string]bool{r.BikeID: true})
	})
}

// DeleteAll deletes the rides in one transaction, see Delete.
func DeleteAll(ctx context.Context, st store.Store, ownerID string, ids []string) error {
	return each(ctx, st, ownerID, ids, func(s store.Stores, r store.Ride) error {
		return s.Rides().DeleteRide(ctx, ownerID, r.ID)
	})
}

// DiscardAll discards the rides in one transaction, see SetDiscarded.
func DiscardAll(ctx context.Context, st store.Store, ownerID string, ids []string) error {
	return each(ctx, st, ownerID, ids, func(s store.Stores, r store.Ride) error {
		return s.Rides().SetRideDiscarded(ctx, ownerID, r.ID, true)
	})
}

// each runs fn for every ride in one transaction and then refreshes the
// touched bikes once.
func each(ctx context.Context, st store.Store, ownerID string, ids []string, fn func(store.Stores, store.Ride) error) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		touched := map[string]bool{}
		for _, id := range ids {
			r, err := s.Rides().GetRide(ctx, ownerID, id)
			if err != nil {
				return err
			}
			if err := fn(s, r); err != nil {
				return err
			}
			touched[r.BikeID] = true
		}
		return refresh(ctx, s, ownerID, touched)
	})
}

// SetGearBike changes the bike of a mapping (empty: none). With existing,
// the rides of that source's bike move along, except those assigned by
// hand and those dated before the bike went into service.
func SetGearBike(ctx context.Context, st store.Store, ownerID, gearID, bikeID string, existing bool) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		g, err := s.Rides().GetRideGear(ctx, ownerID, gearID)
		if err != nil {
			return err
		}
		var bike store.Bike
		if bikeID != "" {
			if bike, err = s.Bikes().GetBike(ctx, ownerID, bikeID); err != nil {
				return err
			}
		}
		if err := s.Rides().SetRideGearBike(ctx, ownerID, g.ID, bikeID); err != nil {
			return err
		}
		if !existing {
			return nil
		}
		all, err := s.Rides().ListRides(ctx, ownerID)
		if err != nil {
			return err
		}
		touched := map[string]bool{bikeID: true}
		for _, r := range all {
			if r.Source != g.Source || r.GearKey != g.GearKey || (!r.BikeByGear && r.BikeID != "") || r.BikeID == bikeID {
				continue
			}
			if bikeID != "" && beforeService(bike, r.RiddenOn) {
				continue
			}
			if err := s.Rides().SetRideBike(ctx, ownerID, r.ID, bikeID, bikeID != ""); err != nil {
				return err
			}
			touched[r.BikeID] = true
		}
		return refresh(ctx, s, ownerID, touched)
	})
}

func beforeService(b store.Bike, day time.Time) bool {
	return b.InServiceAt != nil && day.Before(*b.InServiceAt)
}

func bikesByID(ctx context.Context, s store.Stores, ownerID string) (map[string]store.Bike, error) {
	list, err := s.Bikes().ListBikes(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]store.Bike, len(list))
	for _, b := range list {
		out[b.ID] = b
	}
	return out, nil
}

// refresh updates the installations of the touched bikes ("" is none).
func refresh(ctx context.Context, s store.Stores, ownerID string, touched map[string]bool) error {
	for id := range touched {
		if id == "" {
			continue
		}
		if err := odometer.RefreshTx(ctx, s, ownerID, id); err != nil {
			return err
		}
	}
	return nil
}
