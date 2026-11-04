package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite/db"
)

type componentStore struct{ q *db.Queries }

func toComponent(r db.Component) (store.Component, error) {
	c := store.Component{
		ID: r.ID, OwnerID: r.OwnerID, Type: r.Type, Manufacturer: r.Manufacturer, Model: r.Model,
		PartNumber: r.PartNumber, Dealer: r.Dealer, PriceCents: intPtr(r.PriceCents), Currency: r.Currency,
		MaxDistanceM: intPtr(r.MaxDistanceM), URL: r.Url, Notes: r.Notes, RetireReason: r.RetireReason,
	}
	var err error
	if c.PurchasedAt, err = timePtr(r.PurchasedAt); err != nil {
		return store.Component{}, err
	}
	if c.ReplaceBy, err = timePtr(r.ReplaceBy); err != nil {
		return store.Component{}, err
	}
	if c.RetiredAt, err = timePtr(r.RetiredAt); err != nil {
		return store.Component{}, err
	}
	if c.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
		return store.Component{}, err
	}
	return c, nil
}

func toInstallation(r db.Installation) (store.Installation, error) {
	i := store.Installation{
		ID: r.ID, OwnerID: r.OwnerID, ComponentID: r.ComponentID, BikeID: r.BikeID,
		Position: r.Position, InstalledM: r.InstalledM, InstalledAuto: r.InstalledAuto != 0,
		RemovedM: intPtr(r.RemovedM), RemovedAuto: r.RemovedAuto != 0,
	}
	var err error
	if i.InstalledAt, err = parseTime(r.InstalledAt); err != nil {
		return store.Installation{}, err
	}
	if i.RemovedAt, err = timePtr(r.RemovedAt); err != nil {
		return store.Installation{}, err
	}
	if i.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
		return store.Installation{}, err
	}
	return i, nil
}

func (s componentStore) CreateComponent(ctx context.Context, c store.Component) (store.Component, error) {
	r, err := s.q.CreateComponent(ctx, db.CreateComponentParams{
		ID: c.ID, OwnerID: c.OwnerID, Type: c.Type, Manufacturer: c.Manufacturer, Model: c.Model,
		PartNumber: c.PartNumber, PurchasedAt: nullTime(c.PurchasedAt), Dealer: c.Dealer, PriceCents: nullInt(c.PriceCents),
		Currency: c.Currency, MaxDistanceM: nullInt(c.MaxDistanceM), ReplaceBy: nullTime(c.ReplaceBy),
		Url: c.URL, Notes: c.Notes, CreatedAt: formatTime(c.CreatedAt),
	})
	if err != nil {
		return store.Component{}, mapErr(err)
	}
	return toComponent(r)
}

func (s componentStore) GetComponent(ctx context.Context, ownerID, id string) (store.Component, error) {
	r, err := s.q.GetComponent(ctx, db.GetComponentParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.Component{}, mapErr(err)
	}
	return toComponent(r)
}

func (s componentStore) ListComponents(ctx context.Context, ownerID string) ([]store.Component, error) {
	rows, err := s.q.ListComponents(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toComponent)
}

func (s componentStore) AddInstallation(ctx context.Context, i store.Installation) (store.Installation, error) {
	r, err := s.q.AddInstallation(ctx, db.AddInstallationParams{
		ID: i.ID, OwnerID: i.OwnerID, ComponentID: i.ComponentID, BikeID: i.BikeID,
		Position: i.Position, InstalledAt: formatTime(i.InstalledAt), InstalledM: i.InstalledM,
		InstalledAuto: boolInt(i.InstalledAuto), CreatedAt: formatTime(i.CreatedAt),
	})
	if err != nil {
		return store.Installation{}, mapErr(err)
	}
	return toInstallation(r)
}

func (s componentStore) GetInstallation(ctx context.Context, ownerID, id string) (store.Installation, error) {
	r, err := s.q.GetInstallation(ctx, db.GetInstallationParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.Installation{}, mapErr(err)
	}
	return toInstallation(r)
}

func (s componentStore) CloseInstallation(ctx context.Context, ownerID, id string, removedAt time.Time, removedM int64, auto bool) error {
	n, err := s.q.CloseInstallation(ctx, db.CloseInstallationParams{
		RemovedAt: sql.NullString{String: formatTime(removedAt), Valid: true},
		RemovedM:  sql.NullInt64{Int64: removedM, Valid: true}, RemovedAuto: boolInt(auto),
		OwnerID: ownerID, ID: id,
	})
	return rowsOrNotFound(n, err)
}

func (s componentStore) ListInstallations(ctx context.Context, ownerID, componentID string) ([]store.Installation, error) {
	rows, err := s.q.ListInstallations(ctx, db.ListInstallationsParams{OwnerID: ownerID, ComponentID: componentID})
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toInstallation)
}

func (s componentStore) ListAllInstallations(ctx context.Context, ownerID string) ([]store.Installation, error) {
	rows, err := s.q.ListAllInstallations(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toInstallation)
}

func convertAll[R, T any](rows []R, conv func(R) (T, error)) ([]T, error) {
	out := make([]T, 0, len(rows))
	for _, r := range rows {
		t, err := conv(r)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (s componentStore) UpdateComponent(ctx context.Context, c store.Component) error {
	n, err := s.q.UpdateComponent(ctx, db.UpdateComponentParams{
		Type: c.Type, Manufacturer: c.Manufacturer, Model: c.Model, PartNumber: c.PartNumber,
		PurchasedAt: nullTime(c.PurchasedAt),
		Dealer:      c.Dealer, PriceCents: nullInt(c.PriceCents), Currency: c.Currency,
		MaxDistanceM: nullInt(c.MaxDistanceM), ReplaceBy: nullTime(c.ReplaceBy), Url: c.URL, Notes: c.Notes,
		OwnerID: c.OwnerID, ID: c.ID,
	})
	return rowsOrNotFound(n, err)
}

func (s componentStore) DeleteComponent(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteComponent(ctx, db.DeleteComponentParams{OwnerID: ownerID, ID: id})
	if err := rowsOrNotFound(n, err); err != nil {
		return err
	}
	return mapErr(s.q.DeleteOrphanAttachments(ctx, ownerID))
}

func (s componentStore) ListDealers(ctx context.Context, ownerID string) ([]string, error) {
	dealers, err := s.q.ListDealers(ctx, ownerID)
	return dealers, mapErr(err)
}

func (s componentStore) UpdateInstallation(ctx context.Context, i store.Installation) error {
	n, err := s.q.UpdateInstallation(ctx, db.UpdateInstallationParams{
		BikeID: i.BikeID, Position: i.Position, InstalledAt: formatTime(i.InstalledAt), InstalledM: i.InstalledM,
		InstalledAuto: boolInt(i.InstalledAuto), RemovedAt: nullTime(i.RemovedAt), RemovedM: nullInt(i.RemovedM),
		RemovedAuto: boolInt(i.RemovedAuto), OwnerID: i.OwnerID, ID: i.ID,
	})
	return rowsOrNotFound(n, err)
}

func (s componentStore) SetInstallationDistances(ctx context.Context, ownerID, id string, installedM int64, removedM *int64) error {
	n, err := s.q.SetInstallationDistances(ctx, db.SetInstallationDistancesParams{
		InstalledM: installedM, RemovedM: nullInt(removedM), OwnerID: ownerID, ID: id,
	})
	return rowsOrNotFound(n, err)
}

func (s componentStore) DeleteInstallation(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteInstallation(ctx, db.DeleteInstallationParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s componentStore) CountInstallationsOnBike(ctx context.Context, ownerID, bikeID string) (int64, error) {
	n, err := s.q.CountInstallationsOnBike(ctx, db.CountInstallationsOnBikeParams{OwnerID: ownerID, BikeID: bikeID})
	return n, mapErr(err)
}

func (s componentStore) SetComponentRetired(ctx context.Context, ownerID, id string, at *time.Time, reason string) error {
	n, err := s.q.SetComponentRetired(ctx, db.SetComponentRetiredParams{RetiredAt: nullTime(at), RetireReason: reason, OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s componentStore) ListManufacturers(ctx context.Context, ownerID string) ([]string, error) {
	manufacturers, err := s.q.ListManufacturers(ctx, ownerID)
	return manufacturers, mapErr(err)
}
