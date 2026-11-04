package server

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
	"github.com/spokebook/spokebook/internal/app/spokebook/rides/garmin"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// garminSource returns the owner's Garmin connection; nil if there is
// none.
func (s *Server) garminSource(r *http.Request) (*store.RideSource, error) {
	src, err := s.store.Rides().GetRideSource(r.Context(), ownerID(r.Context()), garmin.Source)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &src, nil
}

func (s *Server) garminPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	src, err := s.garminSource(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := map[string]any{
		"Enabled": s.garmin.Enabled(), "Source": src, "Pending": s.garmin.Pending(ownerID(ctx)),
		"TooSoon": r.URL.Query().Has("too_soon"),
	}
	if p, ok := s.garmin.Progress(ownerID(ctx)); ok {
		fetched, err := s.ridesSince(r, p.Started)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data["Progress"], data["Fetched"] = p, fetched
	}
	if src != nil && src.Cursor != nil {
		data["Until"] = l.Date(src.Cursor.In(time.Local))
	}
	if src != nil && src.LastRunAt != nil {
		at := src.LastRunAt.In(time.Local)
		data["LastRun"] = l.Date(at) + " " + at.Format("15:04")
	}
	s.render(w, r, http.StatusOK, "rides_garmin.html", data)
}

func (s *Server) garminConnectForm(w http.ResponseWriter, r *http.Request) {
	src, err := s.garminSource(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	since, fromRides := time.Now(), false
	if src != nil {
		since = src.Since
	} else if first, rides, err := s.defaultSince(r); err != nil {
		s.fail(w, r, err)
		return
	} else if first != nil {
		since, fromRides = *first, rides
	}
	form := url.Values{"since": {dateValue(since)}}
	if fromRides {
		form.Set("from_rides", "1")
	}
	s.renderGarminConnect(w, r, http.StatusOK, src, form, "")
}

// defaultSince returns the first day offered when connecting: the day of
// the newest ride, as older ones are there already (e.g. uploaded or
// fetched before disconnecting), else the earliest day a bike went into
// service; nil without either. fromRides tells it is the newest ride.
func (s *Server) defaultSince(r *http.Request) (day *time.Time, fromRides bool, err error) {
	ctx := r.Context()
	all, err := s.store.Rides().ListRides(ctx, ownerID(ctx))
	if err != nil {
		return nil, false, err
	}
	if len(all) > 0 {
		return &all[0].RiddenOn, true, nil
	}
	bikes, err := s.store.Bikes().ListBikes(ctx, ownerID(ctx))
	var first *time.Time
	for _, b := range bikes {
		if b.InServiceAt != nil && (first == nil || b.InServiceAt.Before(*first)) {
			first = b.InServiceAt
		}
	}
	return first, false, err
}

func (s *Server) renderGarminConnect(w http.ResponseWriter, r *http.Request, status int, src *store.RideSource, form url.Values, errMsg string) {
	data := map[string]any{"Enabled": s.garmin.Enabled(), "Source": src, "Error": errMsg}
	if src != nil && src.Cursor != nil {
		data["Until"] = i18n.FromContext(r.Context()).Date(src.Cursor.In(time.Local))
	}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "rides_garmin_connect.html", data)
}

func (s *Server) garminConnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	src, err := s.garminSource(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	since, ok := parseDate(r.PostFormValue("since"))
	if !ok || since.After(time.Now()) {
		s.renderGarminConnect(w, r, http.StatusUnprocessableEntity, src, nil, l.T("garmin.error.since"))
		return
	}
	err = s.garmin.Connect(ctx, ownerID(ctx), r.PostFormValue("key"), since)
	msg := ""
	switch {
	case err == nil:
		http.Redirect(w, r, "/rides/garmin", http.StatusSeeOther)
		return
	case errors.Is(err, garmin.ErrDisabled):
		msg = l.T("garmin.disabled")
	case errors.Is(err, garmin.ErrNoKey):
		msg = l.T("garmin.error.no_key")
	case errors.Is(err, garmin.ErrUnauthorized):
		msg = l.T("garmin.error.key")
	case errors.Is(err, garmin.ErrRateLimited):
		msg = l.T("garmin.error.rate_limited")
	case errors.Is(err, garmin.ErrUnreachable):
		s.log.Warn("garmin: connect", "user", ownerID(ctx), "err", err)
		msg = l.T("garmin.error.unreachable")
	default:
		s.fail(w, r, err)
		return
	}
	// The key is never sent back.
	r.PostForm.Del("key")
	s.renderGarminConnect(w, r, http.StatusUnprocessableEntity, src, nil, msg)
}

func (s *Server) garminSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := s.garmin.Trigger(ctx, ownerID(ctx))
	switch {
	case errors.Is(err, garmin.ErrTooSoon):
		http.Redirect(w, r, "/rides/garmin?too_soon=1", http.StatusSeeOther)
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/rides/garmin", http.StatusSeeOther)
	}
}

func (s *Server) confirmGarminDisconnect(w http.ResponseWriter, r *http.Request) {
	s.confirm(w, r, i18n.FromContext(r.Context()).T("confirm.garmin_disconnect"), "/rides/garmin/disconnect", "/rides/garmin")
}

func (s *Server) garminDisconnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.garmin.Disconnect(ctx, ownerID(ctx)); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/rides/garmin", http.StatusSeeOther)
}

// maxFetchedShown limits the rides shown while a run is going.
const maxFetchedShown = 20

// ridesSince returns the newest rides created since t, with their bikes'
// names.
func (s *Server) ridesSince(r *http.Request, t time.Time) ([]rideView, error) {
	ctx := r.Context()
	all, err := s.store.Rides().ListRides(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	names, err := s.bikeNames(ctx)
	if err != nil {
		return nil, err
	}
	var out []rideView
	for _, rd := range all {
		if !rd.CreatedAt.Before(t) {
			out = append(out, rideView{Ride: rd, BikeName: names[rd.BikeID]})
		}
	}
	return out[:min(len(out), maxFetchedShown)], nil
}
