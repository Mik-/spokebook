package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/odometer"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/values"
)

// componentView is a component with its wear and current installation.
type componentView struct {
	store.Component
	TypeName string
	Wear     components.Wear
	Current  *store.Installation
	BikeName string
	// LastBikeID is the bike of the latest installation, empty if never installed.
	LastBikeID string
}

// installationView is an installation with the distance covered in it.
type installationView struct {
	store.Installation
	BikeName  string
	DistanceM int64
}

// componentViews loads all components of the owner with wear and current
// installation, keyed by component ID.
func (s *Server) componentViews(ctx context.Context) (map[string]componentView, error) {
	comps, err := s.store.Components().ListComponents(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	insts, err := s.store.Components().ListAllInstallations(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	latest, err := odometer.Latest(ctx, s.store, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	names, err := s.bikeNames(ctx)
	if err != nil {
		return nil, err
	}
	typeNames, _, err := s.componentTypes(ctx)
	if err != nil {
		return nil, err
	}
	byComp := make(map[string][]store.Installation)
	for _, i := range insts {
		byComp[i.ComponentID] = append(byComp[i.ComponentID], i)
	}
	now := time.Now().UTC()
	views := make(map[string]componentView, len(comps))
	for _, c := range comps {
		v := componentView{
			Component: c,
			TypeName:  typeNames[c.Type],
			Wear:      components.WearOf(c, components.Mileage(byComp[c.ID], latest), now),
		}
		for _, i := range byComp[c.ID] {
			v.LastBikeID = i.BikeID
			if i.Open() {
				v.Current = &i
				v.BikeName = names[i.BikeID]
			}
		}
		views[c.ID] = v
	}
	return views, nil
}

func (s *Server) bikeNames(ctx context.Context) (map[string]string, error) {
	bikes, err := s.store.Bikes().ListBikes(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(bikes))
	for _, b := range bikes {
		names[b.ID] = b.Name
	}
	return names, nil
}

// fromBike returns the bike named by the "bike" parameter, or nil if it is
// missing or unknown. Component pages opened from a bike's component list
// carry it, so their breadcrumbs and redirects lead back to that list.
func (s *Server) fromBike(r *http.Request) (*store.Bike, error) {
	id := r.FormValue("bike")
	if id == "" {
		return nil, nil
	}
	b, err := s.store.Bikes().GetBike(r.Context(), ownerID(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// bikeQuery is the query that keeps b as the bike context of a component page.
func bikeQuery(b *store.Bike) string {
	if b == nil {
		return ""
	}
	return "?bike=" + url.QueryEscape(b.ID)
}

// typeOption is a component type for a select box.
type typeOption struct {
	ID, Name string
	Builtin  bool
	// Custom is the own name; empty for a built-in type that was not renamed.
	Custom string
}

func typeName(l *i18n.Localizer, t store.ComponentType) string {
	if t.Name != "" {
		return t.Name
	}
	return l.T("component.type." + t.ID)
}

// componentTypes returns the display names of all types (including hidden
// ones, for existing components) and the active types sorted by name.
func (s *Server) componentTypes(ctx context.Context) (map[string]string, []typeOption, error) {
	l := i18n.FromContext(ctx)
	types, err := s.store.Components().ListTypes(ctx, ownerID(ctx))
	if err != nil {
		return nil, nil, err
	}
	names := make(map[string]string, len(types))
	var active []typeOption
	for _, t := range types {
		names[t.ID] = typeName(l, t)
		if !t.Hidden {
			active = append(active, typeOption{ID: t.ID, Name: names[t.ID], Builtin: components.IsBuiltin(t.ID), Custom: t.Name})
		}
	}
	slices.SortFunc(active, func(a, b typeOption) int { return l.Compare(a.Name, b.Name) })
	return names, active, nil
}

// sortedViews returns the views sorted by type name, manufacturer and model.
func sortedViews(l *i18n.Localizer, views map[string]componentView) []componentView {
	out := make([]componentView, 0, len(views))
	for _, v := range views {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b componentView) int {
		if c := l.Compare(a.TypeName, b.TypeName); c != 0 {
			return c
		}
		if c := l.Compare(a.Manufacturer, b.Manufacturer); c != 0 {
			return c
		}
		return l.Compare(a.Model, b.Model)
	})
	return out
}

// filterOption is a choice of a filter select.
type filterOption struct{ Value, Label string }

// listComponents shows the active components, filtered by the bike they
// are installed on ("none" for spares) and by type, plus the retired ones,
// filtered by type and the bike they were last installed on.
// The filters offer only bikes and types that occur.
func (s *Server) listComponents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	views, err := s.componentViews(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := i18n.FromContext(ctx)
	bikeF, typeF := r.URL.Query().Get("bike"), r.URL.Query().Get("type")
	var active, retired []componentView
	var bikes, types []filterOption
	seen := make(map[string]bool)
	matches := func(v componentView, bikeID string) bool {
		switch {
		case typeF != "" && v.Type != typeF:
		case bikeF == "none" && bikeID != "":
		case bikeF != "" && bikeF != "none" && bikeID != bikeF:
		default:
			return true
		}
		return false
	}
	for _, v := range sortedViews(l, views) {
		if !v.Active() {
			if matches(v, v.LastBikeID) {
				retired = append(retired, v)
			}
			continue
		}
		if v.Current != nil && !seen["bike:"+v.Current.BikeID] {
			seen["bike:"+v.Current.BikeID] = true
			bikes = append(bikes, filterOption{v.Current.BikeID, v.BikeName})
		}
		if !seen["type:"+v.Type] {
			seen["type:"+v.Type] = true
			types = append(types, filterOption{v.Type, v.TypeName})
		}
		bikeID := ""
		if v.Current != nil {
			bikeID = v.Current.BikeID
		}
		if matches(v, bikeID) {
			active = append(active, v)
		}
	}
	slices.SortFunc(bikes, func(a, b filterOption) int { return l.Compare(a.Label, b.Label) })
	s.render(w, r, http.StatusOK, "components.html", map[string]any{
		"Components": active, "Retired": retired,
		"Bikes": bikes, "Types": types, "Bike": bikeF, "Type": typeF, "Filtered": bikeF != "" || typeF != "",
	})
}

// componentFormData returns what the component form needs: the active types
// and suggestions for manufacturer, dealer and values.
func (s *Server) componentFormData(ctx context.Context) (map[string]any, error) {
	_, types, err := s.componentTypes(ctx)
	if err != nil {
		return nil, err
	}
	manufacturers, err := s.store.Components().ListManufacturers(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	dealers, err := s.store.Components().ListDealers(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	data, err := s.valueSuggestions(ctx)
	if err != nil {
		return nil, err
	}
	data["Types"] = types
	data["Manufacturers"] = manufacturers
	data["Dealers"] = dealers
	return data, nil
}

func (s *Server) createComponent(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	c := store.Component{ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID(r.Context()), CreatedAt: time.Now().UTC()}
	vs, errID, err := s.parseComponentForm(r, &c)
	if err != nil {
		s.fail(w, r, err)
		return
	} else if errID != "" {
		s.renderNewComponent(w, r, http.StatusUnprocessableEntity, r.FormValue("duplicate_of"), l.T(errID))
		return
	}
	err = components.Create(r.Context(), s.store, c, vs)
	if msg, ok := valueErrorMessage(l, err); ok {
		s.renderNewComponent(w, r, http.StatusUnprocessableEntity, r.FormValue("duplicate_of"), msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/components/"+c.ID, http.StatusSeeOther)
}

// parseComponentForm sets the form fields on c and returns the values. It
// returns the message ID of a validation error, or an error if the types
// can't be loaded.
func (s *Server) parseComponentForm(r *http.Request, c *store.Component) ([]store.Value, string, error) {
	errID, err := s.parseComponentFields(r, c)
	if errID != "" || err != nil {
		return nil, errID, err
	}
	vs, errID := parseValues(r)
	return vs, errID, nil
}

func (s *Server) parseComponentFields(r *http.Request, c *store.Component) (string, error) {
	c.Type = r.FormValue("type")
	c.Manufacturer = strings.TrimSpace(r.FormValue("manufacturer"))
	c.Model = strings.TrimSpace(r.FormValue("model"))
	c.PartNumber = strings.TrimSpace(r.FormValue("part_number"))
	c.Dealer = strings.TrimSpace(r.FormValue("dealer"))
	c.Notes = strings.TrimSpace(r.FormValue("notes"))
	if _, err := components.ActiveType(r.Context(), s.store, ownerID(r.Context()), c.Type); errors.Is(err, components.ErrInvalidType) {
		return "error.invalid_type", nil
	} else if err != nil {
		return "", err
	}
	var ok bool
	if c.PurchasedAt, ok = optional(r.FormValue("purchased_at"), parseDate); !ok {
		return "error.invalid_date", nil
	}
	if c.ReplaceBy, ok = optional(r.FormValue("replace_by"), parseDate); !ok {
		return "error.invalid_date", nil
	}
	if c.PriceCents, ok = optional(r.FormValue("price"), parseCents); !ok {
		return "error.invalid_price", nil
	}
	c.Currency = ""
	if c.PriceCents != nil {
		if c.Currency, ok = parseCurrency(r.FormValue("currency")); !ok {
			return "error.invalid_currency", nil
		}
	}
	if c.MaxDistanceM, ok = optional(r.FormValue("max_km"), parseKm); !ok || (c.MaxDistanceM != nil && *c.MaxDistanceM == 0) {
		return "error.invalid_max_km", nil
	}
	c.URL = ""
	if u := strings.TrimSpace(r.FormValue("url")); u != "" {
		if c.URL, ok = parseURL(u); !ok {
			return "error.invalid_url", nil
		}
	}
	return "", nil
}

func (s *Server) showComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, err := s.oneComponentView(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	insts, err := s.store.Components().ListInstallations(ctx, ownerID(ctx), v.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	latest, err := odometer.Latest(ctx, s.store, ownerID(ctx))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names, err := s.bikeNames(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	history := make([]installationView, 0, len(insts))
	for _, i := range insts {
		history = append(history, installationView{
			Installation: i,
			BikeName:     names[i.BikeID],
			DistanceM:    components.Mileage([]store.Installation{i}, latest),
		})
	}
	vs, err := s.effectiveValues(ctx, v.Component)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from, err := s.fromBike(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	files, err := s.store.Attachments().ListComponentAttachments(ctx, ownerID(ctx), v.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	target := attachTarget{AttachmentLink: store.AttachmentLink{ComponentID: v.ID}, Name: displayName(v), Path: "/components/" + v.ID}
	s.render(w, r, http.StatusOK, "component.html", map[string]any{
		"C": v, "Values": vs, "Installations": history, "From": from, "FromQuery": bikeQuery(from),
		"Target": target, "Attachments": files,
	})
}

// effectiveValue is a value that applies to a component; FromType is set
// for a default of its type.
type effectiveValue struct {
	store.Value
	FromType bool
}

// effectiveValues returns the values that apply to c (see values.Effective).
func (s *Server) effectiveValues(ctx context.Context, c store.Component) ([]effectiveValue, error) {
	defaults, err := s.store.Components().ListTypeValues(ctx, ownerID(ctx), c.Type)
	if err != nil {
		return nil, err
	}
	own, err := s.store.Components().ListComponentValues(ctx, ownerID(ctx), c.ID)
	if err != nil {
		return nil, err
	}
	fromType := make(map[string]bool, len(defaults))
	for _, d := range defaults {
		fromType[d.ID] = true
	}
	var out []effectiveValue
	for _, v := range values.Effective(defaults, own) {
		out = append(out, effectiveValue{Value: v, FromType: fromType[v.ID]})
	}
	return out, nil
}

func (s *Server) installComponent(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	invalid := func(msg string) { s.renderInstallForm(w, r, http.StatusUnprocessableEntity, msg) }

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
	_, err := components.Install(r.Context(), s.store, store.Installation{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID(r.Context()), ComponentID: id,
		BikeID: r.FormValue("bike"), Position: r.FormValue("position"), InstalledAt: date, CreatedAt: time.Now().UTC(),
	}, km, r.FormValue("allow_duplicate") != "")
	if msg, ok := s.duplicateWarning(r, err); ok {
		invalid(msg)
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
	http.Redirect(w, r, "/components/"+id, http.StatusSeeOther)
}

func (s *Server) removeComponent(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	invalid := func(msg string) { s.renderRemoveForm(w, r, http.StatusUnprocessableEntity, msg) }

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
	err := components.Remove(r.Context(), s.store, ownerID(r.Context()), r.FormValue("installation"), date, km)
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

// installErrorMessage maps validation errors from the components package,
// including those of an entered odometer.
func installErrorMessage(l *i18n.Localizer, err error) (string, bool) {
	if msg, ok := odometerErrorMessage(l, err); ok {
		return msg, true
	}
	for e, id := range map[error]string{
		components.ErrAlreadyInstalled:   "error.already_installed",
		components.ErrOverlap:            "error.install_overlap",
		components.ErrNoOdometer:         "error.no_odometer",
		components.ErrBeforeInstallation: "error.removal_before_installation",
		components.ErrInvalidPosition:    "error.invalid_position",
		components.ErrIncompleteRemoval:  "error.incomplete_removal",
		components.ErrRetired:            "error.retired",
		components.ErrInvalidReason:      "error.invalid_reason",
	} {
		if errors.Is(err, e) {
			return l.T(id), true
		}
	}
	return "", false
}
