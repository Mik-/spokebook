// Package components handles installing and removing components and
// computes their mileage and wear.
package components

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/odometer"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// BuiltinTypes are the component types every owner starts with. Unless
// renamed, they are translated via the message ID "component.type.<id>".
var BuiltinTypes = []string{
	"chain", "cassette", "chainring", "crankset", "derailleur", "front_derailleur", "shift_brake_lever",
	"shift_lever", "brake_lever",
	"brake_pads", "brake_rotor", "brake_caliper", "pedal",
	"tire", "tube", "wheel", "hub", "rim", "spokes", "axle", "frame", "fork", "shock", "derailleur_hanger",
	"headset", "handlebar", "stem", "grips", "bottom_bracket", "seatpost", "seatpost_clamp", "saddle",
	"suspension_seatpost",
	"handlebar_tape", "cable", "light", "fender", "rack", "kickstand",
	"battery", "motor", "display", "remote", "charger", "speed_sensor",
	"hub_gear", "belt", "belt_sprocket", "shift_battery", "other",
}

// Positions are the places on a bike an installation can have, besides "".
var Positions = []string{"front", "rear", "left", "right"}

var (
	// ErrTypeInUse means a component type can't be deleted because
	// components use it.
	ErrTypeInUse = errors.New("components: type in use")
	// ErrInvalidType means the type doesn't exist or is hidden.
	ErrInvalidType = errors.New("components: invalid type")
	// ErrInvalidPosition means the position is not one of Positions.
	ErrInvalidPosition = errors.New("components: invalid position")
	// ErrBeforeInstallation means a removal is dated or measured before
	// the installation.
	ErrBeforeInstallation = errors.New("components: removal before installation")
	// ErrAlreadyInstalled means the component is currently installed.
	ErrAlreadyInstalled = errors.New("components: already installed")
	// ErrNoOdometer means no distance was given and the bike has neither a
	// reading at or before the date nor an odometer at going into service.
	ErrNoOdometer = errors.New("components: bike has no odometer reading")
	// ErrOverlap means an installation would start before the component
	// was last removed.
	ErrOverlap = errors.New("components: overlaps a previous installation")
)

// DuplicateError means a component of the same type is already installed
// at the same position of the bike.
type DuplicateError struct{ ComponentID string }

func (e *DuplicateError) Error() string {
	return "components: already a component of this type at this position: " + e.ComponentID
}

// Install mounts a component on a bike. If distanceM is nil, the bike's
// odometer at the installation date is used and followed when it changes
// later; otherwise it is recorded as a
// reading if it differs. Unless allowDuplicate, it fails with a
// DuplicateError if a component of the same type is installed at the same
// position.
func Install(ctx context.Context, st store.Store, i store.Installation, distanceM *int64, allowDuplicate bool) (store.Installation, error) {
	if i.Position != "" && !slices.Contains(Positions, i.Position) {
		return store.Installation{}, ErrInvalidPosition
	}
	var added store.Installation
	err := st.WithTx(ctx, func(s store.Stores) error {
		var err error
		added, err = installTx(ctx, s, i, distanceM, allowDuplicate)
		return err
	})
	return added, err
}

// installTx is Install within a running transaction.
func installTx(ctx context.Context, s store.Stores, i store.Installation, distanceM *int64, allowDuplicate bool) (store.Installation, error) {
	c, err := s.Components().GetComponent(ctx, i.OwnerID, i.ComponentID)
	if err != nil {
		return store.Installation{}, err
	}
	b, err := s.Bikes().GetBike(ctx, i.OwnerID, i.BikeID)
	if err != nil {
		return store.Installation{}, err
	}
	if !c.Active() || !b.Active() {
		return store.Installation{}, ErrRetired
	}
	if err := checkOverlap(ctx, s, i); err != nil {
		return store.Installation{}, err
	}
	if !allowDuplicate {
		if err := checkDuplicate(ctx, s, i, c.Type); err != nil {
			return store.Installation{}, err
		}
	}
	if err := recordGiven(ctx, s, i.OwnerID, i.BikeID, i.InstalledAt, distanceM); err != nil {
		return store.Installation{}, err
	}
	m, err := distanceAt(ctx, s, i.OwnerID, i.BikeID, i.InstalledAt, distanceM)
	if err != nil {
		return store.Installation{}, err
	}
	i.InstalledM, i.InstalledAuto = m, distanceM == nil
	added, err := s.Components().AddInstallation(ctx, i)
	if errors.Is(err, store.ErrConflict) {
		return store.Installation{}, ErrAlreadyInstalled
	}
	return added, err
}

// Remove ends an open installation. If distanceM is nil, the bike's
// odometer at the removal date is used; otherwise it is recorded as a
// reading if it differs.
func Remove(ctx context.Context, st store.Store, ownerID, installationID string, at time.Time, distanceM *int64) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		inst, err := s.Components().GetInstallation(ctx, ownerID, installationID)
		if err != nil {
			return err
		}
		if !inst.Open() {
			return store.ErrNotFound
		}
		return closeInstallation(ctx, s, inst, at, distanceM)
	})
}

// checkOverlap returns ErrAlreadyInstalled or ErrOverlap if i overlaps
// another installation of the same component.
func checkOverlap(ctx context.Context, s store.Stores, i store.Installation) error {
	others, err := s.Components().ListInstallations(ctx, i.OwnerID, i.ComponentID)
	if err != nil {
		return err
	}
	for _, o := range others {
		switch {
		case o.ID == i.ID:
		case o.Open() && i.Open():
			return ErrAlreadyInstalled
		case before(i.InstalledAt, o.RemovedAt) && before(o.InstalledAt, i.RemovedAt):
			return ErrOverlap
		}
	}
	return nil
}

// checkDuplicate returns a DuplicateError if another component of type typ
// is on the bike at i's position and date. Type "other" may be installed
// any number of times.
func checkDuplicate(ctx context.Context, s store.Stores, i store.Installation, typ string) error {
	if typ == "other" {
		return nil
	}
	all, err := s.Components().ListAllInstallations(ctx, i.OwnerID)
	if err != nil {
		return err
	}
	for _, o := range InstalledOn(all, i.BikeID, i.InstalledAt) {
		if o.ComponentID == i.ComponentID || o.Position != i.Position {
			continue
		}
		c, err := s.Components().GetComponent(ctx, i.OwnerID, o.ComponentID)
		if err != nil {
			return err
		}
		if c.Type == typ {
			return &DuplicateError{ComponentID: c.ID}
		}
	}
	return nil
}

// before reports whether t is before end; a nil end is open (infinite).
func before(t time.Time, end *time.Time) bool { return end == nil || t.Before(*end) }

// distanceAt returns distanceM if given, otherwise the bike's computed
// odometer at that date. A bike with an odometer at going into service can
// get components before its first ride.
func distanceAt(ctx context.Context, s store.Stores, ownerID, bikeID string, at time.Time, distanceM *int64) (int64, error) {
	if distanceM != nil {
		return *distanceM, nil
	}
	t, err := odometer.LoadBike(ctx, s, ownerID, bikeID)
	if err != nil {
		return 0, err
	}
	m, ok := t.At(at)
	if !ok {
		return 0, ErrNoOdometer
	}
	return m, nil
}

// InstalledOn returns the installations of insts that were on the bike at
// the given time. A component removed at that time no longer counts.
func InstalledOn(insts []store.Installation, bikeID string, at time.Time) []store.Installation {
	var out []store.Installation
	for _, i := range insts {
		if i.BikeID == bikeID && !i.InstalledAt.After(at) && before(at, i.RemovedAt) {
			out = append(out, i)
		}
	}
	return out
}

// Mileage sums the distance a component covered over its installations.
// Open installations count up to the bike's current odometer in latest.
func Mileage(insts []store.Installation, latest map[string]int64) int64 {
	var total int64
	for _, i := range insts {
		end := latest[i.BikeID]
		if i.RemovedM != nil {
			end = *i.RemovedM
		}
		if end > i.InstalledM {
			total += end - i.InstalledM
		}
	}
	return total
}

// Wear describes how close a component is to its limits.
type Wear struct {
	MileageM int64
	// Percent of MaxDistanceM used, -1 without a distance limit.
	Percent      int
	OverDistance bool
	Overdue      bool
}

// WearOf computes the wear of c with the given mileage at time now.
func WearOf(c store.Component, mileageM int64, now time.Time) Wear {
	w := Wear{MileageM: mileageM, Percent: -1}
	if c.MaxDistanceM != nil {
		w.Percent = int(mileageM * 100 / *c.MaxDistanceM)
		w.OverDistance = mileageM >= *c.MaxDistanceM
	}
	if c.ReplaceBy != nil {
		w.Overdue = !now.Before(*c.ReplaceBy)
	}
	return w
}

// Alert reports whether any limit is reached.
func (w Wear) Alert() bool { return w.OverDistance || w.Overdue }
