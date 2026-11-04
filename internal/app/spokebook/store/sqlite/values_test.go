package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

func ptr(f float64) *float64 { return &f }

func TestComponentValues(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	c, err := s.Components().CreateComponent(ctx, store.Component{ID: newID(), OwnerID: "o1", Type: "stem", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	vs := []store.Value{
		{ID: newID(), Key: "clamp", Kind: store.KindTorque, Label: "Clamp", From: ptr(5), To: ptr(6), Unit: "Nm", Hint: "paste"},
		{ID: newID(), Key: "steerer", Kind: store.KindTorque, Label: "Steerer", From: ptr(4), To: ptr(4), Unit: "Nm"},
		{ID: newID(), Key: "wear", Kind: store.KindLimit, Label: "Wear", To: ptr(0.5), Unit: "%"},
	}
	if err := s.Components().ReplaceComponentValues(ctx, "o1", c.ID, vs); err != nil {
		t.Fatal(err)
	}
	got, err := s.Components().ListComponentValues(ctx, "o1", c.ID)
	if err != nil || len(got) != 3 {
		t.Fatalf("list %+v, %v", got, err)
	}
	if got[0].Label != "Clamp" || got[0].Key != "clamp" || got[0].Kind != store.KindTorque || got[0].Hint != "paste" ||
		*got[0].To != 6 || got[2].From != nil || *got[2].To != 0.5 || got[2].Position != 2 {
		t.Errorf("got %+v", got)
	}
	if units, _ := s.Components().ListValueUnits(ctx, "o1"); len(units) != 2 {
		t.Errorf("units %v", units)
	}

	// Replacing drops the old values.
	if err := s.Components().ReplaceComponentValues(ctx, "o1", c.ID, vs[:1]); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Components().ListComponentValues(ctx, "o1", c.ID); len(got) != 1 {
		t.Errorf("after replace: %+v", got)
	}

	// Another owner can't see or add values.
	if got, _ := s.Components().ListComponentValues(ctx, "o2", c.ID); len(got) != 0 {
		t.Errorf("other owner sees %+v", got)
	}
	if err := s.Components().ReplaceComponentValues(ctx, "o2", c.ID, []store.Value{{ID: newID(), Kind: store.KindLimit, Label: "x", From: ptr(1)}}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other owner: want ErrNotFound, got %v", err)
	}

	// The database rejects values without a number, with to < from or
	// without a valid kind.
	for _, v := range []store.Value{
		{ID: newID(), Kind: store.KindTarget, Label: "x"},
		{ID: newID(), Kind: store.KindTarget, Label: "x", From: ptr(2), To: ptr(1)},
		{ID: newID(), Kind: "min", Label: "x", From: ptr(1)},
	} {
		if err := s.Components().ReplaceComponentValues(ctx, "o1", c.ID, []store.Value{v}); err == nil {
			t.Errorf("%+v accepted", v)
		}
	}

	// Deleting the component deletes its values.
	if err := s.Components().DeleteComponent(ctx, "o1", c.ID); err != nil {
		t.Fatal(err)
	}
	if labels, _ := s.Components().ListValueLabels(ctx, "o1"); len(labels) != 0 {
		t.Errorf("values left after delete: %v", labels)
	}
}

func TestTypeValues(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if err := s.Components().SeedType(ctx, "o1", "stem", time.Now()); err != nil {
		t.Fatal(err)
	}
	vs := []store.Value{
		{ID: newID(), Key: "clamp", Kind: store.KindTorque, Label: "Clamp", From: ptr(5), To: ptr(6), Unit: "Nm", Hint: "paste"},
		{ID: newID(), Key: "steerer", Kind: store.KindTorque, Label: "Steerer", From: ptr(5), Unit: "kgf"},
	}
	if err := s.Components().ReplaceTypeValues(ctx, "o1", "stem", vs); err != nil {
		t.Fatal(err)
	}
	got, err := s.Components().ListTypeValues(ctx, "o1", "stem")
	if err != nil || len(got) != 2 || got[0].Key != "clamp" || got[0].Hint != "paste" || got[1].Position != 1 {
		t.Fatalf("list %+v, %v", got, err)
	}
	if units, _ := s.Components().ListValueUnits(ctx, "o1"); len(units) != 2 {
		t.Errorf("units %v", units)
	}
	if err := s.Components().ReplaceTypeValues(ctx, "o1", "stem", vs[1:]); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Components().ListTypeValues(ctx, "o1", "stem"); len(got) != 1 {
		t.Errorf("after replace: %+v", got)
	}
	// Unknown types (or another owner's) have no values.
	err = s.Components().ReplaceTypeValues(ctx, "o2", "stem", vs[:1])
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other owner: want ErrNotFound, got %v", err)
	}
}

func TestBikeValues(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	b, err := s.Bikes().CreateBike(ctx, store.Bike{ID: newID(), OwnerID: "o1", Name: "Gravel", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	vs := []store.Value{
		{ID: newID(), Key: "saddle_height", Kind: store.KindInfo, Label: "Saddle height", From: ptr(745), To: ptr(745), Unit: "mm", Hint: "BB to top"},
		{ID: newID(), Key: "head_angle", Kind: store.KindInfo, Label: "Head angle", From: ptr(71.5), To: ptr(71.5), Unit: "°"},
	}
	if err := s.Bikes().ReplaceBikeValues(ctx, "o1", b.ID, vs); err != nil {
		t.Fatal(err)
	}
	got, err := s.Bikes().ListBikeValues(ctx, "o1", b.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("list %+v, %v", got, err)
	}
	if got[0].Key != "saddle_height" || got[0].Kind != store.KindInfo || got[0].Hint != "BB to top" || *got[1].From != 71.5 || got[1].Position != 1 {
		t.Errorf("got %+v", got)
	}
	if labels, _ := s.Bikes().ListBikeValueLabels(ctx, "o1"); len(labels) != 2 || labels[0] != "Head angle" {
		t.Errorf("labels %v", labels)
	}
	if units, _ := s.Bikes().ListBikeValueUnits(ctx, "o1"); len(units) != 2 {
		t.Errorf("units %v", units)
	}

	// Another owner can't see or add values.
	if got, _ := s.Bikes().ListBikeValues(ctx, "o2", b.ID); len(got) != 0 {
		t.Errorf("other owner sees %+v", got)
	}
	if err := s.Bikes().ReplaceBikeValues(ctx, "o2", b.ID, []store.Value{{ID: newID(), Kind: store.KindInfo, Label: "x", From: ptr(1)}}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other owner: want ErrNotFound, got %v", err)
	}

	// Deleting the bike deletes its values.
	if err := s.Bikes().DeleteBike(ctx, "o1", b.ID); err != nil {
		t.Fatal(err)
	}
	if labels, _ := s.Bikes().ListBikeValueLabels(ctx, "o1"); len(labels) != 0 {
		t.Errorf("values left after delete: %v", labels)
	}
}
