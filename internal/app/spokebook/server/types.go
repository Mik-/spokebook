package server

import (
	"errors"
	"net/http"
	"net/url"
	"slices"

	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

func (s *Server) listTypes(w http.ResponseWriter, r *http.Request) {
	s.renderTypes(w, r, http.StatusOK, "")
}

func (s *Server) renderTypes(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	ctx := r.Context()
	names, types, err := s.componentTypes(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	all, err := s.store.Components().ListTypes(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var hidden []typeOption
	for _, t := range all {
		if t.Hidden {
			hidden = append(hidden, typeOption{ID: t.ID, Name: names[t.ID], Builtin: components.IsBuiltin(t.ID)})
		}
	}
	slices.SortFunc(hidden, func(a, b typeOption) int { return i18n.FromContext(ctx).Compare(a.Name, b.Name) })
	s.render(w, r, status, "types.html", map[string]any{"Types": types, "Hidden": hidden, "Error": errMsg})
}

func (s *Server) newType(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "type_new.html", map[string]any{})
}

func (s *Server) createType(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	_, err := components.CreateType(r.Context(), s.store, ownerID(r.Context()), r.FormValue("name"))
	if errors.Is(err, components.ErrNameRequired) {
		s.render(w, r, http.StatusUnprocessableEntity, "type_new.html", map[string]any{"Error": l.T("error.name_required")})
		return
	}
	s.afterTypeChange(w, r, err)
}

func (s *Server) renameType(w http.ResponseWriter, r *http.Request) {
	err := components.RenameType(r.Context(), s.store, ownerID(r.Context()), r.PathValue("id"), r.FormValue("name"))
	s.afterTypeChange(w, r, err)
}

func (s *Server) deleteType(w http.ResponseWriter, r *http.Request) {
	err := components.DeleteType(r.Context(), s.store, ownerID(r.Context()), r.PathValue("id"))
	s.afterTypeChange(w, r, err)
}

func (s *Server) restoreType(w http.ResponseWriter, r *http.Request) {
	err := components.RestoreType(r.Context(), s.store, ownerID(r.Context()), r.PathValue("id"))
	s.afterTypeChange(w, r, err)
}

func (s *Server) editTypeValues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	vs, err := s.store.Components().ListTypeValues(ctx, ownerID(ctx), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderTypeValues(w, r, http.StatusOK, id, valueForm(url.Values{}, vs), "")
}

// renderTypeValues shows the default values form of a type; form is nil to
// use the submitted one.
func (s *Server) renderTypeValues(w http.ResponseWriter, r *http.Request, status int, id string, form url.Values, errMsg string) {
	ctx := r.Context()
	t, err := components.ActiveType(ctx, s.store, ownerID(ctx), id)
	if err != nil {
		s.fail(w, r, typeNotFound(err))
		return
	}
	data, err := s.valueSuggestions(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data["Type"] = typeOption{ID: t.ID, Name: typeName(i18n.FromContext(ctx), t)}
	data["Error"] = errMsg
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "type_values.html", data)
}

func (s *Server) updateTypeValues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	id := r.PathValue("id")
	vs, errID := parseValues(r)
	if errID != "" {
		s.renderTypeValues(w, r, http.StatusUnprocessableEntity, id, nil, l.T(errID))
		return
	}
	err := components.SetTypeValues(ctx, s.store, ownerID(ctx), id, vs)
	if msg, ok := valueErrorMessage(l, err); ok {
		s.renderTypeValues(w, r, http.StatusUnprocessableEntity, id, nil, msg)
		return
	}
	if err != nil {
		s.fail(w, r, typeNotFound(err))
		return
	}
	http.Redirect(w, r, "/component-types", http.StatusSeeOther)
}

// typeNotFound turns an unknown or hidden type in the URL into a 404.
func typeNotFound(err error) error {
	if errors.Is(err, components.ErrInvalidType) {
		return store.ErrNotFound
	}
	return err
}

// afterTypeChange redirects back to the list or shows a validation error.
func (s *Server) afterTypeChange(w http.ResponseWriter, r *http.Request, err error) {
	l := i18n.FromContext(r.Context())
	switch {
	case errors.Is(err, components.ErrNameRequired):
		s.renderTypes(w, r, http.StatusUnprocessableEntity, l.T("error.name_required"))
	case errors.Is(err, components.ErrTypeInUse):
		s.renderTypes(w, r, http.StatusUnprocessableEntity, l.T("error.type_in_use"))
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/component-types", http.StatusSeeOther)
	}
}
