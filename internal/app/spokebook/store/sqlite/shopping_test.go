package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

func TestShoppingItems(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	sh := s.Shopping()
	now := time.Now().UTC()
	var ids []string
	for i, text := range []string{"Chain oil", "Sealant", "Brake pads"} {
		id := newID()
		ids = append(ids, id)
		if err := sh.CreateShoppingItem(ctx, store.ShoppingItem{ID: id, OwnerID: "o1", Text: text, CreatedAt: now.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sh.CreateShoppingItem(ctx, store.ShoppingItem{ID: newID(), OwnerID: "o1", CreatedAt: now}); err == nil {
		t.Error("empty text accepted")
	}

	// Bought items move to the end.
	if err := sh.SetShoppingItemBought(ctx, "o1", ids[0], &now); err != nil {
		t.Fatal(err)
	}
	got, err := sh.ListShoppingItems(ctx, "o1")
	if err != nil || len(got) != 3 {
		t.Fatalf("list %+v, %v", got, err)
	}
	if got[0].Text != "Sealant" || got[1].Text != "Brake pads" || got[2].Text != "Chain oil" || !got[2].Bought() || got[0].Bought() {
		t.Errorf("order %+v", got)
	}

	if err := sh.UpdateShoppingItem(ctx, "o1", ids[1], "Tubeless sealant"); err != nil {
		t.Fatal(err)
	}
	if i, err := sh.GetShoppingItem(ctx, "o1", ids[1]); err != nil || i.Text != "Tubeless sealant" {
		t.Errorf("get %+v, %v", i, err)
	}

	// Another owner can't see or change the items.
	if got, _ := sh.ListShoppingItems(ctx, "o2"); len(got) != 0 {
		t.Errorf("other owner sees %+v", got)
	}
	if _, err := sh.GetShoppingItem(ctx, "o2", ids[1]); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("get other owner: %v", err)
	}
	for name, err := range map[string]error{
		"update": sh.UpdateShoppingItem(ctx, "o2", ids[1], "x"),
		"bought": sh.SetShoppingItemBought(ctx, "o2", ids[1], &now),
		"delete": sh.DeleteShoppingItem(ctx, "o2", ids[1]),
	} {
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s other owner: %v", name, err)
		}
	}
	if n, err := sh.DeleteBoughtShoppingItems(ctx, "o2"); err != nil || n != 0 {
		t.Errorf("delete bought other owner: %d, %v", n, err)
	}

	// Opening again, then removing bought items keeps the open ones.
	if err := sh.SetShoppingItemBought(ctx, "o1", ids[0], nil); err != nil {
		t.Fatal(err)
	}
	if err := sh.SetShoppingItemBought(ctx, "o1", ids[2], &now); err != nil {
		t.Fatal(err)
	}
	if n, err := sh.DeleteBoughtShoppingItems(ctx, "o1"); err != nil || n != 1 {
		t.Errorf("delete bought: %d, %v", n, err)
	}
	if err := sh.DeleteShoppingItem(ctx, "o1", ids[1]); err != nil {
		t.Fatal(err)
	}
	if got, _ := sh.ListShoppingItems(ctx, "o1"); len(got) != 1 || got[0].ID != ids[0] || got[0].Bought() {
		t.Errorf("after delete %+v", got)
	}
}
