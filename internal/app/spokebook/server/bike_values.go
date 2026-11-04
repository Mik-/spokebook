package server

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/Mik-/spokebook/internal/app/spokebook/bikes"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
)

// bikeValueLabels are suggested labels of bike measurements, translated
// via "bike.values.suggest.<id>".
var bikeValueLabels = []string{
	"saddle_height", "saddle_setback", "saddle_drop", "stack", "reach", "frame_size", "wheelbase",
	"head_angle", "seat_angle", "stem_length", "handlebar_width", "crank_length", "spacers",
}

// bikeValueUnits are suggested even before they were used.
var bikeValueUnits = []string{"mm", "cm", "°"}

func (s *Server) editBikeValues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vs, err := s.store.Bikes().ListBikeValues(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderBikeValues(w, r, http.StatusOK, valueForm(url.Values{}, vs), "")
}

// renderBikeValues shows the measurements form of a bike; form is nil to
// use the submitted one.
func (s *Server) renderBikeValues(w http.ResponseWriter, r *http.Request, status int, form url.Values, errMsg string) {
	ctx := r.Context()
	b, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data, err := s.bikeValueSuggestions(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data["Bike"] = b
	data["HideKind"] = true
	data["ValueHint"] = "bike.values.hint"
	data["Error"] = errMsg
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "bike_values.html", data)
}

// bikeValueSuggestions returns the data for the datalists of the
// measurement fields: the labels and units used on bikes, then the
// suggested ones.
func (s *Server) bikeValueSuggestions(ctx context.Context) (map[string]any, error) {
	labels, err := s.store.Bikes().ListBikeValueLabels(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	units, err := s.store.Bikes().ListBikeValueUnits(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	l := i18n.FromContext(ctx)
	for _, id := range bikeValueLabels {
		if label := l.T("bike.values.suggest." + id); !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	for _, u := range bikeValueUnits {
		if !slices.Contains(units, u) {
			units = append(units, u)
		}
	}
	return map[string]any{"ValueLabels": labels, "ValueUnits": units}, nil
}

func (s *Server) updateBikeValues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	id := r.PathValue("id")
	vs, errID := parseValues(r)
	if errID != "" {
		s.renderBikeValues(w, r, http.StatusUnprocessableEntity, nil, l.T(errID))
		return
	}
	err := bikes.SetValues(ctx, s.store, ownerID(ctx), id, vs)
	if msg, ok := valueErrorMessage(l, err); ok {
		s.renderBikeValues(w, r, http.StatusUnprocessableEntity, nil, msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+id, http.StatusSeeOther)
}
