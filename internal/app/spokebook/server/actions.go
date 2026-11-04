package server

import (
	"context"
	"net/http"

	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// This file has the form pages for actions on a component or bike:
// install, remove and retire.

// oneComponentView returns the view of the component id.
func (s *Server) oneComponentView(ctx context.Context, id string) (componentView, error) {
	views, err := s.componentViews(ctx)
	if err != nil {
		return componentView{}, err
	}
	v, ok := views[id]
	if !ok {
		return componentView{}, store.ErrNotFound
	}
	return v, nil
}

// displayName is "Manufacturer Model", or the type name if both are empty.
func displayName(v componentView) string {
	if v.Manufacturer != "" || v.Model != "" {
		return v.Manufacturer + " " + v.Model
	}
	return v.TypeName
}

func (s *Server) installForm(w http.ResponseWriter, r *http.Request) {
	s.renderInstallForm(w, r, http.StatusOK, "")
}

func (s *Server) renderInstallForm(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	ctx := r.Context()
	v, err := s.oneComponentView(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	bikes, err := s.store.Bikes().ListBikes(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var active []store.Bike
	for _, b := range bikes {
		if b.Active() {
			active = append(active, b)
		}
	}
	s.render(w, r, status, "component_install.html", map[string]any{
		"C": v, "Bikes": active, "Positions": components.Positions, "Error": errMsg,
	})
}

func (s *Server) removeForm(w http.ResponseWriter, r *http.Request) {
	s.renderRemoveForm(w, r, http.StatusOK, "")
}

func (s *Server) renderRemoveForm(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	v, err := s.oneComponentView(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if v.Current == nil {
		http.Redirect(w, r, "/components/"+v.ID, http.StatusSeeOther)
		return
	}
	s.render(w, r, status, "component_remove.html", map[string]any{"C": v, "Error": errMsg})
}

func (s *Server) retireComponentForm(w http.ResponseWriter, r *http.Request) {
	s.renderRetireComponentForm(w, r, http.StatusOK, "")
}

func (s *Server) renderRetireComponentForm(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	l := i18n.FromContext(r.Context())
	v, err := s.oneComponentView(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := map[string]any{
		"Title":    l.T("retire.component"),
		"ListName": l.T("components.title"),
		"ListURL":  "/components",
		"Name":     displayName(v),
		"Back":     "/components/" + v.ID,
		"Action":   "/components/" + v.ID + "/retire",
		"Reasons":  components.ComponentRetireReasons,
		"Error":    errMsg,
	}
	if v.Current != nil {
		data["Hint"] = l.T("retire.component.hint")
	}
	s.render(w, r, status, "retire.html", data)
}

func (s *Server) retireBikeForm(w http.ResponseWriter, r *http.Request) {
	s.renderRetireBikeForm(w, r, http.StatusOK, "")
}

func (s *Server) renderRetireBikeForm(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
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
	data := map[string]any{
		"Title":    l.T("retire.bike"),
		"ListName": l.T("bikes.title"),
		"ListURL":  "/",
		"Name":     bike.Name,
		"Back":     "/bikes/" + bike.ID,
		"Action":   "/bikes/" + bike.ID + "/retire",
		"Reasons":  components.BikeRetireReasons,
		"Error":    errMsg,
	}
	for _, v := range views {
		if v.Current != nil && v.Current.BikeID == bike.ID {
			data["Hint"] = l.T("retire.bike.hint")
			break
		}
	}
	s.render(w, r, status, "retire.html", data)
}
