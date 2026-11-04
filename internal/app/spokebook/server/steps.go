package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/bikes"
	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// stepView is a catalog step with the display names of its component
// types, the title of the step it replaces, the steps that replace it, the
// module hints that can apply to it, the features that drop it, its
// follow-ups by kind, the steps it follows and whether it is first or last
// in its assembly group.
type stepView struct {
	store.Step
	TypeNames     string
	ReplacesTitle string
	ReplacedBy    []store.Step
	Hints         []string
	DroppedBy     string
	Findings      []store.Step
	Always        []store.Step
	FollowedBy    []followRef
	First, Last   bool
}

// followRef is a step that leads to another one, with the kind.
type followRef struct {
	Step store.Step
	Kind string
}

// FindingTitles and AlwaysTitles join the titles of the follow-ups.
func (v stepView) FindingTitles() string { return joinTitles(v.Findings) }

func (v stepView) AlwaysTitles() string { return joinTitles(v.Always) }

func joinTitles(steps []store.Step) string {
	titles := make([]string, len(steps))
	for i, s := range steps {
		titles[i] = s.Title
	}
	return strings.Join(titles, ", ")
}

// stepGroup is an assembly group of a phase (Assembly empty for steps
// without one); phaseView a phase with its non-empty groups, shown
// expanded if Open.
type (
	stepGroup struct {
		Assembly string
		Steps    []stepView
	}
	phaseView struct {
		Phase  string
		Groups []stepGroup
		Open   bool
	}
)

// Count returns the number of steps in the phase.
func (p phaseView) Count() int {
	n := 0
	for _, g := range p.Groups {
		n += len(g.Steps)
	}
	return n
}

// levelCount is the number of steps of a level.
type levelCount struct {
	Level string
	N     int
}

// LevelCounts returns the number of steps per level, for the levels that
// have steps.
func (p phaseView) LevelCounts() []levelCount {
	var out []levelCount
	for _, l := range catalog.Levels {
		n := 0
		for _, g := range p.Groups {
			for _, st := range g.Steps {
				if slices.Contains(st.Levels, l) {
					n++
				}
			}
		}
		if n > 0 {
			out = append(out, levelCount{Level: l, N: n})
		}
	}
	return out
}

// builtinTexts returns title and instructions of a step, translated for a
// built-in step (builtinID set) while they are empty.
func builtinTexts(l *i18n.Localizer, builtinID, title, instructions string) (string, string) {
	if builtinID == "" {
		return title, instructions
	}
	if title == "" {
		title = l.T("catalog." + builtinID + ".title")
	}
	if instructions == "" {
		instructions = l.T("catalog." + builtinID + ".instructions")
	}
	return title, instructions
}

// localizeStep fills in the texts of a built-in step, see builtinTexts.
func localizeStep(l *i18n.Localizer, st *store.Step) {
	st.Title, st.Instructions = builtinTexts(l, st.BuiltinID, st.Title, st.Instructions)
}

// localizeSteps calls localizeStep on each step.
func localizeSteps(l *i18n.Localizer, steps []store.Step) {
	for i := range steps {
		localizeStep(l, &steps[i])
	}
}

// groupSteps groups sorted steps by phase and assembly group.
func groupSteps(steps []stepView) []phaseView {
	var out []phaseView
	for _, st := range steps {
		if n := len(out); n == 0 || out[n-1].Phase != st.Phase {
			out = append(out, phaseView{Phase: st.Phase})
		}
		p := &out[len(out)-1]
		if n := len(p.Groups); n == 0 || p.Groups[n-1].Assembly != st.Assembly {
			p.Groups = append(p.Groups, stepGroup{Assembly: st.Assembly})
		}
		g := &p.Groups[len(p.Groups)-1]
		g.Steps = append(g.Steps, st)
	}
	for i := range out {
		for j := range out[i].Groups {
			g := out[i].Groups[j].Steps
			g[0].First, g[len(g)-1].Last = true, true
		}
	}
	return out
}

// featureNames joins the translated names of the features.
func featureNames(l *i18n.Localizer, features []string) string {
	names := make([]string, len(features))
	for i, f := range features {
		names[i] = l.T("feature." + f)
	}
	return strings.Join(names, ", ")
}

// stepViews returns the catalog in display order with texts translated.
func (s *Server) stepViews(ctx context.Context) ([]stepView, error) {
	steps, err := catalog.Sorted(ctx, s.store, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	typeNames, _, err := s.componentTypes(ctx)
	if err != nil {
		return nil, err
	}
	l := i18n.FromContext(ctx)
	localizeSteps(l, steps)
	titles := make(map[string]string, len(steps))
	byID := make(map[string]store.Step, len(steps))
	replacedBy := map[string][]store.Step{}
	followedBy := map[string][]followRef{}
	for _, st := range steps {
		titles[st.ID] = st.Title
		byID[st.ID] = st
		if st.Replaces != "" {
			replacedBy[st.Replaces] = append(replacedBy[st.Replaces], st)
		}
		for _, f := range st.FollowUps {
			followedBy[f.StepID] = append(followedBy[f.StepID], followRef{Step: st, Kind: f.Kind})
		}
	}
	views := make([]stepView, 0, len(steps))
	for _, st := range steps {
		names := make([]string, 0, len(st.ComponentTypes))
		for _, t := range st.ComponentTypes {
			names = append(names, typeNames[t])
		}
		slices.SortFunc(names, l.Compare)
		v := stepView{
			Step: st, TypeNames: strings.Join(names, ", "), ReplacesTitle: titles[st.Replaces], ReplacedBy: replacedBy[st.ID],
			Hints: catalog.HintsFor(st, bikes.Features), DroppedBy: featureNames(l, catalog.DroppedBy(st.BuiltinID)),
			FollowedBy: followedBy[st.ID],
		}
		// Hidden follow-ups are left out.
		for _, f := range st.FollowUps {
			if target, ok := byID[f.StepID]; ok && f.Kind == store.FollowUpAlways {
				v.Always = append(v.Always, target)
			} else if ok {
				v.Findings = append(v.Findings, target)
			}
		}
		views = append(views, v)
	}
	return views, nil
}

// stepFilter reads the catalog filter from the query.
func stepFilter(r *http.Request) catalog.Filter {
	q := r.URL.Query()
	return catalog.Filter{
		Level: q.Get("level"), Module: q.Get("module"), Assembly: q.Get("assembly"), Action: q.Get("action"),
		Query: strings.TrimSpace(q.Get("q")),
	}
}

func (s *Server) listSteps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	views, err := s.stepViews(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	filter := stepFilter(r)
	if filter.Active() {
		views = slices.DeleteFunc(views, func(v stepView) bool { return !filter.Match(v.Step) })
	}
	hidden, err := catalog.Hidden(ctx, s.store, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	localizeSteps(i18n.FromContext(ctx), hidden)
	phases := groupSteps(views)
	// With a filter every phase with matches is expanded; otherwise the
	// phase of the step just created, edited, moved or restored.
	target := r.URL.Query().Get("step")
	for i := range phases {
		phases[i].Open = filter.Active()
		for _, g := range phases[i].Groups {
			if slices.ContainsFunc(g.Steps, func(st stepView) bool { return st.ID == target }) {
				phases[i].Open = true
			}
		}
	}
	s.render(w, r, http.StatusOK, "steps.html", map[string]any{
		"Phases": phases, "Hidden": hidden, "Filter": filter, "Filtered": filter.Active(),
		"Assemblies": catalog.Assemblies, "Actions": catalog.Actions, "Modules": bikes.Features, "Levels": catalog.Levels,
		"OnDemand": catalog.OnDemand, "NoModule": catalog.NoModule,
	})
}

// showStep shows all that defines a step on one page.
func (s *Server) showStep(w http.ResponseWriter, r *http.Request) {
	views, err := s.stepViews(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	i := slices.IndexFunc(views, func(v stepView) bool { return v.ID == r.PathValue("id") })
	if i < 0 {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	s.render(w, r, http.StatusOK, "step.html", map[string]any{"Step": views[i], "Levels": catalog.Levels})
}

// moduleView is what a module (bike feature) changes in the catalog.
type moduleView struct {
	Feature string
	catalog.Effects
}

// listModules shows for each module which steps it adds, drops and
// annotates.
func (s *Server) listModules(w http.ResponseWriter, r *http.Request) {
	views, err := s.stepViews(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	steps := make([]store.Step, len(views))
	titles := make(map[string]string, len(views))
	for i, v := range views {
		steps[i] = v.Step
		titles[v.ID] = v.Title
	}
	mods := make([]moduleView, len(bikes.Features))
	for i, f := range bikes.Features {
		mods[i] = moduleView{Feature: f, Effects: catalog.ModuleEffects(steps, f)}
	}
	s.render(w, r, http.StatusOK, "step_modules.html", map[string]any{"Modules": mods, "Titles": titles, "Levels": catalog.Levels})
}

func (s *Server) restoreStep(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := catalog.RestoreStep(r.Context(), s.store, ownerID(r.Context()), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, stepURL(id), http.StatusSeeOther)
}

// stepForm prefills the step form with st.
func stepForm(st store.Step) url.Values {
	form := valueForm(url.Values{
		"title":           {st.Title},
		"instructions":    {st.Instructions},
		"phase":           {st.Phase},
		"assembly":        {st.Assembly},
		"action":          {st.Action},
		"module":          {st.Module},
		"level":           st.Levels,
		"component_type":  st.ComponentTypes,
		"component_kind":  st.ValueKinds,
		"interval_months": {optValue(st.Interval.Months, monthsValue)},
		"interval_km":     {optValue(st.Interval.DistanceM, kmValue)},
	}, st.Values)
	for _, f := range st.FollowUps {
		form["follow_up_step"] = append(form["follow_up_step"], f.StepID)
		form["follow_up_kind"] = append(form["follow_up_kind"], f.Kind)
	}
	return form
}

// renderStepForm shows the form for a new step (st.ID empty) or for
// editing st. form is nil to keep the submitted values.
func (s *Server) renderStepForm(w http.ResponseWriter, r *http.Request, status int, st store.Step, form url.Values, errMsg string) {
	ctx := r.Context()
	names, active, err := s.componentTypes(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Types the step keeps although they are hidden by now.
	for _, t := range st.ComponentTypes {
		if !slices.ContainsFunc(active, func(o typeOption) bool { return o.ID == t }) {
			active = append(active, typeOption{ID: t, Name: names[t]})
		}
	}
	l := i18n.FromContext(ctx)
	slices.SortFunc(active, func(a, b typeOption) int { return l.Compare(a.Name, b.Name) })
	data, err := s.valueSuggestions(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	options, err := s.stepOptions(ctx, st.FollowUps)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data["StepOptions"] = options
	localizeStep(l, &st)
	data["Step"] = st
	data["Types"] = active
	data["Phases"] = catalog.Phases
	data["Assemblies"] = catalog.Assemblies
	data["Actions"] = catalog.Actions
	data["Modules"] = bikes.Features
	data["Levels"] = catalog.Levels
	data["Error"] = errMsg
	if form != nil {
		data["Form"] = form
	}
	page := "step_edit.html"
	if st.ID == "" {
		page = "step_new.html"
	}
	s.render(w, r, status, page, data)
}

// stepOption is a phase with its steps, offered as follow-ups.
type stepOption struct {
	Phase string
	Steps []store.Step
}

// stepOptions returns the catalog by phase for choosing follow-ups, with
// the hidden steps among kept (the current follow-ups).
func (s *Server) stepOptions(ctx context.Context, kept []store.FollowUp) ([]stepOption, error) {
	steps, err := catalog.Sorted(ctx, s.store, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	hidden, err := catalog.Hidden(ctx, s.store, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	for _, h := range hidden {
		if slices.ContainsFunc(kept, func(f store.FollowUp) bool { return f.StepID == h.ID }) {
			steps = append(steps, h)
		}
	}
	slices.SortStableFunc(steps, catalog.Compare)
	localizeSteps(i18n.FromContext(ctx), steps)
	var out []stepOption
	for _, st := range steps {
		if n := len(out); n == 0 || out[n-1].Phase != st.Phase {
			out = append(out, stepOption{Phase: st.Phase})
		}
		out[len(out)-1].Steps = append(out[len(out)-1].Steps, st)
	}
	return out, nil
}

// followUpRow is one row of follow-up fields in the step form.
type followUpRow struct{ Step, Kind string }

// blankFollowUpRows is the number of empty follow-up rows in the form.
const blankFollowUpRows = 2

// followUpRows returns the follow-up rows of form plus blank ones.
func followUpRows(form url.Values) []followUpRow {
	ids := form["follow_up_step"]
	rows := make([]followUpRow, 0, len(ids)+blankFollowUpRows)
	for i, id := range ids {
		if id != "" {
			rows = append(rows, followUpRow{Step: id, Kind: nth(form["follow_up_kind"], i)})
		}
	}
	for range blankFollowUpRows {
		rows = append(rows, followUpRow{})
	}
	return rows
}

// parseStepForm sets the form fields on st. It returns the message ID of a
// validation error.
func parseStepForm(r *http.Request, st *store.Step) string {
	st.Title = strings.TrimSpace(r.FormValue("title"))
	st.Instructions = strings.TrimSpace(r.FormValue("instructions"))
	st.Phase = r.FormValue("phase")
	st.Assembly = r.FormValue("assembly")
	st.Action = r.FormValue("action")
	st.Module = r.FormValue("module")
	st.Levels = r.Form["level"]
	st.ComponentTypes = nil
	for _, t := range r.Form["component_type"] {
		if t != "" && !slices.Contains(st.ComponentTypes, t) {
			st.ComponentTypes = append(st.ComponentTypes, t)
		}
	}
	st.ValueKinds = r.Form["component_kind"]
	st.FollowUps = nil
	for _, row := range followUpRows(r.Form) {
		if row.Step == "" {
			continue
		}
		if row.Kind == "" {
			row.Kind = store.FollowUpFinding
		}
		st.FollowUps = append(st.FollowUps, store.FollowUp{StepID: row.Step, Kind: row.Kind})
	}
	if st.Title == "" {
		return "error.title_required"
	}
	if st.Phase == "" {
		return "error.phase_required"
	}
	var ok bool
	if st.Interval, ok = parseInterval(r.FormValue("interval_months"), r.FormValue("interval_km")); !ok {
		return "error.invalid_interval"
	}
	var errID string
	st.Values, errID = parseValues(r)
	return errID
}

// stepError maps validation errors when saving a step.
func stepError(l *i18n.Localizer, err error) (string, bool) {
	switch {
	case errors.Is(err, components.ErrInvalidType):
		return l.T("error.invalid_type"), true
	case errors.Is(err, catalog.ErrInvalid):
		return l.T("error.invalid_choice"), true
	case errors.Is(err, catalog.ErrInvalidInterval):
		return l.T("error.invalid_interval"), true
	}
	return valueErrorMessage(l, err)
}

func (s *Server) newStep(w http.ResponseWriter, r *http.Request) {
	form := url.Values{"phase": {r.URL.Query().Get("phase")}, "assembly": {r.URL.Query().Get("assembly")}}
	s.renderStepForm(w, r, http.StatusOK, store.Step{}, form, "")
}

func (s *Server) createStep(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	st := store.Step{ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID(r.Context()), CreatedAt: time.Now().UTC()}
	invalid := func(msg string) { s.renderStepForm(w, r, http.StatusUnprocessableEntity, store.Step{}, nil, msg) }
	if err := r.ParseForm(); err != nil {
		invalid(l.T("error.invalid_choice"))
		return
	}
	if errID := parseStepForm(r, &st); errID != "" {
		invalid(l.T(errID))
		return
	}
	err := catalog.AddStep(r.Context(), s.store, st)
	if msg, ok := stepError(l, err); ok {
		invalid(msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/steps/"+st.ID, http.StatusSeeOther)
}

// stepURL links to a step on the catalog page, with its phase expanded.
func stepURL(id string) string { return "/steps?step=" + id + "#step-" + id }

func (s *Server) step(ctx context.Context, id string) (store.Step, error) {
	return s.store.Steps().GetStep(ctx, ownerID(ctx), id)
}

func (s *Server) editStep(w http.ResponseWriter, r *http.Request) {
	st, err := s.step(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	shown := st
	localizeStep(i18n.FromContext(r.Context()), &shown)
	s.renderStepForm(w, r, http.StatusOK, st, stepForm(shown), "")
}

func (s *Server) updateStep(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	st, err := s.step(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	old := st
	invalid := func(msg string) { s.renderStepForm(w, r, http.StatusUnprocessableEntity, old, nil, msg) }
	if err := r.ParseForm(); err != nil {
		invalid(l.T("error.invalid_choice"))
		return
	}
	if errID := parseStepForm(r, &st); errID != "" {
		invalid(l.T(errID))
		return
	}
	// Unchanged texts of a built-in step stay translated.
	if title, instructions := builtinTexts(l, st.BuiltinID, "", ""); st.BuiltinID != "" {
		if st.Title == title {
			st.Title = ""
		}
		if st.Instructions == instructions {
			st.Instructions = ""
		}
	}
	err = catalog.UpdateStep(r.Context(), s.store, st)
	if msg, ok := stepError(l, err); ok {
		invalid(msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/steps/"+st.ID, http.StatusSeeOther)
}

func (s *Server) confirmDeleteStep(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	st, err := s.step(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	localizeStep(l, &st)
	s.confirm(w, r, l.T("confirm.delete_step", st.Title), "/steps/"+st.ID+"/delete", "/steps/"+st.ID+"/edit")
}

func (s *Server) deleteStep(w http.ResponseWriter, r *http.Request) {
	if err := catalog.DeleteStep(r.Context(), s.store, ownerID(r.Context()), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/steps", http.StatusSeeOther)
}

// moveStep returns a handler that moves a step up (-1) or down (1) in its
// assembly group. At the start or end of the group nothing happens.
func (s *Server) moveStep(delta int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		err := catalog.MoveStep(r.Context(), s.store, ownerID(r.Context()), id, delta)
		if err != nil && !errors.Is(err, catalog.ErrNoNeighbour) {
			s.fail(w, r, err)
			return
		}
		http.Redirect(w, r, stepURL(id), http.StatusSeeOther)
	}
}
