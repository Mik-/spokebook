package sqlite

import (
	"context"
	"strings"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite/db"
)

type bikeStore struct{ q *db.Queries }

func toBike(r db.Bike) (store.Bike, error) {
	b := store.Bike{
		ID: r.ID, OwnerID: r.OwnerID, Name: r.Name, Type: r.Type, Notes: r.Notes,
		Origin: r.Origin, Dealer: r.Dealer, PriceCents: intPtr(r.PriceCents), Currency: r.Currency,
		InServiceM: intPtr(r.InServiceM), Features: strings.Fields(r.Features), RetireReason: r.RetireReason,
	}
	var err error
	if b.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
		return store.Bike{}, err
	}
	if b.PurchasedAt, err = timePtr(r.PurchasedAt); err != nil {
		return store.Bike{}, err
	}
	if b.InServiceAt, err = timePtr(r.InServiceAt); err != nil {
		return store.Bike{}, err
	}
	if b.RetiredAt, err = timePtr(r.RetiredAt); err != nil {
		return store.Bike{}, err
	}
	return b, nil
}

func (s bikeStore) CreateBike(ctx context.Context, b store.Bike) (store.Bike, error) {
	r, err := s.q.CreateBike(ctx, db.CreateBikeParams{
		ID: b.ID, OwnerID: b.OwnerID, Name: b.Name, Type: b.Type, Notes: b.Notes,
		Origin: b.Origin, PurchasedAt: nullTime(b.PurchasedAt), Dealer: b.Dealer,
		PriceCents: nullInt(b.PriceCents), Currency: b.Currency,
		InServiceAt: nullTime(b.InServiceAt), InServiceM: nullInt(b.InServiceM),
		Features: strings.Join(b.Features, " "), CreatedAt: formatTime(b.CreatedAt),
	})
	if err != nil {
		return store.Bike{}, mapErr(err)
	}
	return toBike(r)
}

func (s bikeStore) GetBike(ctx context.Context, ownerID, id string) (store.Bike, error) {
	r, err := s.q.GetBike(ctx, db.GetBikeParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.Bike{}, mapErr(err)
	}
	return toBike(r)
}

func (s bikeStore) ListBikes(ctx context.Context, ownerID string) ([]store.Bike, error) {
	rows, err := s.q.ListBikes(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.Bike, 0, len(rows))
	for _, r := range rows {
		b, err := toBike(r)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (s bikeStore) UpdateBike(ctx context.Context, b store.Bike) error {
	n, err := s.q.UpdateBike(ctx, db.UpdateBikeParams{
		Name: b.Name, Type: b.Type, Notes: b.Notes,
		Origin: b.Origin, PurchasedAt: nullTime(b.PurchasedAt), Dealer: b.Dealer,
		PriceCents: nullInt(b.PriceCents), Currency: b.Currency,
		InServiceAt: nullTime(b.InServiceAt), InServiceM: nullInt(b.InServiceM),
		Features: strings.Join(b.Features, " "), OwnerID: b.OwnerID, ID: b.ID,
	})
	return rowsOrNotFound(n, err)
}

func (s bikeStore) DeleteBike(ctx context.Context, ownerID, id string) error {
	if err := s.q.UnassignBikeRides(ctx, db.UnassignBikeRidesParams{OwnerID: ownerID, BikeID: nullString(id)}); err != nil {
		return mapErr(err)
	}
	if err := s.q.UnassignBikeGears(ctx, db.UnassignBikeGearsParams{OwnerID: ownerID, BikeID: nullString(id)}); err != nil {
		return mapErr(err)
	}
	n, err := s.q.DeleteBike(ctx, db.DeleteBikeParams{OwnerID: ownerID, ID: id})
	if err := rowsOrNotFound(n, err); err != nil {
		return err
	}
	return mapErr(s.q.DeleteOrphanAttachments(ctx, ownerID))
}

func (s bikeStore) SetBikeRetired(ctx context.Context, ownerID, id string, at *time.Time, reason string) error {
	n, err := s.q.SetBikeRetired(ctx, db.SetBikeRetiredParams{RetiredAt: nullTime(at), RetireReason: reason, OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}
