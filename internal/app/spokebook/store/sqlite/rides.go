package sqlite

import (
	"context"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite/db"
)

type rideStore struct{ q *db.Queries }

func toRide(r db.Ride) (store.Ride, error) {
	out := store.Ride{
		ID: r.ID, OwnerID: r.OwnerID, BikeID: r.BikeID.String, Source: r.Source, ExternalID: r.ExternalID,
		GearKey: r.GearKey, BikeByGear: r.BikeByGear != 0, DistanceM: r.DistanceM, Name: r.Name,
		Discarded: r.Discarded != 0,
	}
	var err error
	for _, f := range []struct {
		dst *time.Time
		src string
	}{
		{&out.StartedAt, r.StartedAt}, {&out.EndedAt, r.EndedAt}, {&out.RiddenOn, r.RiddenOn}, {&out.CreatedAt, r.CreatedAt},
	} {
		if *f.dst, err = parseTime(f.src); err != nil {
			return store.Ride{}, err
		}
	}
	return out, nil
}

func toRideGear(r db.RideGear) (store.RideGear, error) {
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return store.RideGear{}, err
	}
	return store.RideGear{
		ID: r.ID, OwnerID: r.OwnerID, Source: r.Source, GearKey: r.GearKey, Name: r.Name,
		BikeID: r.BikeID.String, CreatedAt: created,
	}, nil
}

func (s rideStore) CreateRide(ctx context.Context, r store.Ride) (store.Ride, error) {
	row, err := s.q.CreateRide(ctx, db.CreateRideParams{
		ID: r.ID, OwnerID: r.OwnerID, BikeID: nullString(r.BikeID), Source: r.Source, ExternalID: r.ExternalID,
		GearKey: r.GearKey, BikeByGear: boolInt(r.BikeByGear), StartedAt: formatTime(r.StartedAt),
		EndedAt: formatTime(r.EndedAt), RiddenOn: formatTime(r.RiddenOn), DistanceM: r.DistanceM, Name: r.Name,
		Discarded: boolInt(r.Discarded), CreatedAt: formatTime(r.CreatedAt),
	})
	if err != nil {
		return store.Ride{}, mapErr(err)
	}
	return toRide(row)
}

func (s rideStore) GetRide(ctx context.Context, ownerID, id string) (store.Ride, error) {
	r, err := s.q.GetRide(ctx, db.GetRideParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.Ride{}, mapErr(err)
	}
	return toRide(r)
}

func (s rideStore) FindRide(ctx context.Context, ownerID, source, externalID string) (store.Ride, error) {
	r, err := s.q.FindRide(ctx, db.FindRideParams{OwnerID: ownerID, Source: source, ExternalID: externalID})
	if err != nil {
		return store.Ride{}, mapErr(err)
	}
	return toRide(r)
}

func (s rideStore) ListRides(ctx context.Context, ownerID string) ([]store.Ride, error) {
	rows, err := s.q.ListRides(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toRide)
}

func (s rideStore) OverlappingRides(ctx context.Context, ownerID string, since, until time.Time) ([]store.Ride, error) {
	rows, err := s.q.OverlappingRides(ctx, db.OverlappingRidesParams{
		OwnerID: ownerID, Since: formatTime(since), Until: formatTime(until),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toRide)
}

func (s rideStore) SetRideBike(ctx context.Context, ownerID, id, bikeID string, byGear bool) error {
	n, err := s.q.SetRideBike(ctx, db.SetRideBikeParams{
		BikeID: nullString(bikeID), BikeByGear: boolInt(byGear), OwnerID: ownerID, ID: id,
	})
	return rowsOrNotFound(n, err)
}

func (s rideStore) SetRideDiscarded(ctx context.Context, ownerID, id string, discarded bool) error {
	n, err := s.q.SetRideDiscarded(ctx, db.SetRideDiscardedParams{Discarded: boolInt(discarded), OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s rideStore) DeleteRide(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteRide(ctx, db.DeleteRideParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s rideStore) DailyDistances(ctx context.Context, ownerID string) ([]store.DayDistance, error) {
	rows, err := s.q.DailyDistances(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, func(r db.DailyDistancesRow) (store.DayDistance, error) {
		day, err := parseTime(r.RiddenOn)
		return store.DayDistance{BikeID: r.BikeID, Day: day, DistanceM: r.DistanceM}, err
	})
}

func (s rideStore) CreateRideGear(ctx context.Context, g store.RideGear) (store.RideGear, error) {
	r, err := s.q.CreateRideGear(ctx, db.CreateRideGearParams{
		ID: g.ID, OwnerID: g.OwnerID, Source: g.Source, GearKey: g.GearKey, Name: g.Name,
		BikeID: nullString(g.BikeID), CreatedAt: formatTime(g.CreatedAt),
	})
	if err != nil {
		return store.RideGear{}, mapErr(err)
	}
	return toRideGear(r)
}

func (s rideStore) GetRideGear(ctx context.Context, ownerID, id string) (store.RideGear, error) {
	r, err := s.q.GetRideGear(ctx, db.GetRideGearParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.RideGear{}, mapErr(err)
	}
	return toRideGear(r)
}

func (s rideStore) FindRideGear(ctx context.Context, ownerID, source, gearKey string) (store.RideGear, error) {
	r, err := s.q.FindRideGear(ctx, db.FindRideGearParams{OwnerID: ownerID, Source: source, GearKey: gearKey})
	if err != nil {
		return store.RideGear{}, mapErr(err)
	}
	return toRideGear(r)
}

func (s rideStore) ListRideGears(ctx context.Context, ownerID string) ([]store.RideGear, error) {
	rows, err := s.q.ListRideGears(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toRideGear)
}

func (s rideStore) DeleteRideGear(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteRideGear(ctx, db.DeleteRideGearParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s rideStore) SetRideGearBike(ctx context.Context, ownerID, id, bikeID string) error {
	n, err := s.q.SetRideGearBike(ctx, db.SetRideGearBikeParams{BikeID: nullString(bikeID), OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}
