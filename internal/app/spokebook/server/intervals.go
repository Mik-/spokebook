package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/due"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

func (s *Server) editIntervals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	intervals, err := catalog.LevelIntervals(ctx, s.store, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	form := url.Values{}
	for _, l := range catalog.IntervalLevels {
		form.Set(l+"_months", optValue(intervals[l].Months, monthsValue))
		form.Set(l+"_km", optValue(intervals[l].DistanceM, kmValue))
	}
	s.renderIntervals(w, r, http.StatusOK, form, "")
}

// renderIntervals shows the level intervals form; form is nil to keep the
// submitted values.
func (s *Server) renderIntervals(w http.ResponseWriter, r *http.Request, status int, form url.Values, errMsg string) {
	data := map[string]any{"Levels": catalog.IntervalLevels, "Error": errMsg}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "intervals.html", data)
}

func (s *Server) updateIntervals(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	invalid := func() {
		s.renderIntervals(w, r, http.StatusUnprocessableEntity, nil, l.T("error.invalid_interval"))
	}
	intervals := map[string]store.Interval{}
	for _, level := range catalog.IntervalLevels {
		iv, ok := parseInterval(r.FormValue(level+"_months"), r.FormValue(level+"_km"))
		if !ok {
			invalid()
			return
		}
		intervals[level] = iv
	}
	err := catalog.SetLevelIntervals(r.Context(), s.store, ownerID(r.Context()), intervals)
	if errors.Is(err, catalog.ErrInvalidInterval) {
		invalid()
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/steps", http.StatusSeeOther)
}

// dueItems returns what is due on an active bike, the most urgent first,
// with translated step titles; nothing for a retired bike.
func (s *Server) dueItems(ctx context.Context, bike store.Bike) ([]due.Item, error) {
	if !bike.Active() {
		return nil, nil
	}
	items, err := due.Load(ctx, s.store, ownerID(ctx), bike, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	l := i18n.FromContext(ctx)
	for i := range items {
		if items[i].Step.ID != "" {
			localizeStep(l, &items[i].Step)
		}
		if o := items[i].Open; o != nil {
			o.Origin.Title, _ = builtinTexts(l, o.Origin.BuiltinID, o.Origin.Title, "")
		}
	}
	return items, nil
}

// openWork returns the IDs of the maintenance steps that are open work of
// the bike.
func (s *Server) openWork(ctx context.Context, bike store.Bike) (map[string]bool, error) {
	items, err := s.dueItems(ctx, bike)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, it := range items {
		if it.Open != nil {
			out[it.Open.Step.ID] = true
		}
	}
	return out, nil
}
