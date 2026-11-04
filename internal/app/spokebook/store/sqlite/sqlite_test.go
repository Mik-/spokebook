package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

func newStore(t *testing.T) store.Store {
	t.Helper()
	s, err := sqlite.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBikes(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	b, err := s.Bikes().CreateBike(ctx, store.Bike{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: "o1", Name: "Gravel", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Bikes().GetBike(ctx, "o1", b.ID)
	if err != nil || got.Name != "Gravel" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := s.Bikes().GetBike(ctx, "other", b.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other owner: want ErrNotFound, got %v", err)
	}
	list, err := s.Bikes().ListBikes(ctx, "o1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v, %v", list, err)
	}
}

func TestWithTxRollback(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	boom := errors.New("boom")
	err := s.WithTx(ctx, func(st store.Stores) error {
		if _, err := st.Bikes().CreateBike(ctx, store.Bike{ID: "x", OwnerID: "o1", Name: "n", CreatedAt: time.Now()}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if list, _ := s.Bikes().ListBikes(ctx, "o1"); len(list) != 0 {
		t.Fatalf("rollback failed: %v", list)
	}
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestOdometer(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	bike, err := s.Bikes().CreateBike(ctx, store.Bike{ID: newID(), OwnerID: "o1", Name: "Gravel", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	add := func(owner, bikeID, date string, m int64) error {
		_, err := s.Odometer().AddReading(ctx, store.OdometerReading{
			ID: newID(), OwnerID: owner, BikeID: bikeID, RecordedAt: day(date),
			DistanceM: m, Source: store.SourceManual, CreatedAt: time.Now(),
		})
		return err
	}
	if err := add("o1", bike.ID, "2026-05-01", 1_000_000); err != nil {
		t.Fatal(err)
	}
	if err := add("o1", bike.ID, "2026-10-01", 2_500_500); err != nil {
		t.Fatal(err)
	}
	if err := add("o2", bike.ID, "2026-10-02", 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other owner's bike: want ErrNotFound, got %v", err)
	}
	if err := add("o1", newID(), "2026-10-02", 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown bike: want ErrNotFound, got %v", err)
	}

	list, err := s.Odometer().ListReadings(ctx, "o1", bike.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].DistanceM != 2_500_500 || !list[0].RecordedAt.Equal(day("2026-10-01")) {
		t.Fatalf("want newest first, got %+v", list)
	}
	if list, _ := s.Odometer().ListReadings(ctx, "o2", bike.ID); len(list) != 0 {
		t.Fatalf("other owner sees readings: %+v", list)
	}
}

// Stored times must sort chronologically as text, also with fractional
// seconds (RFC3339Nano would store "...:00Z" after "...:00.5Z").
func TestTimeOrdering(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	bike, err := s.Bikes().CreateBike(ctx, store.Bike{ID: newID(), OwnerID: "o1", Name: "b", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	base := day("2026-10-25")
	for i, at := range []time.Time{base, base.Add(500 * time.Millisecond)} {
		_, err := s.Odometer().AddReading(ctx, store.OdometerReading{
			ID: newID(), OwnerID: "o1", BikeID: bike.ID, RecordedAt: at,
			DistanceM: int64(i), Source: store.SourceManual, CreatedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.Odometer().ListReadings(ctx, "o1", bike.ID)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].DistanceM != 1 {
		t.Errorf("want the later reading first, got %+v", list)
	}
	prev, err := s.Odometer().PrevReading(ctx, "o1", bike.ID, base, "")
	if err != nil || prev.DistanceM != 0 {
		t.Errorf("PrevReading(base) = %+v, %v; want the reading at base", prev, err)
	}
}

func TestFollowUps(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	step := func(title string, follow ...store.FollowUp) store.Step {
		st := store.Step{
			ID: uuid.Must(uuid.NewV7()).String(), OwnerID: "o1", Title: title, Phase: "inspection",
			FollowUps: follow, CreatedAt: time.Now().UTC(),
		}
		if err := s.Steps().CreateStep(ctx, st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	adjust := step("Adjust")
	replace := step("Replace", store.FollowUp{StepID: adjust.ID, Kind: store.FollowUpAlways})
	check := step("Check", store.FollowUp{StepID: replace.ID, Kind: store.FollowUpFinding},
		store.FollowUp{StepID: adjust.ID, Kind: store.FollowUpFinding})
	got, err := s.Steps().GetStep(ctx, "o1", check.ID)
	if err != nil || len(got.FollowUps) != 2 || got.FollowUps[0] != check.FollowUps[0] || got.FollowUps[1] != check.FollowUps[1] {
		t.Fatalf("got %+v, %v", got.FollowUps, err)
	}
	other := step("Other")
	other.FollowUps = []store.FollowUp{{StepID: adjust.ID, Kind: "sometimes"}}
	if err := s.Steps().UpdateStep(ctx, other); err == nil {
		t.Error("unknown kind saved")
	}

	// Deleting a step removes it as follow-up of others.
	if err := s.Steps().DeleteStep(ctx, "o1", adjust.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.Steps().GetStep(ctx, "o1", check.ID)
	if err != nil || len(got.FollowUps) != 1 || got.FollowUps[0].StepID != replace.ID {
		t.Fatalf("after delete %+v, %v", got.FollowUps, err)
	}

	// A maintenance step keeps its origin and can be dismissed.
	b, err := s.Bikes().CreateBike(ctx, store.Bike{ID: uuid.Must(uuid.NewV7()).String(), OwnerID: "o1", Name: "Gravel", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	m := store.Maintenance{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: "o1", BikeID: b.ID, PerformedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
		Steps: []store.MaintenanceStep{{ID: "m1", Title: "Check"}, {ID: "m2", Title: "Replace", OriginID: "m1"}, {ID: "m3", Title: "Wash", Added: true}},
	}
	if err := s.Maintenances().CreateMaintenance(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := s.Maintenances().SetStepDismissed(ctx, "o1", "m2", true); err != nil {
		t.Fatal(err)
	}
	got2, err := s.Maintenances().GetMaintenance(ctx, "o1", m.ID)
	if err != nil || got2.Steps[1].OriginID != "m1" || !got2.Steps[1].Dismissed || got2.Steps[0].Dismissed {
		t.Fatalf("got %+v, %v", got2.Steps, err)
	}
	if err := s.Maintenances().SetStepDismissed(ctx, "o2", "m2", true); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other owner: %v", err)
	}
	if !got2.Steps[2].Added || got2.Steps[1].Added {
		t.Errorf("added: %+v", got2.Steps)
	}

	// A step can be deleted from a maintenance.
	if err := s.Maintenances().DeleteMaintenanceStep(ctx, "o2", "m3"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("delete of other owner: %v", err)
	}
	if err := s.Maintenances().DeleteMaintenanceStep(ctx, "o1", "m3"); err != nil {
		t.Fatal(err)
	}
	if got2, err = s.Maintenances().GetMaintenance(ctx, "o1", m.ID); err != nil || len(got2.Steps) != 2 {
		t.Errorf("after delete: %+v, %v", got2.Steps, err)
	}
}
