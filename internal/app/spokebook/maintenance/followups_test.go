package maintenance_test

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/maintenance"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// followUps adds two steps that always follow each other and links the
// chain steps: measuring leads to replacing on a finding, replacing always
// to adjusting.
func (f *fixture) followUps() {
	f.t.Helper()
	for _, s := range []store.Step{
		{Title: "Schaltung einstellen", Phase: "adjustment", Assembly: "drivetrain"},
		{Title: "Endkontrolle", Phase: "final_check", Assembly: "drivetrain"},
	} {
		s.ID, s.OwnerID, s.CreatedAt = uuid.NewString(), owner, time.Now()
		if err := catalog.AddStep(f.ctx, f.st, s); err != nil {
			f.t.Fatal(err)
		}
		f.steps[s.Title] = s.ID
	}
	link := func(from, to, kind string) {
		s, err := f.st.Steps().GetStep(f.ctx, owner, f.steps[from])
		if err != nil {
			f.t.Fatal(err)
		}
		s.FollowUps = append(s.FollowUps, store.FollowUp{StepID: f.steps[to], Kind: kind})
		if err := catalog.UpdateStep(f.ctx, f.st, s); err != nil {
			f.t.Fatal(err)
		}
	}
	link("Kette messen", "Kette tauschen", store.FollowUpFinding)
	link("Kette tauschen", "Schaltung einstellen", store.FollowUpAlways)
	link("Schaltung einstellen", "Endkontrolle", store.FollowUpAlways)
	link("Endkontrolle", "Schaltung einstellen", store.FollowUpAlways)
}

// origins returns per step title the title of its origin ("" for none).
func origins(m store.Maintenance) map[string]string {
	out := map[string]string{}
	for _, s := range m.Steps {
		out[s.Title] = ""
		if i := slices.IndexFunc(m.Steps, func(o store.MaintenanceStep) bool { return o.ID == s.OriginID }); i >= 0 {
			out[s.Title] = m.Steps[i].Title
		}
	}
	return out
}

func (f *fixture) get(id string) store.Maintenance {
	f.t.Helper()
	m, err := f.st.Maintenances().GetMaintenance(f.ctx, owner, id)
	if err != nil {
		f.t.Fatal(err)
	}
	maintenance.SortSteps(&m)
	return m
}

func TestAlwaysFollowUps(t *testing.T) {
	f := setup(t)
	f.followUps()

	// The steps that always follow come along, also in a cycle, once.
	m := f.start("", "Kette tauschen")
	want := map[string]string{"Kette tauschen": "", "Schaltung einstellen": "Kette tauschen", "Endkontrolle": "Schaltung einstellen"}
	if got := origins(m); !maps.Equal(got, want) {
		t.Errorf("start: %v", got)
	}

	// Adding a step brings them too, without those already there.
	m = f.start("minor")
	if err := maintenance.AddSteps(f.ctx, f.st, owner, m.ID, []string{f.steps["Kette tauschen"]}); err != nil {
		t.Fatal(err)
	}
	if got := stepTitles(f.get(m.ID)); !slices.Equal(got, []string{"Hebel", "Kette tauschen", "Schaltung einstellen", "Endkontrolle", "Probefahrt"}) {
		t.Errorf("added: %v", got)
	}
}

func TestAddFollowUp(t *testing.T) {
	f := setup(t)
	f.followUps()
	m := f.start("monthly")
	measure := m.Steps[0].ID

	offers, err := maintenance.Offers(f.ctx, f.st, owner, m)
	if err != nil || len(offers) != 1 || len(offers[measure]) != 1 || offers[measure][0].ID != f.steps["Kette tauschen"] {
		t.Fatalf("offers %v, %v", offers, err)
	}
	for name, tc := range map[string]struct{ origin, step string }{
		"not offered":    {measure, f.steps["Probefahrt"]},
		"unknown origin": {"nope", f.steps["Kette tauschen"]},
	} {
		if err := maintenance.AddFollowUp(f.ctx, f.st, owner, m.ID, tc.origin, tc.step); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}

	if err := maintenance.AddFollowUp(f.ctx, f.st, owner, m.ID, measure, f.steps["Kette tauschen"]); err != nil {
		t.Fatal(err)
	}
	// Twice changes nothing.
	if err := maintenance.AddFollowUp(f.ctx, f.st, owner, m.ID, measure, f.steps["Kette tauschen"]); err != nil {
		t.Fatal(err)
	}
	m = f.get(m.ID)
	want := map[string]string{
		"Kette messen": "", "Kette tauschen": "Kette messen", "Schaltung einstellen": "Kette tauschen", "Endkontrolle": "Schaltung einstellen",
	}
	if got := origins(m); !maps.Equal(got, want) || len(m.Steps) != 4 {
		t.Errorf("after adding: %v", got)
	}
	if offers, err := maintenance.Offers(f.ctx, f.st, owner, m); err != nil || len(offers) != 0 {
		t.Errorf("offers after adding: %v, %v", offers, err)
	}

	// Completed, nothing can be added; an undone follow-up can be
	// dismissed, other steps can't.
	if err := maintenance.Dismiss(f.ctx, f.st, owner, m.ID, m.Steps[1].ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("dismiss in open maintenance: %v", err)
	}
	if err := maintenance.Save(f.ctx, f.st, owner, m.ID, maintenance.Progress{}, true); err != nil {
		t.Fatal(err)
	}
	m = f.get(m.ID)
	if err := maintenance.AddFollowUp(f.ctx, f.st, owner, m.ID, measure, f.steps["Kette tauschen"]); !errors.Is(err, maintenance.ErrCompleted) {
		t.Errorf("add to completed: %v", err)
	}
	if offers, err := maintenance.Offers(f.ctx, f.st, owner, m); err != nil || offers != nil {
		t.Errorf("offers when completed: %v, %v", offers, err)
	}
	if err := maintenance.Dismiss(f.ctx, f.st, owner, m.ID, measure); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("dismiss a step that is no follow-up: %v", err)
	}
	replace := m.Steps[slices.IndexFunc(m.Steps, func(s store.MaintenanceStep) bool { return s.Title == "Kette tauschen" })].ID
	if err := maintenance.Dismiss(f.ctx, f.st, owner, m.ID, replace); err != nil {
		t.Fatal(err)
	}
	if m = f.get(m.ID); !m.Steps[slices.IndexFunc(m.Steps, func(s store.MaintenanceStep) bool { return s.ID == replace })].Dismissed {
		t.Error("not dismissed")
	}
}

func TestRemove(t *testing.T) {
	f := setup(t)
	f.followUps()
	m := f.start("monthly")
	measure := m.Steps[0].ID
	if err := maintenance.AddFollowUp(f.ctx, f.st, owner, m.ID, measure, f.steps["Kette tauschen"]); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.AddSteps(f.ctx, f.st, owner, m.ID, []string{f.steps["Hebel"]}); err != nil {
		t.Fatal(err)
	}
	m = f.get(m.ID)
	id := func(title string) string {
		return m.Steps[slices.IndexFunc(m.Steps, func(s store.MaintenanceStep) bool { return s.Title == title })].ID
	}
	// Steps added later are marked, those that came with them are follow-ups.
	removable := map[string]bool{}
	for _, s := range m.Steps {
		removable[s.Title] = maintenance.Removable(s)
	}
	want := map[string]bool{"Kette messen": false, "Hebel": true, "Kette tauschen": true, "Schaltung einstellen": true, "Endkontrolle": true}
	if !maps.Equal(removable, want) {
		t.Errorf("removable %v", removable)
	}

	// Neither a step of the level nor a done one can be removed.
	if err := maintenance.Remove(f.ctx, f.st, owner, m.ID, measure); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("level step: %v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(maintenance.Save(f.ctx, f.st, owner, m.ID, maintenance.Progress{Steps: map[string]maintenance.StepResult{id("Endkontrolle"): {Done: true}}}, false))
	if err := maintenance.Remove(f.ctx, f.st, owner, m.ID, id("Endkontrolle")); !errors.Is(err, maintenance.ErrStepDone) {
		t.Errorf("done step: %v", err)
	}

	// Removing the replacement takes along what it brought, except done
	// steps; it is offered again.
	must(maintenance.Remove(f.ctx, f.st, owner, m.ID, id("Kette tauschen")))
	must(maintenance.Remove(f.ctx, f.st, owner, m.ID, id("Hebel")))
	m = f.get(m.ID)
	if got := stepTitles(m); !slices.Equal(got, []string{"Kette messen", "Endkontrolle"}) {
		t.Errorf("after removing: %v", got)
	}
	if offers, err := maintenance.Offers(f.ctx, f.st, owner, m); err != nil || len(offers[measure]) != 1 {
		t.Errorf("offers after removing: %v, %v", offers, err)
	}

	must(maintenance.Save(f.ctx, f.st, owner, m.ID, maintenance.Progress{}, true))
	if err := maintenance.Remove(f.ctx, f.st, owner, m.ID, id("Endkontrolle")); !errors.Is(err, maintenance.ErrCompleted) {
		t.Errorf("completed: %v", err)
	}
}
