package components_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/odometer"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

const owner = "o1"

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

type fixture struct {
	t     *testing.T
	ctx   context.Context
	st    store.Store
	bikes []string
	comp  string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &fixture{t: t, ctx: ctx, st: st}
	for _, name := range []string{"road", "gravel"} {
		b, err := st.Bikes().CreateBike(ctx, store.Bike{ID: uuid.NewString(), OwnerID: owner, Name: name, CreatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		f.bikes = append(f.bikes, b.ID)
	}
	c, err := st.Components().CreateComponent(ctx, store.Component{
		ID: uuid.NewString(), OwnerID: owner, Type: "wheel", MaxDistanceM: ptr(int64(10_000_000)), CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.comp = c.ID
	return f
}

func (f *fixture) reading(bike int, date string, m int64) {
	f.t.Helper()
	_, err := f.st.Odometer().AddReading(f.ctx, store.OdometerReading{
		ID: uuid.NewString(), OwnerID: owner, BikeID: f.bikes[bike], RecordedAt: day(date),
		DistanceM: m, Source: store.SourceManual, CreatedAt: time.Now(),
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) install(bike int, date string, m *int64) (store.Installation, error) {
	return components.Install(f.ctx, f.st, store.Installation{
		ID: uuid.NewString(), OwnerID: owner, ComponentID: f.comp, BikeID: f.bikes[bike],
		InstalledAt: day(date), CreatedAt: time.Now(),
	}, m, false)
}

func (f *fixture) mileage() int64 {
	f.t.Helper()
	insts, err := f.st.Components().ListInstallations(f.ctx, owner, f.comp)
	if err != nil {
		f.t.Fatal(err)
	}
	latest, err := odometer.Latest(f.ctx, f.st, owner)
	if err != nil {
		f.t.Fatal(err)
	}
	return components.Mileage(insts, latest)
}

// A wheelset moves from the road bike to the gravel bike; its mileage sums
// up both installations.
func TestInstallMoveAndMileage(t *testing.T) {
	f := setup(t)

	if _, err := f.install(0, "2026-01-01", nil); !errors.Is(err, components.ErrNoOdometer) {
		t.Fatalf("install without odometer: want ErrNoOdometer, got %v", err)
	}
	f.reading(0, "2026-01-01", 1_000_000)
	first, err := f.install(0, "2026-01-01", nil)
	if err != nil || first.InstalledM != 1_000_000 {
		t.Fatalf("install: %+v, %v", first, err)
	}
	if _, err := f.install(1, "2026-02-01", ptr(int64(0))); !errors.Is(err, components.ErrAlreadyInstalled) {
		t.Fatalf("second open installation: want ErrAlreadyInstalled, got %v", err)
	}

	f.reading(0, "2026-03-01", 1_400_000)
	if got := f.mileage(); got != 400_000 {
		t.Errorf("mileage while installed = %d, want 400000", got)
	}

	if err := components.Remove(f.ctx, f.st, owner, first.ID, day("2025-12-31"), nil); !errors.Is(err, components.ErrBeforeInstallation) {
		t.Errorf("removal before installation date: got %v", err)
	}
	// An entered odometer is recorded, so it must fit the readings.
	var low *odometer.TooLowError
	if err := components.Remove(f.ctx, f.st, owner, first.ID, day("2026-03-01"), ptr(int64(999_999))); !errors.As(err, &low) {
		t.Errorf("removal below recorded odometer: got %v", err)
	}
	if err := components.Remove(f.ctx, f.st, owner, first.ID, day("2026-03-01"), nil); err != nil {
		t.Fatal(err)
	}
	if err := components.Remove(f.ctx, f.st, owner, first.ID, day("2026-03-02"), nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("removing twice: want ErrNotFound, got %v", err)
	}

	// Riding the road bike further no longer counts.
	f.reading(0, "2026-04-01", 2_000_000)
	if got := f.mileage(); got != 400_000 {
		t.Errorf("mileage after removal = %d, want 400000", got)
	}

	f.reading(1, "2026-03-05", 500_000)
	if _, err := f.install(1, "2026-02-28", nil); !errors.Is(err, components.ErrOverlap) {
		t.Errorf("install before last removal: want ErrOverlap, got %v", err)
	}
	if _, err := f.install(1, "2026-03-05", nil); err != nil {
		t.Fatal(err)
	}
	f.reading(1, "2026-05-01", 650_000)
	if got := f.mileage(); got != 550_000 {
		t.Errorf("mileage across two bikes = %d, want 550000", got)
	}
}

func TestWear(t *testing.T) {
	c := store.Component{MaxDistanceM: ptr(int64(3_000_000)), ReplaceBy: ptr(day("2026-11-01"))}
	tests := []struct {
		mileage int64
		now     string
		want    components.Wear
	}{
		{1_500_000, "2026-10-01", components.Wear{MileageM: 1_500_000, Percent: 50}},
		{3_000_000, "2026-10-01", components.Wear{MileageM: 3_000_000, Percent: 100, OverDistance: true}},
		{0, "2026-11-01", components.Wear{Percent: 0, Overdue: true}},
	}
	for _, tt := range tests {
		if got := components.WearOf(c, tt.mileage, day(tt.now)); got != tt.want {
			t.Errorf("WearOf(%d, %s) = %+v, want %+v", tt.mileage, tt.now, got, tt.want)
		}
	}
	if got := components.WearOf(store.Component{}, 5, time.Now()); got.Percent != -1 || got.Alert() {
		t.Errorf("without limits: %+v", got)
	}
}

// Without a distance, the odometer at the given date is used, not the
// current one.
func TestInstallUsesOdometerAtDate(t *testing.T) {
	f := setup(t)
	f.reading(0, "2026-01-01", 1_000_000)
	f.reading(0, "2026-06-01", 3_000_000)

	if _, err := f.install(0, "2025-12-31", nil); !errors.Is(err, components.ErrNoOdometer) {
		t.Fatalf("before first reading: want ErrNoOdometer, got %v", err)
	}
	inst, err := f.install(0, "2026-03-01", nil)
	if err != nil || inst.InstalledM != 1_000_000 {
		t.Fatalf("install: %+v, %v", inst, err)
	}
	if err := components.Remove(f.ctx, f.st, owner, inst.ID, day("2026-06-01"), nil); err != nil {
		t.Fatal(err)
	}
	if got := f.mileage(); got != 2_000_000 {
		t.Errorf("mileage = %d, want 2000000", got)
	}
}

func TestUpdateInstallation(t *testing.T) {
	f := setup(t)
	f.reading(0, "2026-01-01", 0)
	first, err := f.install(0, "2026-01-01", ptr(int64(0)))
	if err != nil {
		t.Fatal(err)
	}
	if err := components.Remove(f.ctx, f.st, owner, first.ID, day("2026-03-01"), ptr(int64(1000))); err != nil {
		t.Fatal(err)
	}
	second, err := f.install(1, "2026-04-01", ptr(int64(500)))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		edit func(i *store.Installation)
		want error
	}{
		{"removal date only", func(i *store.Installation) { i.RemovedAt = ptr(day("2026-05-01")) }, components.ErrIncompleteRemoval},
		{"removal before installation", func(i *store.Installation) {
			i.RemovedAt, i.RemovedM = ptr(day("2026-03-31")), ptr(int64(600))
		}, components.ErrBeforeInstallation},
		{"start inside first", func(i *store.Installation) { i.InstalledAt = day("2026-02-01") }, components.ErrOverlap},
		{"start on removal day", func(i *store.Installation) { i.InstalledAt = day("2026-03-01") }, nil},
		{"close it", func(i *store.Installation) {
			i.RemovedAt, i.RemovedM = ptr(day("2026-06-01")), ptr(int64(900))
		}, nil},
		{"bad position", func(i *store.Installation) { i.Position = "top" }, components.ErrInvalidPosition},
	}
	for _, tt := range tests {
		i, err := f.st.Components().GetInstallation(f.ctx, owner, second.ID)
		if err != nil {
			t.Fatal(err)
		}
		tt.edit(&i)
		if err := components.UpdateInstallation(f.ctx, f.st, i); !errors.Is(err, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, err, tt.want)
		}
	}
	// Reopening the first installation would overlap the (now closed) second.
	i, _ := f.st.Components().GetInstallation(f.ctx, owner, first.ID)
	i.RemovedAt, i.RemovedM = nil, nil
	if err := components.UpdateInstallation(f.ctx, f.st, i); !errors.Is(err, components.ErrOverlap) {
		t.Errorf("reopen first: got %v, want ErrOverlap", err)
	}
	if got := f.mileage(); got != 1000+400 {
		t.Errorf("mileage = %d, want 1400", got)
	}
}

func TestDeleteBike(t *testing.T) {
	f := setup(t)
	f.reading(0, "2026-01-01", 0)
	inst, err := f.install(0, "2026-01-01", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := components.DeleteBike(f.ctx, f.st, owner, f.bikes[0]); !errors.Is(err, components.ErrBikeHasInstallations) {
		t.Errorf("bike with installation: %v", err)
	}
	// Even a closed installation keeps the bike.
	if err := components.Remove(f.ctx, f.st, owner, inst.ID, day("2026-01-02"), nil); err != nil {
		t.Fatal(err)
	}
	if err := components.DeleteBike(f.ctx, f.st, owner, f.bikes[0]); !errors.Is(err, components.ErrBikeHasInstallations) {
		t.Errorf("bike with past installation: %v", err)
	}
	if err := components.DeleteBike(f.ctx, f.st, owner, f.bikes[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.Bikes().GetBike(f.ctx, owner, f.bikes[1]); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("bike still there: %v", err)
	}
}
