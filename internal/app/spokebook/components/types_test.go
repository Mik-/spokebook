package components_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/components"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

func TestTypes(t *testing.T) {
	f := setup(t)
	ctx, st := f.ctx, f.st
	if err := components.SeedTypes(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	types, err := st.Components().ListTypes(ctx, owner)
	if err != nil || len(types) != len(components.BuiltinTypes) {
		t.Fatalf("seeded %d types, %v", len(types), err)
	}

	own, err := components.CreateType(ctx, st, owner, "  Pedale ")
	if err != nil || own.Name != "Pedale" {
		t.Fatalf("create: %+v, %v", own, err)
	}
	if _, err := components.CreateType(ctx, st, owner, " "); !errors.Is(err, components.ErrNameRequired) {
		t.Errorf("empty name: %v", err)
	}
	if err := components.RenameType(ctx, st, owner, own.ID, ""); !errors.Is(err, components.ErrNameRequired) {
		t.Errorf("own type without name: %v", err)
	}
	if err := components.RenameType(ctx, st, owner, "chain", "Kette 11-fach"); err != nil {
		t.Fatal(err)
	}
	if err := components.RenameType(ctx, st, owner, "chain", ""); err != nil {
		t.Errorf("reset built-in name: %v", err)
	}

	// The fixture's component uses "wheel".
	if err := components.DeleteType(ctx, st, owner, "wheel"); !errors.Is(err, components.ErrTypeInUse) {
		t.Errorf("delete used type: %v", err)
	}
	if err := components.DeleteType(ctx, st, owner, "saddle"); err != nil {
		t.Fatal(err)
	}
	if _, err := components.ActiveType(ctx, st, owner, "saddle"); !errors.Is(err, components.ErrInvalidType) {
		t.Errorf("deleted type still active: %v", err)
	}
	if err := components.RenameType(ctx, st, owner, "saddle", "x"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("rename deleted type: %v", err)
	}

	// Seeding again must not bring back the deleted built-in type.
	if err := components.SeedTypes(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := components.ActiveType(ctx, st, owner, "saddle"); !errors.Is(err, components.ErrInvalidType) {
		t.Errorf("seeding restored a deleted type: %v", err)
	}

	// Restoring shows it again; only hidden types can be restored.
	if err := components.RestoreType(ctx, st, owner, "saddle"); err != nil {
		t.Fatal(err)
	}
	if _, err := components.ActiveType(ctx, st, owner, "saddle"); err != nil {
		t.Errorf("restored type not active: %v", err)
	}
	for _, id := range []string{"saddle", "steel"} {
		if err := components.RestoreType(ctx, st, owner, id); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("restore %s: %v", id, err)
		}
	}
}

func TestInstallPosition(t *testing.T) {
	f := setup(t)
	f.reading(0, "2026-01-01", 0)
	_, err := components.Install(f.ctx, f.st, store.Installation{
		ID: uuid.NewString(), OwnerID: owner, ComponentID: f.comp, BikeID: f.bikes[0],
		Position: "middle", InstalledAt: day("2026-01-01"), CreatedAt: time.Now(),
	}, nil, false)
	if !errors.Is(err, components.ErrInvalidPosition) {
		t.Errorf("invalid position: %v", err)
	}
	inst, err := components.Install(f.ctx, f.st, store.Installation{
		ID: uuid.NewString(), OwnerID: owner, ComponentID: f.comp, BikeID: f.bikes[0],
		Position: "left", InstalledAt: day("2026-01-01"), CreatedAt: time.Now(),
	}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.st.Components().GetInstallation(f.ctx, owner, inst.ID)
	if err != nil || got.Position != "left" {
		t.Errorf("position not stored: %+v, %v", got, err)
	}
}
