package server

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/spokebook/spokebook/internal/app/spokebook/bikes"
	"github.com/spokebook/spokebook/internal/app/spokebook/components"
	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
	"github.com/spokebook/spokebook/internal/app/spokebook/odometer"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

func (s *Server) editBike(w http.ResponseWriter, r *http.Request) {
	bike, err := s.store.Bikes().GetBike(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderBikeForm(w, r, http.StatusOK, "bike_edit.html", bike, bikeForm(bike), "")
}

func (s *Server) updateBike(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	bike, err := s.store.Bikes().GetBike(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if errID := parseBikeForm(r, &bike); errID != "" {
		s.renderBikeForm(w, r, http.StatusUnprocessableEntity, "bike_edit.html", bike, nil, l.T(errID))
		return
	}
	err = bikes.Update(r.Context(), s.store, bike)
	for e, id := range map[error]string{
		bikes.ErrReadingsBeforeService: "error.readings_before_service",
		bikes.ErrRidesBeforeService:    "error.rides_before_service",
		bikes.ErrReadingsBelowService:  "error.readings_below_service",
	} {
		if errors.Is(err, e) {
			s.renderBikeForm(w, r, http.StatusUnprocessableEntity, "bike_edit.html", bike, nil, l.T(id))
			return
		}
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+bike.ID, http.StatusSeeOther)
}

func (s *Server) confirmDeleteBike(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	bike, err := s.store.Bikes().GetBike(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.confirm(w, r, l.T("confirm.delete_bike", bike.Name), "/bikes/"+bike.ID+"/delete", "/bikes/"+bike.ID)
}

func (s *Server) deleteBike(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	err := components.DeleteBike(r.Context(), s.store, ownerID(r.Context()), r.PathValue("id"))
	if errors.Is(err, components.ErrBikeHasInstallations) {
		s.renderBike(w, r, http.StatusUnprocessableEntity, l.T("error.bike_has_installations"))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// reading loads the reading from the path and checks that it belongs to
// the bike in the path.
func (s *Server) reading(r *http.Request) (store.OdometerReading, error) {
	rd, err := s.store.Odometer().GetReading(r.Context(), ownerID(r.Context()), r.PathValue("rid"))
	if err != nil {
		return store.OdometerReading{}, err
	}
	if rd.BikeID != r.PathValue("id") {
		return store.OdometerReading{}, store.ErrNotFound
	}
	return rd, nil
}

func readingPath(rd store.OdometerReading) string {
	return "/bikes/" + rd.BikeID + "/odometer/" + rd.ID
}

func (s *Server) editReading(w http.ResponseWriter, r *http.Request) {
	rd, err := s.reading(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "reading_edit.html", map[string]any{
		"Reading": rd, "Path": readingPath(rd),
		"Form": url.Values{"date": {dateValue(rd.RecordedAt)}, "km": {kmValue(rd.DistanceM)}},
	})
}

func (s *Server) updateReading(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	rd, err := s.reading(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	invalid := func(msg string) {
		s.render(w, r, http.StatusUnprocessableEntity, "reading_edit.html", map[string]any{
			"Reading": rd, "Path": readingPath(rd), "Error": msg,
		})
	}
	var ok bool
	if rd.RecordedAt, ok = parseDate(r.FormValue("date")); !ok {
		invalid(l.T("error.invalid_date"))
		return
	}
	if rd.DistanceM, ok = parseKm(r.FormValue("km")); !ok {
		invalid(l.T("error.invalid_km"))
		return
	}
	err = odometer.Update(r.Context(), s.store, rd)
	if msg, ok := odometerErrorMessage(l, err); ok {
		invalid(msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+rd.BikeID, http.StatusSeeOther)
}

func (s *Server) confirmDeleteReading(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	rd, err := s.reading(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.confirm(w, r, l.T("confirm.delete_reading", l.Distance(rd.DistanceM), l.Date(rd.RecordedAt)),
		readingPath(rd)+"/delete", "/bikes/"+rd.BikeID)
}

func (s *Server) deleteReading(w http.ResponseWriter, r *http.Request) {
	rd, err := s.reading(r)
	if err == nil {
		err = odometer.Delete(r.Context(), s.store, rd)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+rd.BikeID, http.StatusSeeOther)
}

// odometerErrorMessage maps validation errors from the odometer package.
func odometerErrorMessage(l *i18n.Localizer, err error) (string, bool) {
	var low *odometer.TooLowError
	var high *odometer.TooHighError
	var below *odometer.BelowServiceError
	switch {
	case errors.As(err, &below):
		return l.T("error.km_below_service", l.Distance(below.InServiceM)), true
	case errors.Is(err, odometer.ErrBeforeService):
		return l.T("error.reading_before_service"), true
	case errors.As(err, &low):
		return l.T("error.km_too_low", l.Distance(low.Prev.DistanceM), l.Date(low.Prev.RecordedAt)), true
	case errors.As(err, &high):
		return l.T("error.km_too_high", l.Distance(high.Next.DistanceM), l.Date(high.Next.RecordedAt)), true
	}
	return "", false
}
