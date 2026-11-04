package sqlite

import (
	"context"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite/db"
)

type odometerStore struct{ q *db.Queries }

func toReading(r db.OdometerReading) (store.OdometerReading, error) {
	recorded, err := parseTime(r.RecordedAt)
	if err != nil {
		return store.OdometerReading{}, err
	}
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return store.OdometerReading{}, err
	}
	return store.OdometerReading{
		ID: r.ID, OwnerID: r.OwnerID, BikeID: r.BikeID, RecordedAt: recorded,
		DistanceM: r.DistanceM, Source: r.Source, CreatedAt: created,
	}, nil
}

func (s odometerStore) AddReading(ctx context.Context, o store.OdometerReading) (store.OdometerReading, error) {
	r, err := s.q.AddReading(ctx, db.AddReadingParams{
		ID: o.ID, OwnerID: o.OwnerID, BikeID: o.BikeID,
		RecordedAt: formatTime(o.RecordedAt),
		DistanceM:  o.DistanceM, Source: o.Source,
		CreatedAt: formatTime(o.CreatedAt),
	})
	if err != nil {
		return store.OdometerReading{}, mapErr(err)
	}
	return toReading(r)
}

func (s odometerStore) ListReadings(ctx context.Context, ownerID, bikeID string) ([]store.OdometerReading, error) {
	rows, err := s.q.ListReadings(ctx, db.ListReadingsParams{OwnerID: ownerID, BikeID: bikeID})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.OdometerReading, 0, len(rows))
	for _, r := range rows {
		o, err := toReading(r)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (s odometerStore) PrevReading(ctx context.Context, ownerID, bikeID string, at time.Time, excludeID string) (store.OdometerReading, error) {
	r, err := s.q.PrevReading(ctx, db.PrevReadingParams{OwnerID: ownerID, BikeID: bikeID, RecordedAt: formatTime(at), ExcludeID: excludeID})
	if err != nil {
		return store.OdometerReading{}, mapErr(err)
	}
	return toReading(r)
}

func (s odometerStore) NextReading(ctx context.Context, ownerID, bikeID string, at time.Time, excludeID string) (store.OdometerReading, error) {
	r, err := s.q.NextReading(ctx, db.NextReadingParams{OwnerID: ownerID, BikeID: bikeID, RecordedAt: formatTime(at), ExcludeID: excludeID})
	if err != nil {
		return store.OdometerReading{}, mapErr(err)
	}
	return toReading(r)
}

func (s odometerStore) ListAllReadings(ctx context.Context, ownerID string) ([]store.OdometerReading, error) {
	rows, err := s.q.ListAllReadings(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toReading)
}

func (s odometerStore) GetReading(ctx context.Context, ownerID, id string) (store.OdometerReading, error) {
	r, err := s.q.GetReading(ctx, db.GetReadingParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.OdometerReading{}, mapErr(err)
	}
	return toReading(r)
}

func (s odometerStore) UpdateReading(ctx context.Context, o store.OdometerReading) error {
	n, err := s.q.UpdateReading(ctx, db.UpdateReadingParams{
		RecordedAt: formatTime(o.RecordedAt), DistanceM: o.DistanceM, OwnerID: o.OwnerID, ID: o.ID,
	})
	return rowsOrNotFound(n, err)
}

func (s odometerStore) DeleteReading(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteReading(ctx, db.DeleteReadingParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}
