package server

import (
	"cmp"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
	"github.com/spokebook/spokebook/internal/app/spokebook/rides"
	"github.com/spokebook/spokebook/internal/app/spokebook/rides/parse"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

const (
	// maxRideUploadMB limits the files of one import together.
	maxRideUploadMB = 64
	// maxImportRides limits the rides of one import.
	maxImportRides = 5000
)

// gearName is the name of a bike in a source. A Garmin gear has no name
// in the file, only an ID.
func gearName(l *i18n.Localizer, key, name string) string {
	if strings.HasPrefix(key, parse.GarminGearPrefix) {
		return l.T("rides.gear.garmin", name)
	}
	return name
}

// rideView is a ride with the name of its bike.
type rideView struct {
	store.Ride
	BikeName string
}

// rideFilter is the filter of the ride list. Without year it shows the
// newest year with rides; "all" shows every year.
type rideFilter struct {
	Bike, Year, Gear string
	// gear is the mapping chosen by Gear.
	gear *store.RideGear
	// yearSet reports whether the request chose a year or "all".
	yearSet bool
}

func parseRideFilter(r *http.Request) rideFilter {
	f := rideFilter{Bike: r.FormValue("bike"), Year: r.FormValue("year"), Gear: r.FormValue("gear")}
	f.yearSet = r.Form.Has("year")
	if f.Year == "all" {
		f.Year = ""
	}
	return f
}

// Any reports whether the filter narrows the list.
func (f rideFilter) Any() bool { return f.Bike != "" || f.Year != "" || f.Gear != "" }

// Query returns the filter as query string.
func (f rideFilter) Query() string {
	q := url.Values{}
	for k, v := range map[string]string{"bike": f.Bike, "year": cmp.Or(f.Year, "all"), "gear": f.Gear} {
		if v != "" {
			q.Set(k, v)
		}
	}
	return q.Encode()
}

func (f rideFilter) match(rd store.Ride) bool {
	switch {
	case f.Year != "" && strconv.Itoa(rd.RiddenOn.Year()) != f.Year,
		f.Bike == "discarded" && !rd.Discarded,
		f.Bike != "discarded" && rd.Discarded && f.Bike != "",
		f.Bike == "none" && rd.BikeID != "",
		f.Bike != "" && f.Bike != "none" && f.Bike != "discarded" && rd.BikeID != f.Bike,
		f.gear != nil && (rd.Source != f.gear.Source || rd.GearKey != f.gear.GearKey):
		return false
	}
	return true
}

// filterRides returns all rides (newest first) and those matching the
// filter. With newest, a filter without year gets the newest one. An
// unknown mapping in the filter matches nothing.
func (s *Server) filterRides(r *http.Request, f *rideFilter, newest bool) (all, list []store.Ride, err error) {
	ctx := r.Context()
	if f.Gear != "" {
		// An unknown mapping has no source, so no ride matches.
		g, err := s.store.Rides().GetRideGear(ctx, ownerID(ctx), f.Gear)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, nil, err
		}
		f.gear = &g
	}
	if all, err = s.store.Rides().ListRides(ctx, ownerID(ctx)); err != nil {
		return nil, nil, err
	}
	if newest && !f.yearSet && len(all) > 0 {
		f.Year = strconv.Itoa(all[0].RiddenOn.Year())
	}
	for _, rd := range all {
		if f.match(rd) {
			list = append(list, rd)
		}
	}
	return all, list, nil
}

func (s *Server) listRides(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := parseRideFilter(r)
	all, matched, err := s.filterRides(r, &f, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := s.bikeNames(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var years []string
	for _, rd := range all {
		if y := strconv.Itoa(rd.RiddenOn.Year()); !slices.Contains(years, y) {
			years = append(years, y)
		}
	}
	list := make([]rideView, len(matched))
	var total int64
	for i, rd := range matched {
		list[i] = rideView{Ride: rd, BikeName: names[rd.BikeID]}
		if !rd.Discarded {
			total += rd.DistanceM
		}
	}
	bikes, err := s.store.Bikes().ListBikes(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	gears, err := s.store.Rides().ListRideGears(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "rides.html", map[string]any{
		"Rides": list, "Bikes": bikes, "Gears": gears, "Filter": f, "Years": years, "TotalM": total,
		"Any": len(all) > 0, "Result": importResult(r.URL.Query()),
		"DiscardURL": "/rides/filtered/discard?" + f.Query(), "DeleteURL": "/rides/filtered/delete?" + f.Query(),
	})
}

// confirmBulkRides asks before discarding or deleting all rides of the
// filter; action is "discard" or "delete".
func (s *Server) confirmBulkRides(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.confirmBulk(w, r, action)
	}
}

func (s *Server) confirmBulk(w http.ResponseWriter, r *http.Request, action string) {
	f := parseRideFilter(r)
	if !f.Any() {
		http.NotFound(w, r)
		return
	}
	_, list, err := s.filterRides(r, &f, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := i18n.FromContext(r.Context())
	var total int64
	for _, rd := range list {
		total += rd.DistanceM
	}
	msg := l.N("confirm.bulk_"+action, len(list), l.Distance(total))
	if action == "delete" {
		msg += " " + l.T("confirm.bulk_delete_hint")
	}
	s.render(w, r, http.StatusOK, "confirm.html", map[string]any{
		"Message": msg, "Action": "/rides/filtered/" + action + "?" + f.Query(), "Cancel": "/rides?" + f.Query(),
		"Submit": l.T("rides." + action),
	})
}

// bulkRides discards or deletes all rides of the filter.
func (s *Server) bulkRides(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.bulk(w, r, action)
	}
}

func (s *Server) bulk(w http.ResponseWriter, r *http.Request, action string) {
	ctx := r.Context()
	f := parseRideFilter(r)
	if !f.Any() {
		http.NotFound(w, r)
		return
	}
	_, list, err := s.filterRides(r, &f, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := make([]string, 0, len(list))
	for _, rd := range list {
		if action == "delete" || !rd.Discarded {
			ids = append(ids, rd.ID)
		}
	}
	if action == "delete" {
		err = rides.DeleteAll(ctx, s.store, ownerID(ctx), ids)
	} else {
		err = rides.DiscardAll(ctx, s.store, ownerID(ctx), ids)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("rides changed", "user", ownerID(ctx), "action", action, "count", len(ids))
	if action == "discard" {
		f.Bike = "discarded"
	}
	http.Redirect(w, r, "/rides?"+f.Query(), http.StatusSeeOther)
}

// importResult reads the counts an import redirects with; nil without.
func importResult(q url.Values) *rides.Result {
	if !q.Has("imported") {
		return nil
	}
	n := func(k string) int { v, _ := strconv.Atoi(q.Get(k)); return v }
	return &rides.Result{Imported: n("imported"), Skipped: n("skipped"), Unassigned: n("unassigned")}
}

// activeBikes returns the bikes not retired, plus the one with keepID.
func (s *Server) activeBikes(r *http.Request, keepID string) ([]store.Bike, error) {
	bikes, err := s.store.Bikes().ListBikes(r.Context(), ownerID(r.Context()))
	return slices.DeleteFunc(bikes, func(b store.Bike) bool { return !b.Active() && b.ID != keepID }), err
}

func (s *Server) importRidesForm(w http.ResponseWriter, r *http.Request) {
	s.renderRideImport(w, r, http.StatusOK, nil, "")
}

// fileError is a file that could not be read.
type fileError struct {
	Name    string
	Message string
}

func (s *Server) renderRideImport(w http.ResponseWriter, r *http.Request, status int, files []fileError, errMsg string) {
	bikes, err := s.activeBikes(r, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, status, "rides_import.html", map[string]any{
		"Bikes": bikes, "FileErrors": files, "Error": errMsg, "MaxMB": maxRideUploadMB,
	})
}

// fileErrorIDs are the messages of files that could not be read.
var fileErrorIDs = map[error]string{
	parse.ErrUnknownFormat: "rides.error.format",
	parse.ErrNoTime:        "rides.error.no_time",
	parse.ErrNoRide:        "rides.error.no_ride",
}

// previewRides reads the uploaded files and shows what an import would do.
func (s *Server) previewRides(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, maxRideUploadMB<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		msg := l.T("rides.error.upload")
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			msg = l.T("rides.error.too_large", maxRideUploadMB)
		}
		s.renderRideImport(w, r, http.StatusUnprocessableEntity, nil, msg)
		return
	}
	var read []parse.Ride
	var failed []fileError
	for _, fh := range r.MultipartForm.File["files"] {
		f, err := fh.Open()
		if err != nil {
			s.fail(w, r, err)
			return
		}
		got, err := parse.File(fh.Filename, f, time.Local)
		_ = f.Close()
		if err != nil {
			id := "rides.error.invalid"
			for e, msg := range fileErrorIDs {
				if errors.Is(err, e) {
					id = msg
				}
			}
			failed = append(failed, fileError{Name: fh.Filename, Message: l.T(id)})
			continue
		}
		read = append(read, got...)
	}
	if len(read) > maxImportRides {
		s.renderRideImport(w, r, http.StatusUnprocessableEntity, failed, l.T("rides.error.too_many", maxImportRides))
		return
	}
	if len(read) == 0 {
		s.renderRideImport(w, r, http.StatusUnprocessableEntity, failed, l.T("rides.error.none"))
		return
	}
	slices.SortStableFunc(read, func(a, b parse.Ride) int { return a.StartedAt.Compare(b.StartedAt) })
	cands, gears, err := rides.Preview(ctx, s.store, ownerID(ctx), read)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	bikes, err := s.activeBikes(r, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := s.bikeNames(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rows := make([]previewRow, len(cands))
	for i, c := range cands {
		data, err := json.Marshal(c.Ride)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		rows[i] = previewRow{Candidate: c, Data: string(data), BikeName: names[c.Existing.BikeID]}
	}
	s.render(w, r, http.StatusOK, "rides_preview.html", map[string]any{
		"Rows": rows, "Gears": gears, "Bikes": bikes, "DefaultBike": r.FormValue("bike"), "FileErrors": failed,
		"New": slices.IndexFunc(cands, func(c rides.Candidate) bool { return c.Status != rides.Known }) >= 0,
	})
}

// previewRow is a ride of the preview with the ride as JSON for the form.
type previewRow struct {
	rides.Candidate
	Data     string
	BikeName string
}

// validRide checks a ride that came back from the preview form.
func validRide(rd parse.Ride) bool {
	return slices.Contains([]string{parse.SourceFIT, parse.SourceGPX, parse.SourceTCX}, rd.Source) &&
		rd.ExternalID != "" && len(rd.ExternalID) <= 200 && len(rd.GearKey) <= 200 && len(rd.Name) <= 500 &&
		!rd.StartedAt.IsZero() && !rd.EndedAt.Before(rd.StartedAt) && !rd.RiddenOn.IsZero() &&
		rd.DistanceM >= 0 && rd.DistanceM <= 10_000_000
}

// importRides saves the rides chosen in the preview.
func (s *Server) importRides(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var items []rides.Item
	for i := 0; i < maxImportRides; i++ {
		n := strconv.Itoa(i)
		data := r.PostFormValue("ride." + n)
		if data == "" {
			break
		}
		if r.PostFormValue("import."+n) == "" {
			continue
		}
		var it rides.Item
		if err := json.Unmarshal([]byte(data), &it.Ride); err != nil || !validRide(it.Ride) {
			s.errorPage(w, r, http.StatusBadRequest, i18n.FromContext(r.Context()).T("error.bad_request"))
			return
		}
		it.BikeID = r.PostFormValue("bike." + n)
		it.Auto = it.BikeID == "auto"
		if it.Auto {
			it.BikeID = ""
		}
		items = append(items, it)
	}
	var gears []rides.GearChoice
	for i := 0; ; i++ {
		n := strconv.Itoa(i)
		key := r.PostFormValue("gear." + n + ".key")
		if key == "" || i >= maxImportRides {
			break
		}
		gears = append(gears, rides.GearChoice{
			Source: r.PostFormValue("gear." + n + ".source"), Key: key, Name: r.PostFormValue("gear." + n + ".name"),
			BikeID: r.PostFormValue("gear." + n + ".bike"), Remember: r.PostFormValue("gear."+n+".remember") != "",
		})
	}
	res, err := rides.Import(ctx, s.store, ownerID(ctx), items, gears, r.PostFormValue("default_bike"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("rides imported", "user", ownerID(ctx), "imported", res.Imported, "skipped", res.Skipped)
	q := url.Values{
		"imported": {strconv.Itoa(res.Imported)}, "skipped": {strconv.Itoa(res.Skipped)},
		"unassigned": {strconv.Itoa(res.Unassigned)},
	}
	http.Redirect(w, r, "/rides?"+q.Encode(), http.StatusSeeOther)
}

func (s *Server) editRide(w http.ResponseWriter, r *http.Request) {
	rd, err := s.store.Rides().GetRide(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderRideEdit(w, r, http.StatusOK, rd, url.Values{"bike": {rd.BikeID}}, "")
}

func (s *Server) renderRideEdit(w http.ResponseWriter, r *http.Request, status int, rd store.Ride, form url.Values, errMsg string) {
	bikes, err := s.activeBikes(r, rd.BikeID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	gear, err := s.store.Rides().FindRideGear(r.Context(), ownerID(r.Context()), rd.Source, rd.GearKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	data := map[string]any{"Ride": rd, "Bikes": bikes, "Gear": gear, "Error": errMsg}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, "ride_edit.html", data)
}

func (s *Server) updateRide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rd, err := s.store.Rides().GetRide(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = rides.SetBike(ctx, s.store, ownerID(ctx), rd.ID, r.FormValue("bike"))
	if errors.Is(err, rides.ErrBeforeService) {
		s.renderRideEdit(w, r, http.StatusUnprocessableEntity, rd, nil, i18n.FromContext(ctx).T("rides.error.before_service"))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/rides", http.StatusSeeOther)
}

// setRideDiscarded discards or restores a ride.
func (s *Server) setRideDiscarded(discarded bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := rides.SetDiscarded(ctx, s.store, ownerID(ctx), r.PathValue("id"), discarded); err != nil {
			s.fail(w, r, err)
			return
		}
		http.Redirect(w, r, "/rides", http.StatusSeeOther)
	}
}

func (s *Server) confirmDeleteRide(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	rd, err := s.store.Rides().GetRide(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.confirm(w, r, l.T("confirm.delete_ride", l.Date(rd.RiddenOn), l.Distance(rd.DistanceM)),
		"/rides/"+rd.ID+"/delete", "/rides/"+rd.ID+"/edit")
}

func (s *Server) deleteRide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := rides.Delete(ctx, s.store, ownerID(ctx), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/rides", http.StatusSeeOther)
}

// gearView is a mapping with the name of its bike and its number of rides.
type gearView struct {
	store.RideGear
	BikeName string
	Rides    int
}

func (s *Server) listGears(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gears, err := s.store.Rides().ListRideGears(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	all, err := s.store.Rides().ListRides(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := s.bikeNames(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list := make([]gearView, len(gears))
	for i, g := range gears {
		list[i] = gearView{RideGear: g, BikeName: names[g.BikeID]}
		for _, rd := range all {
			if rd.Source == g.Source && rd.GearKey == g.GearKey {
				list[i].Rides++
			}
		}
	}
	s.render(w, r, http.StatusOK, "ride_gears.html", map[string]any{"Gears": list})
}

func (s *Server) editGear(w http.ResponseWriter, r *http.Request) {
	g, err := s.store.Rides().GetRideGear(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	bikes, err := s.activeBikes(r, g.BikeID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "ride_gear_edit.html", map[string]any{
		"Gear": g, "Bikes": bikes, "Form": url.Values{"bike": {g.BikeID}, "existing": {"1"}},
	})
}

func (s *Server) updateGear(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := rides.SetGearBike(ctx, s.store, ownerID(ctx), r.PathValue("id"), r.FormValue("bike"), r.FormValue("existing") != "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/ride-gears", http.StatusSeeOther)
}

func (s *Server) confirmDeleteGear(w http.ResponseWriter, r *http.Request) {
	g, err := s.store.Rides().GetRideGear(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := i18n.FromContext(r.Context())
	s.confirm(w, r, l.T("confirm.delete_gear", gearName(l, g.GearKey, g.Name)),
		"/ride-gears/"+g.ID+"/delete", "/ride-gears/"+g.ID+"/edit")
}

func (s *Server) deleteGear(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Rides().DeleteRideGear(r.Context(), ownerID(r.Context()), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/ride-gears", http.StatusSeeOther)
}
