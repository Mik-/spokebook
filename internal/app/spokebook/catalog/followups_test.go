package catalog_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

func TestFollowUpChecks(t *testing.T) {
	ctx, st := setup(t)
	pads := newStep("Replace pads", "replacement", "brakes")
	add(t, ctx, st, pads)
	foreign := newStep("Foreign", "replacement", "brakes")
	foreign.OwnerID = "o2"
	if err := catalog.AddStep(ctx, st, foreign); err != nil {
		t.Fatal(err)
	}

	check := newStep("Check pads", "inspection", "brakes")
	for name, f := range map[string]store.FollowUp{
		"itself":       {StepID: check.ID, Kind: store.FollowUpFinding},
		"unknown kind": {StepID: pads.ID, Kind: "sometimes"},
		"no step":      {Kind: store.FollowUpFinding},
		"unknown step": {StepID: "nope", Kind: store.FollowUpFinding},
		"other owner":  {StepID: foreign.ID, Kind: store.FollowUpFinding},
	} {
		s := check
		s.FollowUps = []store.FollowUp{f}
		if err := catalog.AddStep(ctx, st, s); !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// A repeated follow-up is kept once, the first kind wins.
	check.FollowUps = []store.FollowUp{{StepID: pads.ID, Kind: store.FollowUpFinding}, {StepID: pads.ID, Kind: store.FollowUpAlways}}
	add(t, ctx, st, check)
	got, err := st.Steps().GetStep(ctx, owner, check.ID)
	if err != nil || len(got.FollowUps) != 1 || got.FollowUps[0].Kind != store.FollowUpFinding {
		t.Fatalf("got %+v, %v", got.FollowUps, err)
	}
}

func TestBuiltinFollowUps(t *testing.T) {
	for _, b := range catalog.Builtins {
		for _, id := range slices.Concat(b.Findings, b.Always) {
			if !slices.ContainsFunc(catalog.Builtins, func(o catalog.Builtin) bool { return o.ID == id }) || id == b.ID {
				t.Errorf("%s: unknown follow-up %q", b.ID, id)
			}
		}
	}

	ctx, st := setup(t)
	if err := catalog.SeedSteps(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	steps, err := catalog.Sorted(ctx, st, owner)
	if err != nil {
		t.Fatal(err)
	}
	byBuiltin := func(steps []store.Step, id string) store.Step {
		t.Helper()
		i := slices.IndexFunc(steps, func(s store.Step) bool { return s.BuiltinID == id })
		if i < 0 {
			t.Fatalf("no step %s", id)
		}
		return steps[i]
	}
	builtins := func(steps []store.Step) []string {
		var out []string
		for _, s := range steps {
			out = append(out, s.BuiltinID)
		}
		return out
	}
	follow := func(features []string, from, kind string) []string {
		applicable := catalog.Applicable(steps, features)
		return builtins(catalog.FollowUps(applicable, byBuiltin(steps, from), kind))
	}

	// Seeding links the built-in steps.
	if got := follow(nil, "chain_wear", store.FollowUpFinding); !slices.Equal(got, []string{"replace_chain"}) {
		t.Errorf("chain wear: %v", got)
	}
	if got := follow(nil, "chain_wear", store.FollowUpAlways); len(got) != 0 {
		t.Errorf("chain wear always: %v", got)
	}
	if got := follow(nil, "replace_chain", store.FollowUpAlways); !slices.Equal(got, []string{"adjust_derailleur"}) {
		t.Errorf("replace chain: %v", got)
	}
	// A follow-up replaced by a module step becomes that step; one of a
	// module the bike lacks is left out.
	if got := follow([]string{"hub_gear"}, "replace_chain", store.FollowUpAlways); !slices.Equal(got, []string{"hub_gear_adjust"}) {
		t.Errorf("hub gear: %v", got)
	}
	if got := follow([]string{"belt"}, "chain_wear", store.FollowUpFinding); !slices.Equal(got, []string{"replace_belt"}) {
		t.Errorf("belt: %v", got)
	}
	if got := follow(nil, "belt_wear", store.FollowUpFinding); len(got) != 0 {
		t.Errorf("belt wear without belt: %v", got)
	}

	// A hidden follow-up is left out.
	if err := catalog.DeleteStep(ctx, st, owner, byBuiltin(steps, "adjust_derailleur").ID); err != nil {
		t.Fatal(err)
	}
	if steps, err = catalog.Sorted(ctx, st, owner); err != nil {
		t.Fatal(err)
	}
	if got := follow(nil, "replace_chain", store.FollowUpAlways); len(got) != 0 {
		t.Errorf("hidden follow-up: %v", got)
	}
}
