package catalog_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/bikes"
	"github.com/spokebook/spokebook/internal/app/spokebook/catalog"
	"github.com/spokebook/spokebook/internal/app/spokebook/components"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite"
	"github.com/spokebook/spokebook/internal/app/spokebook/values"
)

const owner = "o1"

func ptr(f float64) *float64 { return &f }

func setup(t *testing.T) (context.Context, store.Store) {
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
	return ctx, st
}

func newStep(title, phase, assembly string) store.Step {
	return store.Step{
		ID: uuid.NewString(), OwnerID: owner, Title: title, Phase: phase, Assembly: assembly, CreatedAt: time.Now(),
	}
}

// add adds steps and returns their IDs by title.
func add(t *testing.T, ctx context.Context, st store.Store, steps ...store.Step) map[string]string {
	t.Helper()
	ids := map[string]string{}
	for _, s := range steps {
		if err := catalog.AddStep(ctx, st, s); err != nil {
			t.Fatal(err)
		}
		ids[s.Title] = s.ID
	}
	return ids
}

func titles(t *testing.T, ctx context.Context, st store.Store) []string {
	t.Helper()
	steps, err := catalog.Sorted(ctx, st, owner)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range steps {
		out = append(out, s.Title)
	}
	return out
}

func TestOrder(t *testing.T) {
	ctx, st := setup(t)
	// Added out of order: sorted by phase, assembly group (none first),
	// then the order of adding.
	ids := add(t, ctx, st,
		newStep("brakes 1", "inspection", "brakes"),
		newStep("wash", "cleaning", ""),
		newStep("frame", "inspection", "frame_fork"),
		newStep("brakes 2", "inspection", "brakes"),
		newStep("brakes 3", "inspection", "brakes"),
		newStep("notes", "inspection", ""),
	)
	want := []string{"wash", "notes", "frame", "brakes 1", "brakes 2", "brakes 3"}
	if got := titles(t, ctx, st); !slices.Equal(got, want) {
		t.Fatalf("added: %v", got)
	}

	if err := catalog.MoveStep(ctx, st, owner, ids["brakes 3"], -1); err != nil {
		t.Fatal(err)
	}
	want = []string{"wash", "notes", "frame", "brakes 1", "brakes 3", "brakes 2"}
	if got := titles(t, ctx, st); !slices.Equal(got, want) {
		t.Fatalf("brakes 3 up: %v", got)
	}
	// Steps only move within their phase and assembly group.
	if err := catalog.MoveStep(ctx, st, owner, ids["brakes 1"], -1); !errors.Is(err, catalog.ErrNoNeighbour) {
		t.Errorf("first in group up: got %v", err)
	}
	if err := catalog.MoveStep(ctx, st, owner, ids["frame"], 1); !errors.Is(err, catalog.ErrNoNeighbour) {
		t.Errorf("last in group down: got %v", err)
	}
	if err := catalog.MoveStep(ctx, st, owner, "nope", 1); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown step: got %v", err)
	}

	// Moved to another group, a step goes to its end.
	s, err := st.Steps().GetStep(ctx, owner, ids["brakes 1"])
	if err != nil {
		t.Fatal(err)
	}
	s.Assembly = "frame_fork"
	if err := catalog.UpdateStep(ctx, st, s); err != nil {
		t.Fatal(err)
	}
	want = []string{"wash", "notes", "frame", "brakes 1", "brakes 3", "brakes 2"}
	if got := titles(t, ctx, st); !slices.Equal(got, want) {
		t.Fatalf("moved to frame: %v", got)
	}
	s.Assembly = "brakes"
	if err := catalog.UpdateStep(ctx, st, s); err != nil {
		t.Fatal(err)
	}
	want = []string{"wash", "notes", "frame", "brakes 3", "brakes 2", "brakes 1"}
	if got := titles(t, ctx, st); !slices.Equal(got, want) {
		t.Fatalf("moved back: %v", got)
	}

	// Editing within the group keeps the place.
	s, _ = st.Steps().GetStep(ctx, owner, ids["brakes 3"])
	s.Title = "brakes 3b"
	if err := catalog.UpdateStep(ctx, st, s); err != nil {
		t.Fatal(err)
	}
	if err := catalog.DeleteStep(ctx, st, owner, ids["wash"]); err != nil {
		t.Fatal(err)
	}
	want = []string{"notes", "frame", "brakes 3b", "brakes 2", "brakes 1"}
	if got := titles(t, ctx, st); !slices.Equal(got, want) {
		t.Fatalf("after edit and delete: %v", got)
	}
}

func TestStepDetails(t *testing.T) {
	ctx, st := setup(t)
	s := newStep("Brakes", "inspection", "brakes")
	s.Action = "measure"
	s.Levels = []string{"major", "monthly", "major"}
	s.ComponentTypes = []string{"shift_brake_lever", "brake_pads"}
	s.ValueKinds = []string{store.KindTarget, store.KindLimit}
	s.Values = []store.Value{{Label: "Pad thickness", From: ptr(1.2), Unit: "mm"}}
	add(t, ctx, st, s)
	got, err := st.Steps().GetStep(ctx, owner, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != "inspection" || got.Assembly != "brakes" || got.Action != "measure" ||
		!slices.Equal(got.Levels, []string{"monthly", "major"}) ||
		!slices.Equal(got.ComponentTypes, []string{"brake_pads", "shift_brake_lever"}) ||
		!slices.Equal(got.ValueKinds, []string{store.KindLimit, store.KindTarget}) ||
		len(got.Values) != 1 || got.Values[0].Kind != store.KindLimit || got.Values[0].Key != "pad_thickness" {
		t.Fatalf("got %+v", got)
	}

	// Levels, types and kinds can be cleared.
	got.Levels, got.ComponentTypes, got.ValueKinds, got.Values = nil, nil, nil, nil
	if err := catalog.UpdateStep(ctx, st, got); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Steps().GetStep(ctx, owner, s.ID); len(got.Levels)+len(got.ComponentTypes)+len(got.ValueKinds)+len(got.Values) != 0 {
		t.Errorf("not cleared: %+v", got)
	}

	for name, change := range map[string]func(*store.Step){
		"no phase":       func(s *store.Step) { s.Phase = "" },
		"unknown phase":  func(s *store.Step) { s.Phase = "nope" },
		"unknown group":  func(s *store.Step) { s.Assembly = "nope" },
		"unknown action": func(s *store.Step) { s.Action = "nope" },
		"unknown level":  func(s *store.Step) { s.Levels = []string{"weekly"} },
		"unknown kind":   func(s *store.Step) { s.ValueKinds = []string{"min"} },
	} {
		bad := got
		change(&bad)
		if err := catalog.UpdateStep(ctx, st, bad); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}

	bad := got
	bad.ComponentTypes = []string{"nope"}
	if err := catalog.UpdateStep(ctx, st, bad); !errors.Is(err, components.ErrInvalidType) {
		t.Errorf("unknown type: got %v", err)
	}
	bad = got
	bad.Values = []store.Value{{Label: "x", From: ptr(2), To: ptr(1)}}
	if err := catalog.UpdateStep(ctx, st, bad); !errors.Is(err, values.ErrRange) {
		t.Errorf("bad range: got %v", err)
	}

	// A hidden type stays on a step that already has it.
	got.ComponentTypes = []string{"brake_pads"}
	if err := catalog.UpdateStep(ctx, st, got); err != nil {
		t.Fatal(err)
	}
	if err := components.DeleteType(ctx, st, owner, "brake_pads"); err != nil {
		t.Fatal(err)
	}
	got.Title = "Renamed"
	if err := catalog.UpdateStep(ctx, st, got); err != nil {
		t.Errorf("kept hidden type: %v", err)
	}
}

func TestOwner(t *testing.T) {
	ctx, st := setup(t)
	s := newStep("a", "intake", "")
	add(t, ctx, st, s)
	if _, err := st.Steps().GetStep(ctx, "o2", s.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other owner's step: %v", err)
	}
	if steps, _ := st.Steps().ListSteps(ctx, "o2"); len(steps) != 0 {
		t.Errorf("other owner sees %+v", steps)
	}
	s.OwnerID = "o2"
	if err := catalog.UpdateStep(ctx, st, s); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("update other owner's step: %v", err)
	}
	if err := catalog.DeleteStep(ctx, st, "o2", s.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("delete other owner's step: %v", err)
	}
}

// TestBuiltins checks that every built-in step uses known IDs and types.
func TestBuiltins(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range catalog.Builtins {
		if seen[b.ID] {
			t.Errorf("%s: duplicate", b.ID)
		}
		seen[b.ID] = true
		if !slices.Contains(catalog.Phases, b.Phase) || (b.Assembly != "" && !slices.Contains(catalog.Assemblies, b.Assembly)) ||
			(b.Action != "" && !slices.Contains(catalog.Actions, b.Action)) {
			t.Errorf("%s: unknown phase, assembly or action", b.ID)
		}
		for _, l := range b.Levels {
			if !slices.Contains(catalog.Levels, l) {
				t.Errorf("%s: unknown level %s", b.ID, l)
			}
		}
		for _, typ := range b.Types {
			if !components.IsBuiltin(typ) {
				t.Errorf("%s: unknown type %s", b.ID, typ)
			}
		}
		for _, k := range b.Kinds {
			if !slices.Contains(store.ValueKinds, k) {
				t.Errorf("%s: unknown kind %s", b.ID, k)
			}
		}
	}
	n := len(catalog.Builtins)
	for _, b := range catalog.Builtins {
		if b.Module != "" {
			n--
			if !slices.Contains(bikes.Features, b.Module) {
				t.Errorf("%s: unknown module %s", b.ID, b.Module)
			}
		}
		if b.Replaces != "" && !slices.ContainsFunc(catalog.Builtins, func(o catalog.Builtin) bool { return o.ID == b.Replaces && o.Module == "" }) {
			t.Errorf("%s: replaces unknown base step %s", b.ID, b.Replaces)
		}
	}
	base := func(id string) bool {
		return slices.ContainsFunc(catalog.Builtins, func(o catalog.Builtin) bool { return o.ID == id && o.Module == "" })
	}
	for _, d := range catalog.Drops {
		if !slices.Contains(bikes.Features, d.Feature) || !base(d.Step) {
			t.Errorf("drop %v: unknown feature or base step", d)
		}
	}
	for _, h := range catalog.Hints {
		if !slices.Contains(bikes.Features, h.Feature) || (h.Step != "" && !slices.ContainsFunc(catalog.Builtins, func(o catalog.Builtin) bool { return o.ID == h.Step })) {
			t.Errorf("hint %v: unknown feature or step", h)
		}
	}
	if n != 53 {
		t.Errorf("%d built-in base steps, want 53", n)
	}
}

func TestSeedSteps(t *testing.T) {
	ctx, st := setup(t)
	own := newStep("own", "inspection", "drivetrain")
	add(t, ctx, st, own)
	if err := catalog.SeedSteps(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	steps, err := catalog.Sorted(ctx, st, owner)
	if err != nil {
		t.Fatal(err)
	}
	want := len(catalog.Builtins) + 1
	if len(steps) != want {
		t.Fatalf("%d steps, want %d", len(steps), want)
	}
	// Built-in steps come after the own step of their group, in the order
	// of Builtins (base steps before module steps), with empty texts to be
	// translated.
	var group []string
	for _, s := range steps {
		if s.Phase == "inspection" && s.Assembly == "drivetrain" {
			group = append(group, s.BuiltinID)
		}
	}
	if want := []string{"", "chain_wear", "cassette_chainrings", "shift_cables", "derailleur_pulleys", "hub_gear_check"}; len(group) < len(want) || !slices.Equal(group[:len(want)], want) {
		t.Errorf("drivetrain inspection %v, want %v", group, want)
	}
	i := slices.IndexFunc(steps, func(s store.Step) bool { return s.BuiltinID == "chain_wear" })
	if s := steps[i]; s.Title != "" || !slices.Equal(s.ComponentTypes, []string{"chain"}) ||
		!slices.Equal(s.ValueKinds, []string{store.KindLimit}) || !slices.Equal(s.Levels, []string{"monthly", "minor", "major"}) {
		t.Errorf("chain_wear %+v", s)
	}

	// Deleting hides a built-in step; seeding again neither duplicates
	// steps nor brings hidden ones back. Own steps are really deleted.
	if err := catalog.DeleteStep(ctx, st, owner, steps[i].ID); err != nil {
		t.Fatal(err)
	}
	if err := catalog.DeleteStep(ctx, st, owner, own.ID); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SeedSteps(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	if steps, _ := catalog.Sorted(ctx, st, owner); len(steps) != want-2 {
		t.Errorf("%d visible steps after seeding again, want %d", len(steps), want-2)
	}
	if all, _ := st.Steps().ListSteps(ctx, owner); len(all) != want-1 {
		t.Errorf("%d stored steps, want %d", len(all), want-1)
	}
}

func TestModules(t *testing.T) {
	ctx, st := setup(t)
	if err := catalog.SeedSteps(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	steps, err := catalog.Sorted(ctx, st, owner)
	if err != nil {
		t.Fatal(err)
	}
	byBuiltin := map[string]store.Step{}
	for _, s := range steps {
		byBuiltin[s.BuiltinID] = s
	}
	if r := byBuiltin["tubeless_sealant"].Replaces; r == "" || r != byBuiltin["tube_valve"].ID {
		t.Errorf("tubeless_sealant replaces %q, want the ID of tube_valve", r)
	}
	has := func(steps []store.Step, id string) bool {
		return slices.ContainsFunc(steps, func(s store.Step) bool { return s.BuiltinID == id })
	}
	plain := catalog.Applicable(steps, nil)
	if !has(plain, "tube_valve") || has(plain, "tubeless_sealant") || has(plain, "carbon_hotspots") {
		t.Error("bike without features gets module steps")
	}
	tubeless := catalog.Applicable(steps, []string{"tubeless"})
	if has(tubeless, "tube_valve") || !has(tubeless, "tubeless_sealant") || has(tubeless, "carbon_hotspots") {
		t.Error("tubeless bike: wrong steps")
	}
	if len(steps) != len(byBuiltin) {
		t.Error("Applicable changed its input")
	}

	// Modules drop steps without replacing them; modules combine.
	drive := catalog.Applicable(steps, []string{"hub_gear", "belt", "electronic_shifting"})
	for _, id := range []string{"adjust_front_derailleur", "derailleur_pulleys", "torque_derailleurs", "adjust_derailleur", "chain_wear", "lubricate_chain", "shift_cables", "replace_cables"} {
		if has(drive, id) {
			t.Errorf("hub gear, belt and electronic shifting keep %s", id)
		}
	}
	for _, id := range []string{"hub_gear_adjust", "belt_wear", "belt_tension", "eshift_battery", "shift_test"} {
		if !has(drive, id) {
			t.Errorf("hub gear, belt and electronic shifting lack %s", id)
		}
	}
	if !has(plain, "adjust_front_derailleur") || !has(catalog.Applicable(steps, []string{"belt"}), "adjust_front_derailleur") {
		t.Error("adjust_front_derailleur dropped without hub gear")
	}
	if got := catalog.DroppedBy("adjust_front_derailleur"); !slices.Equal(got, []string{"hub_gear"}) {
		t.Errorf("DroppedBy(adjust_front_derailleur) = %v", got)
	}
	if got := byBuiltin["hub_gear_oil"].Interval; got.Months == nil || *got.Months != 12 || got.DistanceM == nil || *got.DistanceM != 5_000_000 {
		t.Errorf("hub_gear_oil interval %+v", got)
	}

	// Hints: carbon on all tighten steps, tubeless on one step.
	torque := byBuiltin["torque_cockpit"]
	if got := catalog.HintsFor(torque, []string{"carbon", "tubeless"}); !slices.Equal(got, []string{"carbon_torque"}) {
		t.Errorf("hints for torque_cockpit %v", got)
	}
	if got := catalog.HintsFor(torque, []string{"tubeless"}); len(got) != 0 {
		t.Errorf("hints without carbon %v", got)
	}
	axles := byBuiltin["axles_tire_pressure"]
	if got := catalog.HintsFor(axles, []string{"carbon", "tubeless"}); !slices.Equal(got, []string{"carbon_torque", "tubeless_pressure"}) {
		t.Errorf("hints for axles_tire_pressure %v", got)
	}

	// Own steps can belong to a module; unknown modules are rejected.
	own := newStep("own carbon", "inspection", "")
	own.Module = "carbon"
	add(t, ctx, st, own)
	own.Module = "steel"
	if err := catalog.UpdateStep(ctx, st, own); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("unknown module: %v", err)
	}
}
