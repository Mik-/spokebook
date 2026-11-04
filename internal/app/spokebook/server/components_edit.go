package server

import (
	"net/http"
	"net/url"

	"github.com/spokebook/spokebook/internal/app/spokebook/components"
	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// componentForm prefills the component form with c and its values.
func componentForm(c store.Component, vs []store.Value) url.Values {
	return valueForm(url.Values{
		"type":         {c.Type},
		"manufacturer": {c.Manufacturer},
		"model":        {c.Model},
		"part_number":  {c.PartNumber},
		"purchased_at": {optValue(c.PurchasedAt, dateValue)},
		"dealer":       {c.Dealer},
		"price":        {optValue(c.PriceCents, centsValue)},
		"currency":     {c.Currency},
		"max_km":       {optValue(c.MaxDistanceM, kmValue)},
		"replace_by":   {optValue(c.ReplaceBy, dateValue)},
		"url":          {c.URL},
		"notes":        {c.Notes},
	}, vs)
}

func (s *Server) editComponent(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.Components().GetComponent(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vs, err := s.store.Components().ListComponentValues(r.Context(), ownerID(r.Context()), c.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderComponentEdit(w, r, http.StatusOK, c, componentForm(c, vs), "")
}

// renderComponentEdit shows the edit form; form is nil to use the submitted one.
func (s *Server) renderComponentEdit(w http.ResponseWriter, r *http.Request, status int, c store.Component, form url.Values, errMsg string) {
	ctx := r.Context()
	data, err := s.componentFormData(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from, err := s.fromBike(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data["C"] = c
	data["From"] = from
	data["FromQuery"] = bikeQuery(from)
	data["Error"] = errMsg
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "component_edit.html", data)
}

func (s *Server) updateComponent(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	c, err := s.store.Components().GetComponent(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vs, errID, err := s.parseComponentForm(r, &c)
	if err != nil {
		s.fail(w, r, err)
		return
	} else if errID != "" {
		s.renderComponentEdit(w, r, http.StatusUnprocessableEntity, c, nil, l.T(errID))
		return
	}
	err = components.Update(r.Context(), s.store, c, vs)
	if msg, ok := valueErrorMessage(l, err); ok {
		s.renderComponentEdit(w, r, http.StatusUnprocessableEntity, c, nil, msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from, err := s.fromBike(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/components/"+c.ID+bikeQuery(from), http.StatusSeeOther)
}

func (s *Server) confirmDeleteComponent(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	v, err := s.oneComponentView(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from, err := s.fromBike(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q := bikeQuery(from)
	s.confirm(w, r, l.T("confirm.delete_component", displayName(v)), "/components/"+v.ID+"/delete"+q, "/components/"+v.ID+q)
}

func (s *Server) deleteComponent(w http.ResponseWriter, r *http.Request) {
	from, err := s.fromBike(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.Components().DeleteComponent(r.Context(), ownerID(r.Context()), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	if from != nil {
		http.Redirect(w, r, "/bikes/"+from.ID+"/components", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/components", http.StatusSeeOther)
}

func installationForm(i store.Installation) url.Values {
	return url.Values{
		"bike":         {i.BikeID},
		"position":     {i.Position},
		"installed_at": {dateValue(i.InstalledAt)},
		"installed_km": {autoKm(i.InstalledM, i.InstalledAuto)},
		"removed_at":   {optValue(i.RemovedAt, dateValue)},
		"removed_km":   {optValue(i.RemovedM, func(m int64) string { return autoKm(m, i.RemovedAuto) })},
	}
}

// autoKm is the form value of a distance; empty if it follows the odometer.
func autoKm(m int64, auto bool) string {
	if auto {
		return ""
	}
	return kmValue(m)
}

func (s *Server) editInstallation(w http.ResponseWriter, r *http.Request) {
	inst, err := s.store.Components().GetInstallation(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderInstallationEdit(w, r, http.StatusOK, inst, installationForm(inst), "")
}

func (s *Server) renderInstallationEdit(w http.ResponseWriter, r *http.Request, status int, i store.Installation, form url.Values, errMsg string) {
	bikes, err := s.store.Bikes().ListBikes(r.Context(), ownerID(r.Context()))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := map[string]any{"I": i, "Bikes": bikes, "Positions": components.Positions, "Error": errMsg}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "installation_edit.html", data)
}

func (s *Server) updateInstallation(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	inst, err := s.store.Components().GetInstallation(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	invalid := func(msg string) { s.renderInstallationEdit(w, r, http.StatusUnprocessableEntity, inst, nil, msg) }

	inst.BikeID = r.FormValue("bike")
	inst.Position = r.FormValue("position")
	var ok bool
	if inst.InstalledAt, ok = parseDate(r.FormValue("installed_at")); !ok {
		invalid(l.T("error.invalid_date"))
		return
	}
	installedM, ok := optional(r.FormValue("installed_km"), parseKm)
	if !ok {
		invalid(l.T("error.invalid_km"))
		return
	}
	if inst.InstalledAuto = installedM == nil; installedM != nil {
		inst.InstalledM = *installedM
	}
	if inst.RemovedAt, ok = optional(r.FormValue("removed_at"), parseDate); !ok {
		invalid(l.T("error.invalid_date"))
		return
	}
	if inst.RemovedM, ok = optional(r.FormValue("removed_km"), parseKm); !ok {
		invalid(l.T("error.invalid_km"))
		return
	}
	inst.RemovedAuto = inst.RemovedAt != nil && inst.RemovedM == nil
	err = components.UpdateInstallation(r.Context(), s.store, inst)
	if msg, ok := installErrorMessage(l, err); ok {
		invalid(msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/components/"+inst.ComponentID, http.StatusSeeOther)
}

func (s *Server) confirmDeleteInstallation(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	inst, err := s.store.Components().GetInstallation(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.confirm(w, r, l.T("confirm.delete_installation", l.Date(inst.InstalledAt)),
		"/installations/"+inst.ID+"/delete", "/components/"+inst.ComponentID)
}

func (s *Server) deleteInstallation(w http.ResponseWriter, r *http.Request) {
	inst, err := s.store.Components().GetInstallation(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err == nil {
		err = s.store.Components().DeleteInstallation(r.Context(), ownerID(r.Context()), inst.ID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/components/"+inst.ComponentID, http.StatusSeeOther)
}

func (s *Server) newComponent(w http.ResponseWriter, r *http.Request) {
	s.renderNewComponent(w, r, http.StatusOK, "", "")
}

// duplicateComponent shows the form for a new component prefilled with an
// existing one, e.g. for a second tire of the same kind.
func (s *Server) duplicateComponent(w http.ResponseWriter, r *http.Request) {
	s.renderNewComponent(w, r, http.StatusOK, r.PathValue("id"), "")
}

// renderNewComponent shows the form for a new component. With srcID it is
// a duplicate, prefilled from that component. After a validation error the
// submitted values are kept.
func (s *Server) renderNewComponent(w http.ResponseWriter, r *http.Request, status int, srcID, errMsg string) {
	ctx := r.Context()
	data, err := s.componentFormData(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data["Error"] = errMsg
	if srcID != "" {
		src, err := s.store.Components().GetComponent(ctx, ownerID(ctx), srcID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data["Source"] = src
		if errMsg == "" {
			vs, err := s.store.Components().ListComponentValues(ctx, ownerID(ctx), src.ID)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			data["Form"] = componentForm(src, vs)
		}
	}
	s.render(w, r, status, "component_new.html", data)
}
