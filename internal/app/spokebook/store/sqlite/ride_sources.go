package sqlite

import (
	"context"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite/db"
)

func toRideSource(r db.RideSource) (store.RideSource, error) {
	out := store.RideSource{
		OwnerID: r.OwnerID, Source: r.Source, Credentials: r.Credentials,
		RideSourceRun: store.RideSourceRun{
			LastError: r.LastError, LastImported: int(r.LastImported), LastUnassigned: int(r.LastUnassigned),
			More: r.More != 0,
		},
	}
	var err error
	if out.Since, err = parseTime(r.Since); err != nil {
		return store.RideSource{}, err
	}
	if out.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
		return store.RideSource{}, err
	}
	if out.Cursor, err = timePtr(r.Cursor); err != nil {
		return store.RideSource{}, err
	}
	if out.LastRunAt, err = timePtr(r.LastRunAt); err != nil {
		return store.RideSource{}, err
	}
	return out, nil
}

func (s rideStore) CreateRideSource(ctx context.Context, src store.RideSource) error {
	return mapErr(s.q.CreateRideSource(ctx, db.CreateRideSourceParams{
		OwnerID: src.OwnerID, Source: src.Source, Credentials: src.Credentials, Since: formatTime(src.Since),
		CreatedAt: formatTime(src.CreatedAt),
	}))
}

func (s rideStore) GetRideSource(ctx context.Context, ownerID, source string) (store.RideSource, error) {
	r, err := s.q.GetRideSource(ctx, db.GetRideSourceParams{OwnerID: ownerID, Source: source})
	if err != nil {
		return store.RideSource{}, mapErr(err)
	}
	return toRideSource(r)
}

func (s rideStore) ListAllRideSources(ctx context.Context) ([]store.RideSource, error) {
	rows, err := s.q.ListAllRideSources(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toRideSource)
}

func (s rideStore) SetRideSourceCredentials(ctx context.Context, ownerID, source string, credentials []byte) error {
	n, err := s.q.SetRideSourceCredentials(ctx, db.SetRideSourceCredentialsParams{
		Credentials: credentials, OwnerID: ownerID, Source: source,
	})
	return rowsOrNotFound(n, err)
}

func (s rideStore) SetRideSourceSince(ctx context.Context, ownerID, source string, since time.Time, cursor *time.Time) error {
	n, err := s.q.SetRideSourceSince(ctx, db.SetRideSourceSinceParams{
		Since: formatTime(since), Cursor: nullTime(cursor), OwnerID: ownerID, Source: source,
	})
	return rowsOrNotFound(n, err)
}

func (s rideStore) SetRideSourceRun(ctx context.Context, ownerID, source string, run store.RideSourceRun) error {
	n, err := s.q.SetRideSourceRun(ctx, db.SetRideSourceRunParams{
		Cursor: nullTime(run.Cursor), LastRunAt: nullTime(run.LastRunAt), LastError: run.LastError,
		LastImported: int64(run.LastImported), LastUnassigned: int64(run.LastUnassigned), More: boolInt(run.More),
		OwnerID: ownerID, Source: source,
	})
	return rowsOrNotFound(n, err)
}

func (s rideStore) DeleteRideSource(ctx context.Context, ownerID, source string) error {
	if err := s.q.DeleteRideSourceItems(ctx, db.DeleteRideSourceItemsParams{OwnerID: ownerID, Source: source}); err != nil {
		return mapErr(err)
	}
	n, err := s.q.DeleteRideSource(ctx, db.DeleteRideSourceParams{OwnerID: ownerID, Source: source})
	return rowsOrNotFound(n, err)
}

func (s rideStore) HasRideSourceItem(ctx context.Context, ownerID, source, itemID string) (bool, error) {
	ok, err := s.q.HasRideSourceItem(ctx, db.HasRideSourceItemParams{OwnerID: ownerID, Source: source, ItemID: itemID})
	return ok, mapErr(err)
}

func (s rideStore) AddRideSourceItem(ctx context.Context, ownerID, source, itemID string, at time.Time) error {
	return mapErr(s.q.AddRideSourceItem(ctx, db.AddRideSourceItemParams{
		OwnerID: ownerID, Source: source, ItemID: itemID, CreatedAt: formatTime(at),
	}))
}
