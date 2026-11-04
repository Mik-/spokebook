package server

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// This file has the workflows started from a bike: adding a new component,
// installing a spare and replacing an installed component.

// mountForm prefills date and position. The odometer stays empty, which
// uses the bike's odometer at the date.
func mountForm(position string) url.Values {
	return url.Values{"date": {dateValue(time.Now().UTC())}, "position": {position}}
}

// parseMount reads date, odometer and position. It returns the message ID
// of a validation error.
func parseMount(r *http.Request) (components.Mount, string) {
	m := components.Mount{
		OwnerID: ownerID(r.Context()), Position: r.FormValue("position"),
		AllowDuplicate: r.FormValue("allow_duplicate") != "",
	}
	var ok bool
	if m.At, ok = parseDate(r.FormValue("date")); !ok {
		return m, "error.invalid_date"
	}
	if m.DistanceM, ok = optional(r.FormValue("km"), parseKm); !ok {
		return m, "error.invalid_km"
	}
	return m, ""
}

// duplicateWarning returns the warning for a components.DuplicateError,
// naming the component already installed. It marks the submitted form with
// "duplicate_of", so the page offers to confirm or to replace that one.
func (s *Server) duplicateWarning(r *http.Request, err error) (string, bool) {
	var dup *components.DuplicateError
	if !errors.As(err, &dup) {
		return "", false
	}
	v, err := s.oneComponentView(r.Context(), dup.ComponentID)
	if err != nil {
		return "", false
	}
	if r.PostForm == nil {
		r.PostForm = url.Values{}
	}
	r.PostForm.Set("duplicate_of", v.ID)
	return i18n.FromContext(r.Context()).T("warning.duplicate", displayName(v)), true
}

// workflowError maps validation errors of the combined workflows.
func workflowError(l *i18n.Localizer, err error) (string, bool) {
	if msg, ok := valueErrorMessage(l, err); ok {
		return msg, true
	}
	return installErrorMessage(l, err)
}

func (s *Server) addToBikeForm(w http.ResponseWriter, r *http.Request) {
	s.renderAddToBike(w, r, http.StatusOK, mountForm(""), "")
}

// renderAddToBike shows the form; form is nil to keep the submitted values.
func (s *Server) renderAddToBike(w http.ResponseWriter, r *http.Request, status int, form url.Values, errMsg string) {
	ctx := r.Context()
	bike, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data, err := s.componentFormData(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data["Bike"] = bike
	data["Positions"] = components.Positions
	data["Error"] = errMsg
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "bike_component_new.html", data)
}

func (s *Server) addToBike(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	invalid := func(msg string) { s.renderAddToBike(w, r, http.StatusUnprocessableEntity, nil, msg) }

	m, errID := parseMount(r)
	if errID != "" {
		invalid(l.T(errID))
		return
	}
	m.BikeID = r.PathValue("id")
	c := store.Component{ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID(r.Context()), CreatedAt: time.Now().UTC()}
	vs, errID, err := s.parseComponentForm(r, &c)
	if err != nil {
		s.fail(w, r, err)
		return
	} else if errID != "" {
		invalid(l.T(errID))
		return
	}
	err = components.AddToBike(r.Context(), s.store, c, vs, m)
	if msg, ok := s.duplicateWarning(r, err); ok {
		invalid(msg)
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
	http.Redirect(w, r, "/bikes/"+m.BikeID+"/components", http.StatusSeeOther)
}

func (s *Server) installSpareForm(w http.ResponseWriter, r *http.Request) {
	s.renderInstallSpare(w, r, http.StatusOK, mountForm(""), "")
}

// renderInstallSpare shows the form to install a component that is active
// and not installed; form is nil to keep the submitted values.
func (s *Server) renderInstallSpare(w http.ResponseWriter, r *http.Request, status int, form url.Values, errMsg string) {
	ctx := r.Context()
	bike, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	views, err := s.componentViews(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := map[string]any{"Bike": bike, "Spares": spares(i18n.FromContext(ctx), views), "Positions": components.Positions, "Error": errMsg}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "bike_component_install.html", data)
}

// spares returns the components that are active and not installed, sorted
// by type name.
func spares(l *i18n.Localizer, views map[string]componentView) []componentView {
	var out []componentView
	for _, v := range sortedViews(l, views) {
		if v.Active() && v.Current == nil {
			out = append(out, v)
		}
	}
	return out
}

func (s *Server) installSpare(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	invalid := func(msg string) { s.renderInstallSpare(w, r, http.StatusUnprocessableEntity, nil, msg) }

	m, errID := parseMount(r)
	if errID != "" {
		invalid(l.T(errID))
		return
	}
	m.BikeID = r.PathValue("id")
	_, err := components.Install(r.Context(), s.store, store.Installation{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: m.OwnerID, ComponentID: r.FormValue("component"),
		BikeID: m.BikeID, Position: m.Position, InstalledAt: m.At, CreatedAt: time.Now().UTC(),
	}, m.DistanceM, m.AllowDuplicate)
	if msg, ok := s.duplicateWarning(r, err); ok {
		invalid(msg)
		return
	}
	if msg, ok := installErrorMessage(l, err); ok {
		invalid(msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+m.BikeID+"/components", http.StatusSeeOther)
}

func (s *Server) replaceForm(w http.ResponseWriter, r *http.Request) {
	v, err := s.oneComponentView(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if v.Current == nil {
		http.Redirect(w, r, "/components/"+v.ID, http.StatusSeeOther)
		return
	}
	form := mountForm(v.Current.Position)
	vs, err := s.store.Components().ListComponentValues(r.Context(), ownerID(r.Context()), v.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// The new component starts with the old one's details and values,
	// except the dates that belong to the old part.
	for k, vals := range componentForm(v.Component, vs) {
		if k != "purchased_at" && k != "replace_by" {
			form[k] = vals
		}
	}
	s.renderReplace(w, r, http.StatusOK, form, "")
}

// renderReplace shows the form; form is nil to keep the submitted values.
// With "existing" set, the replacement is chosen from the spares instead of
// entered as a new component.
func (s *Server) renderReplace(w http.ResponseWriter, r *http.Request, status int, form url.Values, errMsg string) {
	ctx := r.Context()
	v, err := s.oneComponentView(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if v.Current == nil {
		http.Redirect(w, r, "/components/"+v.ID, http.StatusSeeOther)
		return
	}
	data, err := s.componentFormData(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	views, err := s.componentViews(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Spares of the same type come first.
	list := spares(i18n.FromContext(ctx), views)
	slices.SortStableFunc(list, func(a, b componentView) int {
		switch {
		case a.Type == v.Type && b.Type != v.Type:
			return -1
		case a.Type != v.Type && b.Type == v.Type:
			return 1
		}
		return 0
	})
	data["C"] = v
	data["Spares"] = list
	data["Existing"] = r.FormValue("existing") != ""
	data["Positions"] = components.Positions
	data["Reasons"] = components.ComponentRetireReasons
	data["Error"] = errMsg
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "component_replace.html", data)
}

func (s *Server) replaceComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	invalid := func(msg string) { s.renderReplace(w, r, http.StatusUnprocessableEntity, nil, msg) }

	old, err := s.oneComponentView(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if old.Current == nil {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	m, errID := parseMount(r)
	if errID != "" {
		invalid(l.T(errID))
		return
	}
	reason := ""
	if r.FormValue("retire") != "" {
		reason = r.FormValue("reason")
	}
	if r.FormValue("existing") != "" {
		spare := r.FormValue("component")
		if spare == "" {
			invalid(l.T("error.invalid_choice"))
			return
		}
		err = components.ReplaceWithSpare(ctx, s.store, old.Current.ID, reason, spare, m)
	} else {
		c := store.Component{ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID(ctx), CreatedAt: time.Now().UTC()}
		var vs []store.Value
		vs, errID, err = s.parseComponentForm(r, &c)
		if err != nil {
			s.fail(w, r, err)
			return
		} else if errID != "" {
			invalid(l.T(errID))
			return
		}
		err = components.Replace(ctx, s.store, old.Current.ID, reason, c, vs, m)
	}
	if msg, ok := s.duplicateWarning(r, err); ok {
		invalid(msg)
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
	http.Redirect(w, r, "/bikes/"+old.Current.BikeID+"/components", http.StatusSeeOther)
}

// pastInstallation is a closed installation on a bike with its component.
type pastInstallation struct {
	store.Installation
	C         componentView
	DistanceM int64
}

// installedOn returns the components currently installed on the bike,
// sorted by type name and position.
func installedOn(l *i18n.Localizer, views map[string]componentView, bikeID string) []componentView {
	var installed []componentView
	for _, v := range sortedViews(l, views) {
		if v.Current != nil && v.Current.BikeID == bikeID {
			installed = append(installed, v)
		}
	}
	return installed
}

// bikeComponents lists the components installed on a bike and those that
// were installed before, including retired ones.
func (s *Server) bikeComponents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	bike, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	views, err := s.componentViews(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	insts, err := s.store.Components().ListAllInstallations(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var past []pastInstallation
	for _, i := range insts {
		if i.BikeID == bike.ID && !i.Open() {
			past = append(past, pastInstallation{
				Installation: i,
				C:            views[i.ComponentID],
				DistanceM:    components.Mileage([]store.Installation{i}, nil),
			})
		}
	}
	slices.SortFunc(past, func(a, b pastInstallation) int { return b.RemovedAt.Compare(*a.RemovedAt) })
	s.render(w, r, http.StatusOK, "bike_components.html", map[string]any{
		"Bike":      bike,
		"Installed": installedOn(l, views, bike.ID),
		"Past":      past,
	})
}
