package maintenance_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/maintenance"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

const owner = "o1"

func ptr(f float64) *float64 { return &f }

func ptr64(i int64) *int64 { return &i }

func ptrTime(t time.Time) *time.Time { return &t }

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

type fixture struct {
	t     *testing.T
	ctx   context.Context
	st    store.Store
	bike  string
	steps map[string]string // catalog step IDs by title
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := components.SeedTypes(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, ctx: ctx, st: st, steps: map[string]string{}}
	b, err := st.Bikes().CreateBike(ctx, store.Bike{ID: uuid.NewString(), OwnerID: owner, Name: "Gravel", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	f.bike = b.ID
	for _, s := range []store.Step{
		{Title: "Probefahrt", Phase: "test_ride", Levels: []string{"minor", "major"}},
		{
			Title: "Hebel", Phase: "inspection", Assembly: "brakes", Levels: []string{"minor"},
			ComponentTypes: []string{"shift_brake_lever"}, Values: []store.Value{{Label: "Pads", From: ptr(1.2), Unit: "mm"}},
		},
		{Title: "Kette tauschen", Phase: "replacement", Assembly: "drivetrain", Action: "replace"},
		{
			Title: "Kette messen", Phase: "inspection", Assembly: "drivetrain", Levels: []string{"monthly"},
			ComponentTypes: []string{"chain"}, ValueKinds: []string{store.KindLimit},
		},
	} {
		s.ID, s.OwnerID, s.CreatedAt = uuid.NewString(), owner, time.Now()
		if err := catalog.AddStep(ctx, st, s); err != nil {
			t.Fatal(err)
		}
		f.steps[s.Title] = s.ID
	}
	return f
}

// start starts a maintenance on 2026-10-26.
func (f *fixture) start(level string, extras ...string) store.Maintenance {
	f.t.Helper()
	var ids []string
	for _, title := range extras {
		ids = append(ids, f.steps[title])
	}
	m, err := maintenance.Start(f.ctx, f.st, owner, f.bike, level, ids, day("2026-10-26"), 0)
	if err != nil {
		f.t.Fatal(err)
	}
	got, err := f.st.Maintenances().GetMaintenance(f.ctx, owner, m.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	maintenance.SortSteps(&got)
	return got
}

func stepTitles(m store.Maintenance) []string {
	var out []string
	for _, s := range m.Steps {
		out = append(out, s.Title)
	}
	return out
}

// mount adds a component with values and installs it on the bike.
func (f *fixture) mount(typ, model, position, date string, vs ...store.Value) string {
	f.t.Helper()
	c := store.Component{ID: uuid.NewString(), OwnerID: owner, Type: typ, Manufacturer: "Shimano", Model: model, CreatedAt: time.Now()}
	err := components.AddToBike(f.ctx, f.st, c, vs, components.Mount{OwnerID: owner, BikeID: f.bike, Position: position, At: day(date), DistanceM: ptr64(0)})
	if err != nil {
		f.t.Fatal(err)
	}
	return c.ID
}

func TestStart(t *testing.T) {
	f := setup(t)
	torque := store.Value{Label: "Clamp", From: ptr(6), To: ptr(8), Unit: "Nm"}
	f.mount("shift_brake_lever", "GRX right", "right", "2026-01-01", torque)
	f.mount("shift_brake_lever", "GRX left", "left", "2026-01-01", torque)
	f.mount("chain", "XT", "", "2026-01-01", store.Value{Label: "Wear", To: ptr(0.5), Unit: "%"})
	old := f.mount("shift_brake_lever", "Old", "", "2025-01-01", torque)
	insts, _ := f.st.Components().ListInstallations(f.ctx, owner, old)
	if err := components.Remove(f.ctx, f.st, owner, insts[0].ID, day("2025-06-01"), ptr64(0)); err != nil {
		t.Fatal(err)
	}

	m, err := maintenance.Start(f.ctx, f.st, owner, f.bike, "minor", []string{f.steps["Kette tauschen"]}, day("2026-10-26"), 1_234_000)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.st.Maintenances().GetMaintenance(f.ctx, owner, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	maintenance.SortSteps(&got)
	// The steps of the level plus the extra one, in catalog order.
	if got.Level != "minor" || got.Title != "" || !got.Open() ||
		!slices.Equal(stepTitles(got), []string{"Hebel", "Kette tauschen", "Probefahrt"}) {
		t.Fatalf("got %+v", got)
	}
	if s := got.Steps[1]; s.CatalogStepID != f.steps["Kette tauschen"] || s.Phase != "replacement" ||
		s.Assembly != "drivetrain" || s.Action != "replace" {
		t.Errorf("copied step %+v", s)
	}
	vs := got.Steps[0].Values
	if len(vs) != 3 {
		t.Fatalf("values %+v", vs)
	}
	// Own value first, then the installed levers left before right; the
	// removed lever and the chain are not part of the step.
	if vs[0].ComponentID != "" || vs[1].ComponentPosition != "left" || vs[2].ComponentName != "Shimano GRX right" {
		t.Errorf("values %+v", vs)
	}
	if r, _ := f.st.Odometer().ListReadings(f.ctx, owner, f.bike); r[0].DistanceM != 1_234_000 {
		t.Errorf("odometer not recorded: %+v", r)
	}

	// Later changes to the catalog don't change the maintenance.
	if err := catalog.DeleteStep(f.ctx, f.st, owner, f.steps["Hebel"]); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.st.Maintenances().GetMaintenance(f.ctx, owner, m.ID); len(got.Steps) != 3 {
		t.Errorf("steps changed with catalog: %d", len(got.Steps))
	}
}

// TestStartValueKinds checks that a step gets only the component values of
// its value kinds.
func TestStartValueKinds(t *testing.T) {
	f := setup(t)
	f.mount("chain", "XT", "", "2026-01-01",
		store.Value{Label: "Wear", To: ptr(0.5), Unit: "%"}, store.Value{Label: "Pin", From: ptr(5), Unit: "Nm"})
	m := f.start("monthly")
	if len(m.Steps) != 1 || len(m.Steps[0].Values) != 1 || m.Steps[0].Values[0].Label != "Wear" {
		t.Errorf("got %+v", m.Steps)
	}
}

// TestStartTypeDefaults checks that a step gets the defaults of a type,
// overridden by the component's values with the same key, but no infos.
func TestStartTypeDefaults(t *testing.T) {
	f := setup(t)
	err := components.SetTypeValues(f.ctx, f.st, owner, "shift_brake_lever", []store.Value{
		{Label: "Clamp", From: ptr(6), To: ptr(8), Unit: "Nm", Hint: "paste"},
		{Label: "Lever travel", Kind: store.KindLimit, To: ptr(10), Unit: "mm"},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.mount("shift_brake_lever", "GRX", "", "2026-01-01", store.Value{Label: "Clamp", Kind: store.KindTorque, From: ptr(5), To: ptr(6), Unit: "Nm"},
		store.Value{Label: "Reach", Kind: store.KindInfo, From: ptr(80), Unit: "mm"})
	got := f.start("minor")
	vs := got.Steps[0].Values
	if len(vs) != 3 {
		t.Fatalf("values %+v", vs)
	}
	if vs[1].Key != "clamp" || *vs[1].From != 5 || vs[1].Hint != "" {
		t.Errorf("override: %+v", vs[1])
	}
	if vs[2].Key != "lever_travel" || vs[2].Kind != store.KindLimit || vs[2].ComponentName != "Shimano GRX" {
		t.Errorf("default: %+v", vs[2])
	}
}

func TestAddSteps(t *testing.T) {
	f := setup(t)
	f.mount("chain", "XT", "", "2026-01-01", store.Value{Label: "Wear", To: ptr(0.5), Unit: "%"})
	// Installed after the maintenance date: not part of added steps.
	f.mount("shift_brake_lever", "GRX", "", "2026-10-27")
	m := f.start("minor")
	err := maintenance.AddSteps(f.ctx, f.st, owner, m.ID, []string{f.steps["Kette messen"], f.steps["Hebel"], f.steps["Kette tauschen"]})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := f.st.Maintenances().GetMaintenance(f.ctx, owner, m.ID)
	maintenance.SortSteps(&got)
	if want := []string{"Hebel", "Kette messen", "Kette tauschen", "Probefahrt"}; !slices.Equal(stepTitles(got), want) {
		t.Fatalf("steps %v, want %v", stepTitles(got), want)
	}
	if vs := got.Steps[1].Values; len(vs) != 1 || vs[0].ComponentName != "Shimano XT" {
		t.Errorf("chain values %+v", vs)
	}
	if vs := got.Steps[0].Values; len(vs) != 1 {
		t.Errorf("lever values %+v", vs)
	}

	if err := maintenance.AddSteps(f.ctx, f.st, owner, m.ID, []string{f.steps["Hebel"]}); !errors.Is(err, maintenance.ErrNoSteps) {
		t.Errorf("only contained steps: %v", err)
	}
	if err := maintenance.AddSteps(f.ctx, f.st, owner, m.ID, []string{uuid.NewString()}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown step: %v", err)
	}
	if err := maintenance.Save(f.ctx, f.st, owner, m.ID, maintenance.Progress{}, true); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.AddSteps(f.ctx, f.st, owner, m.ID, []string{f.steps["Kette tauschen"]}); !errors.Is(err, maintenance.ErrCompleted) {
		t.Errorf("completed: %v", err)
	}
}

func TestStartModules(t *testing.T) {
	f := setup(t)
	if err := catalog.SeedSteps(f.ctx, f.st, owner); err != nil {
		t.Fatal(err)
	}
	contains := func(m store.Maintenance, builtinID string) bool {
		return slices.ContainsFunc(m.Steps, func(s store.MaintenanceStep) bool { return s.BuiltinID == builtinID })
	}
	m := f.start("minor")
	if !contains(m, "tube_valve") || contains(m, "tubeless_sealant") || contains(m, "carbon_cable_rub") {
		t.Errorf("bike without features: wrong steps")
	}
	for _, s := range m.Steps {
		if len(s.Hints) > 0 {
			t.Errorf("%s has hints %v", s.BuiltinID, s.Hints)
		}
	}

	b, _ := f.st.Bikes().GetBike(f.ctx, owner, f.bike)
	b.Features = []string{"carbon", "tubeless"}
	if err := f.st.Bikes().UpdateBike(f.ctx, b); err != nil {
		t.Fatal(err)
	}
	m = f.start("minor")
	if contains(m, "tube_valve") || !contains(m, "tubeless_sealant") || !contains(m, "carbon_cable_rub") {
		t.Errorf("carbon tubeless bike: wrong steps")
	}
	for _, s := range m.Steps {
		if s.BuiltinID == "torque_cockpit" && (!slices.Equal(s.Hints, []string{"carbon_torque"}) || s.Module != "") {
			t.Errorf("torque_cockpit %+v", s)
		}
		if s.BuiltinID == "tubeless_sealant" && s.Module != "tubeless" {
			t.Errorf("tubeless_sealant module %q", s.Module)
		}
	}
	// On-demand module steps of other features can't be added.
	b.Features = nil
	if err := f.st.Bikes().UpdateBike(f.ctx, b); err != nil {
		t.Fatal(err)
	}
	steps, _ := catalog.Sorted(f.ctx, f.st, owner)
	crash := steps[slices.IndexFunc(steps, func(s store.Step) bool { return s.BuiltinID == "carbon_crash_check" })]
	if err := maintenance.AddSteps(f.ctx, f.st, owner, m.ID, []string{crash.ID}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("module step for bike without feature: %v", err)
	}
	// The started maintenance stays as it was.
	got, _ := f.st.Maintenances().GetMaintenance(f.ctx, owner, m.ID)
	if !contains(got, "tubeless_sealant") {
		t.Error("maintenance changed with features")
	}
}

func TestSave(t *testing.T) {
	f := setup(t)
	m := f.start("minor")
	pads := m.Steps[0].Values[0]
	p := maintenance.Progress{
		Notes:    " alles gut ",
		Steps:    map[string]maintenance.StepResult{m.Steps[0].ID: {Done: true, Note: "neu"}},
		Measured: map[string]*float64{pads.ID: ptr(1.0)},
	}
	if err := maintenance.Save(f.ctx, f.st, owner, m.ID, p, false); err != nil {
		t.Fatal(err)
	}
	got, _ := f.st.Maintenances().GetMaintenance(f.ctx, owner, m.ID)
	maintenance.SortSteps(&got)
	v := got.Steps[0].Values[0]
	if got.Notes != "alles gut" || !got.Steps[0].Done || got.Steps[0].Note != "neu" || got.Steps[1].Done || !v.OutOfLimit() {
		t.Fatalf("got %+v", got)
	}

	if err := maintenance.Save(f.ctx, f.st, owner, m.ID, maintenance.Progress{}, true); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.Save(f.ctx, f.st, owner, m.ID, p, false); !errors.Is(err, maintenance.ErrCompleted) {
		t.Errorf("save completed: %v", err)
	}
	if err := maintenance.Reopen(f.ctx, f.st, owner, m.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.st.Maintenances().GetMaintenance(f.ctx, owner, m.ID); !got.Open() {
		t.Error("not reopened")
	}
}

func TestStartChecks(t *testing.T) {
	f := setup(t)
	start := func(ownerID, level string, extras ...string) error {
		_, err := maintenance.Start(f.ctx, f.st, ownerID, f.bike, level, extras, day("2026-10-26"), 0)
		return err
	}
	if err := start(owner, "weekly"); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("unknown level: %v", err)
	}
	if err := start(owner, ""); !errors.Is(err, maintenance.ErrNoSteps) {
		t.Errorf("nothing chosen: %v", err)
	}
	if err := start(owner, "pre_ride"); !errors.Is(err, maintenance.ErrNoSteps) {
		t.Errorf("level without steps: %v", err)
	}
	if err := start(owner, "", uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown step: %v", err)
	}
	if err := start("o2", "minor"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other owner: %v", err)
	}
	if err := f.st.Bikes().SetBikeRetired(f.ctx, owner, f.bike, ptrTime(day("2026-10-01")), "sold"); err != nil {
		t.Fatal(err)
	}
	if err := start(owner, "minor"); !errors.Is(err, components.ErrRetired) {
		t.Errorf("retired bike: %v", err)
	}
}

func TestChangeDate(t *testing.T) {
	f := setup(t)
	m, err := maintenance.Start(f.ctx, f.st, owner, f.bike, "minor", nil, day("2026-10-26"), 100_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := maintenance.ChangeDate(f.ctx, f.st, owner, m.ID, day("2026-10-20"), 90_000); err != nil {
		t.Fatal(err)
	}
	got, _ := f.st.Maintenances().GetMaintenance(f.ctx, owner, m.ID)
	if !got.PerformedAt.Equal(day("2026-10-20")) || got.DistanceM != 90_000 || len(got.Steps) != 2 {
		t.Errorf("got %+v", got)
	}
	if r, _ := f.st.Odometer().ListReadings(f.ctx, owner, f.bike); len(r) != 2 {
		t.Errorf("readings %+v", r)
	}

	// The odometer must fit the other readings.
	if err := maintenance.ChangeDate(f.ctx, f.st, owner, m.ID, day("2026-10-30"), 50_000); err == nil {
		t.Error("odometer going backwards accepted")
	}

	if err := maintenance.Save(f.ctx, f.st, owner, m.ID, maintenance.Progress{}, true); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.ChangeDate(f.ctx, f.st, owner, m.ID, day("2026-10-21"), 90_000); !errors.Is(err, maintenance.ErrCompleted) {
		t.Errorf("completed: %v", err)
	}
}
