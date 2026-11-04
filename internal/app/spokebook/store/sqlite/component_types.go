package sqlite

import (
	"context"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite/db"
)

func toComponentType(r db.ComponentType) (store.ComponentType, error) {
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return store.ComponentType{}, err
	}
	return store.ComponentType{OwnerID: r.OwnerID, ID: r.ID, Name: r.Name, Hidden: r.Hidden != 0, CreatedAt: created}, nil
}

func (s componentStore) SeedType(ctx context.Context, ownerID, id string, at time.Time) error {
	return mapErr(s.q.SeedComponentType(ctx, db.SeedComponentTypeParams{OwnerID: ownerID, ID: id, CreatedAt: formatTime(at)}))
}

func (s componentStore) CreateType(ctx context.Context, t store.ComponentType) (store.ComponentType, error) {
	r, err := s.q.CreateComponentType(ctx, db.CreateComponentTypeParams{
		OwnerID: t.OwnerID, ID: t.ID, Name: t.Name, CreatedAt: formatTime(t.CreatedAt),
	})
	if err != nil {
		return store.ComponentType{}, mapErr(err)
	}
	return toComponentType(r)
}

func (s componentStore) ListTypes(ctx context.Context, ownerID string) ([]store.ComponentType, error) {
	rows, err := s.q.ListComponentTypes(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toComponentType)
}

func (s componentStore) RenameType(ctx context.Context, ownerID, id, name string) error {
	n, err := s.q.RenameComponentType(ctx, db.RenameComponentTypeParams{Name: name, OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s componentStore) HideType(ctx context.Context, ownerID, id string) error {
	n, err := s.q.HideComponentType(ctx, db.HideComponentTypeParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s componentStore) ShowType(ctx context.Context, ownerID, id string) error {
	n, err := s.q.ShowComponentType(ctx, db.ShowComponentTypeParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s componentStore) CountComponentsOfType(ctx context.Context, ownerID, typeID string) (int64, error) {
	n, err := s.q.CountComponentsOfType(ctx, db.CountComponentsOfTypeParams{OwnerID: ownerID, Type: typeID})
	return n, mapErr(err)
}

// rowsOrNotFound turns "no row affected" into store.ErrNotFound.
func rowsOrNotFound(n int64, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
