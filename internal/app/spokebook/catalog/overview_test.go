package catalog_test

import (
	"slices"
	"testing"

	"github.com/spokebook/spokebook/internal/app/spokebook/catalog"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

func TestFilter(t *testing.T) {
	chain := store.Step{Title: "Chain wear", Instructions: "Measure with the Gauge", Action: "measure", Assembly: "drivetrain", Levels: []string{"monthly", "major"}}
	belt := store.Step{Title: "Belt wear", Action: "check", Assembly: "drivetrain", Module: "belt", Levels: []string{"major"}}
	wash := store.Step{Title: "Wash", Action: "clean"}
	for _, tc := range []struct {
		name string
		f    catalog.Filter
		want []string
	}{
		{"empty", catalog.Filter{}, []string{"Chain wear", "Belt wear", "Wash"}},
		{"level", catalog.Filter{Level: "monthly"}, []string{"Chain wear"}},
		{"major", catalog.Filter{Level: "major"}, []string{"Chain wear", "Belt wear"}},
		{"on demand", catalog.Filter{Level: catalog.OnDemand}, []string{"Wash"}},
		{"module", catalog.Filter{Module: "belt"}, []string{"Belt wear"}},
		{"no module", catalog.Filter{Module: catalog.NoModule}, []string{"Chain wear", "Wash"}},
		{"assembly", catalog.Filter{Assembly: "drivetrain"}, []string{"Chain wear", "Belt wear"}},
		{"action", catalog.Filter{Action: "clean"}, []string{"Wash"}},
		{"title", catalog.Filter{Query: " BELT "}, []string{"Belt wear"}},
		{"instructions", catalog.Filter{Query: "gauge"}, []string{"Chain wear"}},
		{"combined", catalog.Filter{Assembly: "drivetrain", Level: "monthly", Query: "wear"}, []string{"Chain wear"}},
		{"nothing", catalog.Filter{Level: "pre_ride"}, nil},
	} {
		var got []string
		for _, s := range []store.Step{chain, belt, wash} {
			if tc.f.Match(s) {
				got = append(got, s.Title)
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if (catalog.Filter{}).Active() || !(catalog.Filter{Query: "x"}).Active() {
		t.Error("Active")
	}
}

func TestModuleEffects(t *testing.T) {
	var steps []store.Step
	for _, b := range catalog.Builtins {
		steps = append(steps, store.Step{ID: b.ID, BuiltinID: b.ID, Action: b.Action, Module: b.Module, Replaces: b.Replaces})
	}
	ids := func(steps []store.Step) []string {
		var out []string
		for _, s := range steps {
			out = append(out, s.ID)
		}
		return out
	}

	e := catalog.ModuleEffects(steps, "belt")
	if got := ids(e.Added); !slices.Equal(got, []string{"belt_clean", "belt_wear", "belt_sprockets", "replace_belt", "belt_tension"}) {
		t.Errorf("belt added: %v", got)
	}
	if len(e.Dropped) != 0 || len(e.Hinted) != 0 {
		t.Errorf("belt: %v dropped, %v hinted", e.Dropped, e.Hinted)
	}

	e = catalog.ModuleEffects(steps, "hub_gear")
	if got := ids(e.Dropped); !slices.Equal(got, []string{"derailleur_hanger", "derailleur_pulleys", "adjust_front_derailleur", "torque_derailleurs"}) {
		t.Errorf("hub gear dropped: %v", got)
	}

	// A hint with two steps is one entry; one with an action applies to all steps of it.
	e = catalog.ModuleEffects(steps, "electronic_shifting")
	if len(e.Hinted) != 1 || e.Hinted[0].ID != "eshift_adjust" || !slices.Equal(ids(e.Hinted[0].Steps), []string{"adjust_derailleur", "adjust_front_derailleur"}) {
		t.Errorf("eshift hints: %v", e.Hinted)
	}
	e = catalog.ModuleEffects(steps, "carbon")
	if len(e.Hinted) != 1 || e.Hinted[0].ID != "carbon_torque" || !slices.Contains(ids(e.Hinted[0].Steps), "torque_saddle") {
		t.Errorf("carbon hints: %v", e.Hinted)
	}
}
