package catalog_test

import (
	"errors"
	"testing"

	"github.com/spokebook/spokebook/internal/app/spokebook/catalog"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

func iptr(v int64) *int64 { return &v }

func TestLevelIntervals(t *testing.T) {
	ctx, st := setup(t)
	got, err := catalog.LevelIntervals(ctx, st, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || *got["minor"].Months != 6 || *got["minor"].DistanceM != 1_500_000 || got["monthly"].DistanceM != nil {
		t.Fatalf("defaults = %+v", got)
	}

	// The minor inspection gets kilometers only; the monthly check none.
	err = catalog.SetLevelIntervals(ctx, st, owner, map[string]store.Interval{
		"minor": {DistanceM: iptr(2_000_000)},
		"major": catalog.DefaultIntervals["major"],
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = catalog.LevelIntervals(ctx, st, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["monthly"]; ok || got["minor"].Months != nil || *got["minor"].DistanceM != 2_000_000 || *got["major"].Months != 12 {
		t.Fatalf("set = %+v", got)
	}

	if err := catalog.SetLevelIntervals(ctx, st, owner, map[string]store.Interval{"pre_ride": {Months: iptr(1)}}); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("pre_ride: %v", err)
	}
	for _, bad := range []store.Interval{{Months: iptr(0)}, {DistanceM: iptr(-5)}, {Months: iptr(121)}} {
		err := catalog.SetLevelIntervals(ctx, st, owner, map[string]store.Interval{"minor": bad})
		if !errors.Is(err, catalog.ErrInvalidInterval) {
			t.Errorf("SetLevelIntervals(%+v) = %v", bad, err)
		}
	}
}

func TestStepInterval(t *testing.T) {
	ctx, st := setup(t)
	s := newStep("Service seatpost", "replacement", "suspension")
	s.Interval.DistanceM = iptr(500_000)
	add(t, ctx, st, s)
	got, err := st.Steps().GetStep(ctx, owner, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Interval.Months != nil || *got.Interval.DistanceM != 500_000 {
		t.Errorf("interval = %+v", got.Interval)
	}

	s.Interval = store.Interval{Months: iptr(3)}
	if err := catalog.UpdateStep(ctx, st, s); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.Steps().GetStep(ctx, owner, s.ID); *got.Interval.Months != 3 || got.Interval.DistanceM != nil {
		t.Errorf("updated interval = %+v", got.Interval)
	}

	s.Interval = store.Interval{Months: iptr(-1)}
	if err := catalog.UpdateStep(ctx, st, s); !errors.Is(err, catalog.ErrInvalidInterval) {
		t.Errorf("negative interval: %v", err)
	}
}

func TestBuiltinInterval(t *testing.T) {
	ctx, st := setup(t)
	if err := catalog.SeedSteps(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	steps, err := st.Steps().ListSteps(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		want := s.BuiltinID == "tubeless_sealant"
		if has := s.Interval.Months != nil && *s.Interval.Months == 1; has != want {
			t.Errorf("%s: interval %+v", s.BuiltinID, s.Interval)
		}
	}
}
