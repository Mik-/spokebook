package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/catalog"
	"github.com/spokebook/spokebook/internal/app/spokebook/due"
	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
	"github.com/spokebook/spokebook/internal/app/spokebook/maintenance"
	"github.com/spokebook/spokebook/internal/app/spokebook/odometer"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// maintenanceSummary is a maintenance in the list of a bike.
type maintenanceSummary struct {
	store.Maintenance
	Done, Total int
	OutOfLimit  bool
}

func summarize(m store.Maintenance) maintenanceSummary {
	sum := maintenanceSummary{Maintenance: m, Total: len(m.Steps)}
	for _, st := range m.Steps {
		if st.Done {
			sum.Done++
		}
		for _, v := range st.Values {
			sum.OutOfLimit = sum.OutOfLimit || v.OutOfLimit()
		}
	}
	return sum
}

// maintenanceName is the translated level of m, else its title (from
// before the catalog), else a generic name.
func maintenanceName(l *i18n.Localizer, m store.Maintenance) string {
	switch {
	case m.Level != "":
		return l.T("level." + m.Level)
	case m.Title != "":
		return m.Title
	}
	return l.T("maintenance.title")
}

// bikeMaintenances returns the maintenances of a bike, newest first.
func (s *Server) bikeMaintenances(ctx context.Context, bikeID string) ([]maintenanceSummary, error) {
	list, err := s.store.Maintenances().ListMaintenances(ctx, ownerID(ctx), bikeID)
	if err != nil {
		return nil, err
	}
	out := make([]maintenanceSummary, 0, len(list))
	for _, m := range list {
		full, err := s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), m.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, summarize(full))
	}
	return out, nil
}

// lastMaintenance returns the newest maintenance of a bike, nil without one.
func (s *Server) lastMaintenance(ctx context.Context, bikeID string) (*maintenanceSummary, error) {
	list, err := s.store.Maintenances().ListMaintenances(ctx, ownerID(ctx), bikeID)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	full, err := s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), list[0].ID)
	if err != nil {
		return nil, err
	}
	sum := summarize(full)
	return &sum, nil
}

func (s *Server) newMaintenance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	km, err := s.currentKm(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	form := url.Values{"date": {dateValue(time.Now().UTC())}, "km": {km}, "level": {r.URL.Query().Get("level")}}
	// Chosen steps (e.g. open work to catch up) are preselected alone;
	// without them or a chosen level, what is due.
	if steps := r.URL.Query()["step"]; len(steps) > 0 {
		form["step"] = steps
	} else if form.Get("level") == "" {
		bike, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		items, err := s.dueItems(ctx, bike)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		level, steps := due.Suggest(items)
		form["level"], form["step"] = []string{level}, steps
	}
	s.renderNewMaintenance(w, r, http.StatusOK, form, "")
}

// currentKm returns the bike's current odometer (or the one at going into
// service) rounded to whole km as form value, "" without either.
func (s *Server) currentKm(r *http.Request, bikeID string) (string, error) {
	t, err := odometer.LoadBike(r.Context(), s.store, ownerID(r.Context()), bikeID)
	if err != nil {
		return "", err
	}
	if m, ok := t.Current(); ok {
		return strconv.FormatInt((m+500)/1000, 10), nil
	}
	return "", nil
}

// levelOption is a level with its number of catalog steps.
type levelOption struct {
	Level string
	Steps int
}

// renderNewMaintenance shows the start form; form is nil to keep the
// submitted values.
func (s *Server) renderNewMaintenance(w http.ResponseWriter, r *http.Request, status int, form url.Values, errMsg string) {
	ctx := r.Context()
	bike, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	steps, err := catalog.Sorted(ctx, s.store, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	steps = catalog.Applicable(steps, bike.Features)
	levels := make([]levelOption, 0, len(catalog.Levels))
	for _, l := range catalog.Levels {
		n := 0
		for _, st := range steps {
			if slices.Contains(st.Levels, l) {
				n++
			}
		}
		levels = append(levels, levelOption{Level: l, Steps: n})
	}
	localizeSteps(i18n.FromContext(ctx), steps)
	items, err := s.dueItems(ctx, bike)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Pending steps with their own interval are listed apart from the
	// other on-demand steps.
	pending := due.Pending(items)
	pending = slices.DeleteFunc(pending, func(it due.Item) bool { return it.Step.ID == "" })
	data := map[string]any{"Bike": bike, "Levels": levels, "HasSteps": len(steps) > 0, "Due": pending, "Error": errMsg}
	data["OnDemand"] = slices.DeleteFunc(steps, func(st store.Step) bool {
		return len(st.Levels) > 0 || slices.ContainsFunc(pending, func(it due.Item) bool { return it.Step.ID == st.ID })
	})
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "maintenance_new.html", data)
}

func (s *Server) startMaintenance(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	invalid := func(msg string) { s.renderNewMaintenance(w, r, http.StatusUnprocessableEntity, nil, msg) }
	date, ok := parseDate(r.FormValue("date"))
	if !ok {
		invalid(l.T("error.invalid_date"))
		return
	}
	km, ok := parseKm(r.FormValue("km"))
	if !ok {
		invalid(l.T("error.invalid_km"))
		return
	}
	m, err := maintenance.Start(r.Context(), s.store, ownerID(r.Context()), r.PathValue("id"), r.FormValue("level"), r.Form["step"], date, km)
	if msg, ok := workflowError(l, err); ok {
		invalid(msg)
		return
	}
	switch {
	case errors.Is(err, maintenance.ErrNoSteps):
		invalid(l.T("error.no_steps"))
		return
	case errors.Is(err, catalog.ErrInvalid):
		invalid(l.T("error.invalid_choice"))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/maintenances/"+m.ID, http.StatusSeeOther)
}

// valueGroup is a list of values of a step: its own (Name empty) or those
// of one component.
type valueGroup struct {
	Name     string
	Position string
	Values   []store.MaintenanceValue
}

// maintenanceStepView is a step with its values grouped by component.
type maintenanceStepView struct {
	store.MaintenanceStep
	Groups []valueGroup
	// The step it follows up, the follow-ups added for it, those offered
	// on a finding, whether a measurement says one is needed, whether it
	// is open work of the bike and whether it can be removed.
	Origin    *store.MaintenanceStep
	Added     []store.MaintenanceStep
	Offers    []store.Step
	Needed    bool
	OpenWork  bool
	Removable bool
}

// maintenanceGroup is the steps of an assembly group (Assembly empty for
// steps without one), maintenancePhase those of a phase (Phase empty for
// steps from before the catalog).
type (
	maintenanceGroup struct {
		Assembly string
		Steps    []maintenanceStepView
	}
	maintenancePhase struct {
		Phase  string
		Groups []maintenanceGroup
	}
)

// groupMaintenanceSteps groups steps in display order by phase and
// assembly group.
func groupMaintenanceSteps(steps []maintenanceStepView) []maintenancePhase {
	var out []maintenancePhase
	for _, st := range steps {
		if n := len(out); n == 0 || out[n-1].Phase != st.Phase {
			out = append(out, maintenancePhase{Phase: st.Phase})
		}
		p := &out[len(out)-1]
		if n := len(p.Groups); n == 0 || p.Groups[n-1].Assembly != st.Assembly {
			p.Groups = append(p.Groups, maintenanceGroup{Assembly: st.Assembly})
		}
		g := &p.Groups[len(p.Groups)-1]
		g.Steps = append(g.Steps, st)
	}
	return out
}

func groupValues(st store.MaintenanceStep, typeNames map[string]string) maintenanceStepView {
	v := maintenanceStepView{MaintenanceStep: st}
	for _, val := range st.Values {
		n := len(v.Groups)
		if n == 0 || v.Groups[n-1].Values[0].ComponentID != val.ComponentID {
			g := valueGroup{Name: val.ComponentName, Position: val.ComponentPosition}
			if val.ComponentID != "" && g.Name == "" {
				g.Name = typeNames[val.ComponentType]
			}
			v.Groups = append(v.Groups, g)
			n++
		}
		v.Groups[n-1].Values = append(v.Groups[n-1].Values, val)
	}
	return v
}

// maintenanceForm prefills the progress form with m.
func maintenanceForm(m store.Maintenance) url.Values {
	form := url.Values{"notes": {m.Notes}}
	for _, st := range m.Steps {
		if st.Done {
			form.Set("done_"+st.ID, "1")
		}
		form.Set("note_"+st.ID, st.Note)
		for _, v := range st.Values {
			form.Set("measured_"+v.ID, optValue(v.Measured, numberValue))
		}
	}
	return form
}

func (s *Server) showMaintenance(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.Maintenances().GetMaintenance(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderMaintenance(w, r, http.StatusOK, m, maintenanceForm(m), "")
}

// renderMaintenance shows a maintenance; form is nil to keep the submitted
// values. Rendered for a POST without error, it reports the autosave.
func (s *Server) renderMaintenance(w http.ResponseWriter, r *http.Request, status int, m store.Maintenance, form url.Values, errMsg string) {
	ctx := r.Context()
	bike, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), m.BikeID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	typeNames, _, err := s.componentTypes(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	offers, err := maintenance.Offers(ctx, s.store, ownerID(ctx), m)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	open, err := s.openWork(ctx, bike)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	maintenance.SortSteps(&m)
	l := i18n.FromContext(ctx)
	for i := range m.Steps {
		st := &m.Steps[i]
		st.Title, st.Instructions = builtinTexts(l, st.BuiltinID, st.Title, st.Instructions)
	}
	steps := make([]maintenanceStepView, 0, len(m.Steps))
	// With htmx, saving also swaps the follow-ups of the steps that have
	// offers, as a measurement may make them needed.
	var oob []string
	for _, st := range m.Steps {
		v := groupValues(st, typeNames)
		for i, o := range m.Steps {
			if o.ID == st.OriginID {
				v.Origin = &m.Steps[i]
			}
			if o.OriginID == st.ID {
				v.Added = append(v.Added, o)
			}
		}
		v.Offers = offers[st.ID]
		localizeSteps(l, v.Offers)
		v.Needed = slices.ContainsFunc(st.Values, func(val store.MaintenanceValue) bool { return val.OutOfLimit() })
		v.OpenWork = open[st.ID]
		v.Removable = m.Open() && maintenance.Removable(st)
		if len(v.Offers) > 0 {
			oob = append(oob, "#follow-ups-"+st.ID)
		}
		steps = append(steps, v)
	}
	data := map[string]any{
		"M": summarize(m), "Bike": bike, "Phases": groupMaintenanceSteps(steps), "Error": errMsg,
		"Saved": r.Method == http.MethodPost && errMsg == "", "FollowUpsOOB": strings.Join(oob, ","),
	}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "maintenance.html", data)
}

func (s *Server) saveMaintenance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	m, err := s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	p, errMsg := progress(l, r, m)
	if errMsg != "" {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, m, nil, errMsg)
		return
	}
	err = maintenance.Save(ctx, s.store, ownerID(ctx), m.ID, p, r.PostForm.Get("complete") != "")
	if errors.Is(err, maintenance.ErrCompleted) {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, m, maintenanceForm(m), l.T("error.maintenance_completed"))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// htmx saves on every change and only swaps in the new status.
	if r.Header.Get("HX-Request") == "true" {
		if m, err = s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), m.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		s.renderMaintenance(w, r, http.StatusOK, m, nil, "")
		return
	}
	http.Redirect(w, r, "/maintenances/"+m.ID, http.StatusSeeOther)
}

// progress reads the progress form of m; errMsg reports an invalid
// measurement.
func progress(l *i18n.Localizer, r *http.Request, m store.Maintenance) (p maintenance.Progress, errMsg string) {
	p = maintenance.Progress{
		Notes: r.PostForm.Get("notes"), Steps: map[string]maintenance.StepResult{}, Measured: map[string]*float64{},
	}
	for _, st := range m.Steps {
		p.Steps[st.ID] = maintenance.StepResult{Done: r.PostForm.Get("done_"+st.ID) != "", Note: r.PostForm.Get("note_" + st.ID)}
		for _, v := range st.Values {
			key := "measured_" + v.ID
			if _, sent := r.PostForm[key]; !sent {
				continue
			}
			x, ok := optional(r.PostForm.Get(key), parseNumber)
			if !ok {
				return p, l.T("error.invalid_measured", v.Label)
			}
			p.Measured[v.ID] = x
		}
	}
	return p, ""
}

// addFollowUp saves the progress form and adds the follow-up its button
// names as "<origin step ID>:<catalog step ID>".
func (s *Server) addFollowUp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	m, err := s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	p, errMsg := progress(l, r, m)
	if errMsg != "" {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, m, nil, errMsg)
		return
	}
	origin, step, _ := strings.Cut(r.PostForm.Get("follow_up"), ":")
	err = maintenance.Save(ctx, s.store, ownerID(ctx), m.ID, p, false)
	if err == nil {
		err = maintenance.AddFollowUp(ctx, s.store, ownerID(ctx), m.ID, origin, step)
	}
	if errors.Is(err, maintenance.ErrCompleted) {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, m, maintenanceForm(m), l.T("error.maintenance_completed"))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/maintenances/"+m.ID+"#mstep-"+origin, http.StatusSeeOther)
}

// removeStep saves the progress form and removes the step its button
// names, with what it brought along.
func (s *Server) removeStep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	m, err := s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	p, errMsg := progress(l, r, m)
	if errMsg != "" {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, m, nil, errMsg)
		return
	}
	err = maintenance.Save(ctx, s.store, ownerID(ctx), m.ID, p, false)
	if err == nil {
		err = maintenance.Remove(ctx, s.store, ownerID(ctx), m.ID, r.PathValue("step"))
	}
	switch {
	case errors.Is(err, maintenance.ErrCompleted):
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, m, maintenanceForm(m), l.T("error.maintenance_completed"))
		return
	case errors.Is(err, maintenance.ErrStepDone):
		if m, err = s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), m.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, m, maintenanceForm(m), l.T("error.step_done"))
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	target := "/maintenances/" + m.ID
	if i := slices.IndexFunc(m.Steps, func(st store.MaintenanceStep) bool { return st.ID == r.PathValue("step") }); i >= 0 && m.Steps[i].OriginID != "" {
		target += "#mstep-" + m.Steps[i].OriginID
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// dismissStep marks a follow-up left undone as no longer needed and goes
// back to the maintenance or, with back=bike, to the bike.
func (s *Server) dismissStep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m, err := s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := maintenance.Dismiss(ctx, s.store, ownerID(ctx), m.ID, r.PathValue("step")); err != nil {
		s.fail(w, r, err)
		return
	}
	if r.FormValue("back") == "bike" {
		http.Redirect(w, r, "/bikes/"+m.BikeID, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/maintenances/"+m.ID, http.StatusSeeOther)
}

func (s *Server) addStepsForm(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.Maintenances().GetMaintenance(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderAddSteps(w, r, http.StatusOK, m, "")
}

// renderAddSteps shows the catalog steps that m doesn't contain yet,
// grouped by phase and assembly group.
func (s *Server) renderAddSteps(w http.ResponseWriter, r *http.Request, status int, m store.Maintenance, errMsg string) {
	ctx := r.Context()
	bike, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), m.BikeID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	steps, err := catalog.Sorted(ctx, s.store, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	steps = catalog.Applicable(steps, bike.Features)
	localizeSteps(i18n.FromContext(ctx), steps)
	var views []stepView
	for _, st := range steps {
		if !slices.ContainsFunc(m.Steps, func(ms store.MaintenanceStep) bool { return ms.CatalogStepID == st.ID }) {
			views = append(views, stepView{Step: st})
		}
	}
	s.render(w, r, status, "maintenance_steps.html", map[string]any{
		"M": m, "Bike": bike, "Phases": groupSteps(views), "Error": errMsg,
	})
}

func (s *Server) addSteps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	m, err := s.store.Maintenances().GetMaintenance(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	err = maintenance.AddSteps(ctx, s.store, ownerID(ctx), m.ID, r.PostForm["step"])
	switch {
	case errors.Is(err, maintenance.ErrNoSteps):
		s.renderAddSteps(w, r, http.StatusUnprocessableEntity, m, l.T("error.no_steps_chosen"))
		return
	case errors.Is(err, maintenance.ErrCompleted):
		s.renderAddSteps(w, r, http.StatusUnprocessableEntity, m, l.T("error.maintenance_completed"))
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/maintenances/"+m.ID, http.StatusSeeOther)
}

func (s *Server) editMaintenance(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.Maintenances().GetMaintenance(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderMaintenanceEdit(w, r, http.StatusOK, m, url.Values{
		"date": {dateValue(m.PerformedAt)}, "km": {kmValue(m.DistanceM)},
	}, "")
}

// renderMaintenanceEdit shows the form for date and odometer; form is nil
// to keep the submitted values.
func (s *Server) renderMaintenanceEdit(w http.ResponseWriter, r *http.Request, status int, m store.Maintenance, form url.Values, errMsg string) {
	bike, err := s.store.Bikes().GetBike(r.Context(), ownerID(r.Context()), m.BikeID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := map[string]any{"M": m, "Bike": bike, "Error": errMsg}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "maintenance_edit.html", data)
}

func (s *Server) updateMaintenance(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	m, err := s.store.Maintenances().GetMaintenance(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	invalid := func(msg string) { s.renderMaintenanceEdit(w, r, http.StatusUnprocessableEntity, m, nil, msg) }
	date, ok := parseDate(r.FormValue("date"))
	if !ok {
		invalid(l.T("error.invalid_date"))
		return
	}
	km, ok := parseKm(r.FormValue("km"))
	if !ok {
		invalid(l.T("error.invalid_km"))
		return
	}
	err = maintenance.ChangeDate(r.Context(), s.store, ownerID(r.Context()), m.ID, date, km)
	if errors.Is(err, maintenance.ErrCompleted) {
		invalid(l.T("error.maintenance_completed"))
		return
	}
	if msg, ok := workflowError(l, err); ok {
		invalid(msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/maintenances/"+m.ID, http.StatusSeeOther)
}

func (s *Server) reopenMaintenance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := maintenance.Reopen(r.Context(), s.store, ownerID(r.Context()), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/maintenances/"+id, http.StatusSeeOther)
}

func (s *Server) confirmDeleteMaintenance(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	m, err := s.store.Maintenances().GetMaintenance(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.confirm(w, r, l.T("confirm.delete_maintenance", maintenanceName(l, m), l.Date(m.PerformedAt)),
		"/maintenances/"+m.ID+"/delete", "/maintenances/"+m.ID)
}

func (s *Server) deleteMaintenance(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.Maintenances().GetMaintenance(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err == nil {
		err = s.store.Maintenances().DeleteMaintenance(r.Context(), ownerID(r.Context()), m.ID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+m.BikeID, http.StatusSeeOther)
}
