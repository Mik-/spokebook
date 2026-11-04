package sqlite

import (
	"context"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite/db"
)

type shoppingStore struct{ q *db.Queries }

func toShoppingItem(r db.ShoppingItem) (store.ShoppingItem, error) {
	bought, err := timePtr(r.BoughtAt)
	if err != nil {
		return store.ShoppingItem{}, err
	}
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return store.ShoppingItem{}, err
	}
	return store.ShoppingItem{ID: r.ID, OwnerID: r.OwnerID, Text: r.Text, BoughtAt: bought, CreatedAt: created}, nil
}

func (s shoppingStore) CreateShoppingItem(ctx context.Context, i store.ShoppingItem) error {
	_, err := s.q.CreateShoppingItem(ctx, db.CreateShoppingItemParams{
		ID: i.ID, OwnerID: i.OwnerID, Text: i.Text, BoughtAt: nullTime(i.BoughtAt), CreatedAt: formatTime(i.CreatedAt),
	})
	return mapErr(err)
}

func (s shoppingStore) GetShoppingItem(ctx context.Context, ownerID, id string) (store.ShoppingItem, error) {
	r, err := s.q.GetShoppingItem(ctx, db.GetShoppingItemParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.ShoppingItem{}, mapErr(err)
	}
	return toShoppingItem(r)
}

func (s shoppingStore) ListShoppingItems(ctx context.Context, ownerID string) ([]store.ShoppingItem, error) {
	rows, err := s.q.ListShoppingItems(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toShoppingItem)
}

func (s shoppingStore) UpdateShoppingItem(ctx context.Context, ownerID, id, text string) error {
	n, err := s.q.UpdateShoppingItem(ctx, db.UpdateShoppingItemParams{Text: text, OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s shoppingStore) SetShoppingItemBought(ctx context.Context, ownerID, id string, at *time.Time) error {
	n, err := s.q.SetShoppingItemBought(ctx, db.SetShoppingItemBoughtParams{BoughtAt: nullTime(at), OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s shoppingStore) DeleteShoppingItem(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteShoppingItem(ctx, db.DeleteShoppingItemParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s shoppingStore) DeleteBoughtShoppingItems(ctx context.Context, ownerID string) (int64, error) {
	n, err := s.q.DeleteBoughtShoppingItems(ctx, ownerID)
	return n, mapErr(err)
}
