package server

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/bikes"
	"github.com/spokebook/spokebook/internal/app/spokebook/due"
	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
	"github.com/spokebook/spokebook/internal/app/spokebook/odometer"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// bikeRow is a bike in the overview with its odometer and warnings.
type bikeRow struct {
	store.Bike
	DistanceM *int64
	Alerts    []componentView
	// Worn is the installed component below its distance limit that used
	// most of it (those at the limit are in Alerts), Last the newest
	// maintenance; Last only for active bikes.
	Worn *componentView
	Last *maintenanceSummary
	// Due is the highest level due soon or overdue, else the most urgent
	// such step.
	Due *due.Item
	// CoverID is the attachment ID of the bike's photo; empty if none.
	CoverID string
}

func (s *Server) listBikes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bikes, err := s.store.Bikes().ListBikes(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	latest, err := odometer.Latest(ctx, s.store, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	views, err := s.componentViews(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	covers, err := s.store.Attachments().Covers(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := i18n.FromContext(ctx)
	var active, retired []bikeRow
	for _, b := range bikes {
		row := bikeRow{Bike: b, CoverID: covers[b.ID]}
		if m, ok := latest[b.ID]; ok {
			row.DistanceM = &m
		}
		for _, v := range installedOn(l, views, b.ID) {
			if v.Wear.Alert() {
				row.Alerts = append(row.Alerts, v)
			}
			if v.Wear.Percent >= 0 && !v.Wear.OverDistance && (row.Worn == nil || v.Wear.Percent > row.Worn.Wear.Percent) {
				row.Worn = &v
			}
		}
		if b.Active() {
			if row.Last, err = s.lastMaintenance(ctx, b.ID); err != nil {
				s.fail(w, r, err)
				return
			}
			items, err := s.dueItems(ctx, b)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			row.Due = due.Next(items)
			active = append(active, row)
		} else {
			retired = append(retired, row)
		}
	}
	s.render(w, r, http.StatusOK, "bikes.html", map[string]any{"Bikes": active, "Retired": retired})
}

func (s *Server) newBike(w http.ResponseWriter, r *http.Request) {
	// New and self-built bikes start at 0 km; used ones are changed.
	s.renderBikeForm(w, r, http.StatusOK, "bike_new.html", store.Bike{}, url.Values{"in_service_km": {"0"}}, "")
}

func (s *Server) createBike(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	b := store.Bike{ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID(r.Context()), CreatedAt: time.Now().UTC()}
	if errID := parseBikeForm(r, &b); errID != "" {
		s.renderBikeForm(w, r, http.StatusUnprocessableEntity, "bike_new.html", b, nil, l.T(errID))
		return
	}
	if _, err := s.store.Bikes().CreateBike(r.Context(), b); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+b.ID, http.StatusSeeOther)
}

// renderBikeForm shows the new or edit page; form is nil to keep the
// submitted values.
func (s *Server) renderBikeForm(w http.ResponseWriter, r *http.Request, status int, page string, b store.Bike, form url.Values, errMsg string) {
	dealers, err := s.store.Components().ListDealers(r.Context(), ownerID(r.Context()))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data := map[string]any{"Bike": b, "Origins": bikes.Origins, "Features": bikes.Features, "Dealers": dealers, "Error": errMsg}
	if form != nil {
		data["Form"] = form
	}
	s.render(w, r, status, page, data)
}

// parseBikeForm sets the form fields on b and returns the message ID of a
// validation error.
func parseBikeForm(r *http.Request, b *store.Bike) string {
	b.Name = strings.TrimSpace(r.FormValue("name"))
	b.Type = strings.TrimSpace(r.FormValue("type"))
	b.Notes = strings.TrimSpace(r.FormValue("notes"))
	b.Origin = r.FormValue("origin")
	b.Dealer = strings.TrimSpace(r.FormValue("dealer"))
	b.Features = bikes.SortFeatures(r.Form["feature"])
	if b.Name == "" {
		return "error.name_required"
	}
	switch bikes.Validate(*b) {
	case bikes.ErrInvalidOrigin:
		return "error.invalid_origin"
	case bikes.ErrInvalidFeature:
		return "error.invalid_choice"
	}
	var ok bool
	if b.PurchasedAt, ok = optional(r.FormValue("purchased_at"), parseDate); !ok {
		return "error.invalid_date"
	}
	if b.PriceCents, ok = optional(r.FormValue("price"), parseCents); !ok {
		return "error.invalid_price"
	}
	b.Currency = ""
	if b.PriceCents != nil {
		if b.Currency, ok = parseCurrency(r.FormValue("currency")); !ok {
			return "error.invalid_currency"
		}
	}
	if b.InServiceAt, ok = optional(r.FormValue("in_service_at"), parseDate); !ok {
		return "error.invalid_date"
	}
	if b.InServiceM, ok = optional(r.FormValue("in_service_km"), parseKm); !ok {
		return "error.invalid_km"
	}
	return ""
}

func bikeForm(b store.Bike) url.Values {
	return url.Values{
		"name":          {b.Name},
		"type":          {b.Type},
		"notes":         {b.Notes},
		"origin":        {b.Origin},
		"purchased_at":  {optValue(b.PurchasedAt, dateValue)},
		"dealer":        {b.Dealer},
		"price":         {optValue(b.PriceCents, centsValue)},
		"currency":      {b.Currency},
		"in_service_at": {optValue(b.InServiceAt, dateValue)},
		"in_service_km": {optValue(b.InServiceM, kmValue)},
		"feature":       b.Features,
	}
}

func (s *Server) showBike(w http.ResponseWriter, r *http.Request) {
	s.renderBike(w, r, http.StatusOK, "")
}

func (s *Server) renderBike(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	ctx := r.Context()
	bike, err := s.store.Bikes().GetBike(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	readings, err := s.store.Odometer().ListReadings(ctx, ownerID(ctx), bike.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	timeline, err := odometer.Load(ctx, s.store, bike)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var current *int64
	if m, ok := timeline.Current(); ok {
		current = &m
	}
	all, err := s.store.Rides().ListRides(ctx, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var rideCount int
	var ridesM int64
	for _, rd := range all {
		if rd.BikeID == bike.ID && !rd.Discarded {
			rideCount++
			ridesM += rd.DistanceM
		}
	}
	views, err := s.componentViews(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	installed := installedOn(i18n.FromContext(ctx), views, bike.ID)
	// Alerts are the components at a limit, Worn up to three others that
	// used most of their distance limit.
	var alerts, worn []componentView
	for _, v := range installed {
		switch {
		case v.Wear.Alert():
			alerts = append(alerts, v)
		case v.Wear.Percent >= 0:
			worn = append(worn, v)
		}
	}
	slices.SortStableFunc(worn, func(a, b componentView) int { return b.Wear.Percent - a.Wear.Percent })
	worn = worn[:min(len(worn), 3)]
	maintenances, err := s.bikeMaintenances(ctx, bike.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items, err := s.dueItems(ctx, bike)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	target, err := s.bikeTarget(ctx, bike.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	files, err := s.store.Attachments().ListBikeAttachments(ctx, ownerID(ctx), bike.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vs, err := s.store.Bikes().ListBikeValues(ctx, ownerID(ctx), bike.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var open *maintenanceSummary
	for i := range maintenances {
		if maintenances[i].Open() {
			open = &maintenances[i]
			break
		}
	}
	s.render(w, r, status, "bike.html", map[string]any{
		"Open":         open,
		"Target":       target,
		"Attachments":  files,
		"Cover":        coverOf(files),
		"Due":          items,
		"Pending":      len(due.Pending(items)),
		"Bike":         bike,
		"Readings":     readings,
		"Values":       vs,
		"Current":      current,
		"RideCount":    rideCount,
		"RidesM":       ridesM,
		"Installed":    len(installed),
		"Alerts":       alerts,
		"Worn":         worn,
		"Maintenances": maintenances,
		"Error":        errMsg,
	})
}

func (s *Server) newReading(w http.ResponseWriter, r *http.Request) {
	s.renderNewReading(w, r, http.StatusOK, "")
}

func (s *Server) renderNewReading(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	bike, err := s.store.Bikes().GetBike(r.Context(), ownerID(r.Context()), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, status, "reading_new.html", map[string]any{"Bike": bike, "Error": errMsg})
}

func (s *Server) addReading(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	date, ok := parseDate(r.FormValue("date"))
	if !ok {
		s.renderNewReading(w, r, http.StatusUnprocessableEntity, l.T("error.invalid_date"))
		return
	}
	meters, ok := parseKm(r.FormValue("km"))
	if !ok {
		s.renderNewReading(w, r, http.StatusUnprocessableEntity, l.T("error.invalid_km"))
		return
	}
	bikeID := r.PathValue("id")
	_, err := odometer.Record(r.Context(), s.store, store.OdometerReading{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID(r.Context()), BikeID: bikeID, RecordedAt: date,
		DistanceM: meters, Source: store.SourceManual, CreatedAt: time.Now().UTC(),
	})
	if msg, ok := odometerErrorMessage(l, err); ok {
		s.renderNewReading(w, r, http.StatusUnprocessableEntity, msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bikes/"+bikeID, http.StatusSeeOther)
}
