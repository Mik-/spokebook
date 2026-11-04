package server

import (
	"errors"
	"net/http"

	"github.com/spokebook/spokebook/internal/app/spokebook/components"
	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
)

func (s *Server) retireComponent(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	invalid := func(msg string) { s.renderRetireComponentForm(w, r, http.StatusUnprocessableEntity, msg) }
	date, ok := parseDate(r.FormValue("date"))
	if !ok {
		invalid(l.T("error.invalid_date"))
		return
	}
	km, ok := optional(r.FormValue("km"), parseKm)
	if !ok {
		invalid(l.T("error.invalid_km"))
		return
	}
	id := r.PathValue("id")
	err := components.RetireComponent(r.Context(), s.store, ownerID(r.Context()), id, date, r.FormValue("reason"), km)
	if msg, ok := installErrorMessage(l, err); ok {
		invalid(msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/components/"+id, http.StatusSeeOther)
}

func (s *Server) reactivateComponent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := components.Reactivate(r.Context(), s.store, ownerID(r.Context()), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/components/"+id, http.StatusSeeOther)
}

func (s *Server) retireBike(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	invalid := func(msg string) { s.renderRetireBikeForm(w, r, http.StatusUnprocessableEntity, msg) }
	date, ok := parseDate(r.FormValue("date"))
	if !ok {
		invalid(l.T("error.invalid_date"))
		return
	}
	km, ok := optional(r.FormValue("km"), parseKm)
	if !ok {
		invalid(l.T("error.invalid_km"))
		return
	}
	id := r.PathValue("id")
	err := components.RetireBike(r.Context(), s.store, ownerID(r.Context()), id, date, r.FormValue("reason"), km)
	if errors.Is(err, components.ErrNoOdometer) {
		invalid(l.T("error.retire_no_odometer"))
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
	http.Redirect(w, r, "/bikes/"+id, http.StatusSeeOther)
}

func (s *Server) reactivateBike(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := components.ReactivateBike(r.Context(), s.store, ownerID(r.Context()), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+id, http.StatusSeeOther)
}
