// Package server contains the HTTP handlers.
package server

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/auth"
	"github.com/spokebook/spokebook/internal/app/spokebook/catalog"
	"github.com/spokebook/spokebook/internal/app/spokebook/i18n"
	"github.com/spokebook/spokebook/internal/app/spokebook/rides/garmin"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/web"
)

// Files in web/templates that are not pages but parsed into every page.
var sharedTemplates = []string{"layout.html", "partials.html"}

// Config configures the server.
type Config struct {
	// BaseURL is the address users open, e.g. "https://bikes.example.com".
	// Passkeys are bound to its host; with https, cookies are Secure and
	// HSTS is sent.
	BaseURL string
	// TrustedProxies are the reverse proxies whose X-Forwarded-For header
	// tells the client address, see ParseTrustedProxies.
	TrustedProxies []netip.Prefix
	// Version is the app version shown in the footer.
	Version string
	// SourceURL is the link to the source code in the footer.
	SourceURL string
	// Garmin fetches rides from Garmin Connect; nil disables it.
	Garmin *garmin.Syncer
}

type Server struct {
	store    store.Store
	i18n     *i18n.Bundle
	pages    map[string]*template.Template
	log      *slog.Logger
	setup    *auth.Setup
	passkeys *auth.Passkeys
	secure   bool
	// loginLimit throttles login and setup attempts per client.
	loginLimit     *limiter
	trustedProxies []netip.Prefix
	version        string
	sourceURL      string
	garmin         *garmin.Syncer
}

// New creates the server and adds new built-in component types and steps
// for every user. Without an account, it logs the link to set one up.
func New(ctx context.Context, st store.Store, log *slog.Logger, cfg Config) (*Server, error) {
	users, err := st.Users().ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if err := auth.Seed(ctx, st, u.ID); err != nil {
			return nil, err
		}
	}
	setup, err := auth.NewSetup(ctx, st)
	if err != nil {
		return nil, err
	}
	if token := setup.Token(); token != "" {
		log.Warn("no account yet, open this link to create one", "url", cfg.BaseURL+"/setup?token="+token)
	}
	passkeys, err := auth.NewPasskeys(st, cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	bundle, err := i18n.Load(web.Locales, "locales")
	if err != nil {
		return nil, err
	}
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	return &Server{
		store: st, i18n: bundle, pages: pages, log: log, setup: setup, passkeys: passkeys,
		secure: strings.HasPrefix(cfg.BaseURL, "https://"), loginLimit: newLimiter(10, 6*time.Second),
		trustedProxies: cfg.TrustedProxies, version: cfg.Version, sourceURL: cfg.SourceURL, garmin: cmp.Or(cfg.Garmin, garmin.NewSyncer(st, nil, nil, log)),
	}, nil
}

// parsePages parses every page together with the shared templates, keyed by
// file name. The funcs are placeholders; render binds them to the request's
// language.
func parsePages() (map[string]*template.Template, error) {
	files, err := fs.Glob(web.Templates, "templates/*.html")
	if err != nil {
		return nil, err
	}
	var shared []string
	for _, f := range sharedTemplates {
		shared = append(shared, "templates/"+f)
	}
	pages := make(map[string]*template.Template)
	for _, f := range files {
		name := path.Base(f)
		if slices.Contains(sharedTemplates, name) {
			continue
		}
		tmpl, err := template.New(name).Funcs(template.FuncMap{
			"t":        func(string, ...any) string { return "" },
			"tn":       func(string, int, ...any) string { return "" },
			"lang":     func() string { return "" },
			"km":       func(int64) string { return "" },
			"date":     func(time.Time) string { return "" },
			"money":    func(int64, string) string { return "" },
			"value":    func(store.Value) string { return "" },
			"num":      func(float64) string { return "" },
			"gearName": func(string, string) string { return "" },
			// Not language dependent, so render keeps them.
			"valueRows":     valueRows,
			"valueKinds":    func() []string { return store.ValueKinds },
			"stepKinds":     func() []string { return store.StepValueKinds },
			"levels":        func() []string { return catalog.Levels },
			"followUpRows":  followUpRows,
			"followUpKinds": func() []string { return store.FollowUpKinds },
			"has":           func(list []string, s string) bool { return slices.Contains(list, s) },
			"dict":          dict,
			"deref":         func(p *int64) int64 { return *p },
		}).ParseFS(web.Templates, append(shared, f)...)
		if err != nil {
			return nil, err
		}
		pages[name] = tmpl
	}
	return pages, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.listBikes)
	mux.HandleFunc("GET /bikes/new", s.newBike)
	mux.HandleFunc("POST /bikes", s.createBike)
	mux.HandleFunc("GET /bikes/{id}", s.showBike)
	mux.HandleFunc("GET /bikes/{id}/odometer/new", s.newReading)
	mux.HandleFunc("POST /bikes/{id}/odometer", s.addReading)
	mux.HandleFunc("GET /components", s.listComponents)
	mux.HandleFunc("GET /components/new", s.newComponent)
	mux.HandleFunc("POST /components", s.createComponent)
	mux.HandleFunc("GET /components/{id}", s.showComponent)
	mux.HandleFunc("GET /components/{id}/install", s.installForm)
	mux.HandleFunc("POST /components/{id}/install", s.installComponent)
	mux.HandleFunc("GET /bikes/{id}/components", s.bikeComponents)
	mux.HandleFunc("GET /bikes/{id}/components/new", s.addToBikeForm)
	mux.HandleFunc("POST /bikes/{id}/components", s.addToBike)
	mux.HandleFunc("GET /bikes/{id}/components/install", s.installSpareForm)
	mux.HandleFunc("POST /bikes/{id}/components/install", s.installSpare)
	mux.HandleFunc("GET /components/{id}/replace", s.replaceForm)
	mux.HandleFunc("POST /components/{id}/replace", s.replaceComponent)
	mux.HandleFunc("GET /components/{id}/remove", s.removeForm)
	mux.HandleFunc("POST /components/{id}/remove", s.removeComponent)
	mux.HandleFunc("GET /bikes/{id}/values", s.editBikeValues)
	mux.HandleFunc("POST /bikes/{id}/values", s.updateBikeValues)
	mux.HandleFunc("GET /bikes/{id}/edit", s.editBike)
	mux.HandleFunc("POST /bikes/{id}", s.updateBike)
	mux.HandleFunc("GET /bikes/{id}/delete", s.confirmDeleteBike)
	mux.HandleFunc("POST /bikes/{id}/delete", s.deleteBike)
	mux.HandleFunc("GET /bikes/{id}/odometer/{rid}/edit", s.editReading)
	mux.HandleFunc("POST /bikes/{id}/odometer/{rid}", s.updateReading)
	mux.HandleFunc("GET /bikes/{id}/odometer/{rid}/delete", s.confirmDeleteReading)
	mux.HandleFunc("POST /bikes/{id}/odometer/{rid}/delete", s.deleteReading)
	mux.HandleFunc("GET /components/{id}/edit", s.editComponent)
	mux.HandleFunc("GET /components/{id}/duplicate", s.duplicateComponent)
	mux.HandleFunc("POST /components/{id}", s.updateComponent)
	mux.HandleFunc("GET /components/{id}/delete", s.confirmDeleteComponent)
	mux.HandleFunc("POST /components/{id}/delete", s.deleteComponent)
	mux.HandleFunc("GET /installations/{id}/edit", s.editInstallation)
	mux.HandleFunc("POST /installations/{id}", s.updateInstallation)
	mux.HandleFunc("GET /installations/{id}/delete", s.confirmDeleteInstallation)
	mux.HandleFunc("POST /installations/{id}/delete", s.deleteInstallation)
	mux.HandleFunc("GET /bikes/{id}/retire", s.retireBikeForm)
	mux.HandleFunc("POST /bikes/{id}/retire", s.retireBike)
	mux.HandleFunc("POST /bikes/{id}/reactivate", s.reactivateBike)
	mux.HandleFunc("GET /components/{id}/retire", s.retireComponentForm)
	mux.HandleFunc("POST /components/{id}/retire", s.retireComponent)
	mux.HandleFunc("POST /components/{id}/reactivate", s.reactivateComponent)
	mux.HandleFunc("GET /component-types", s.listTypes)
	mux.HandleFunc("GET /component-types/new", s.newType)
	mux.HandleFunc("POST /component-types", s.createType)
	mux.HandleFunc("POST /component-types/{id}", s.renameType)
	mux.HandleFunc("POST /component-types/{id}/delete", s.deleteType)
	mux.HandleFunc("POST /component-types/{id}/restore", s.restoreType)
	mux.HandleFunc("GET /component-types/{id}/values", s.editTypeValues)
	mux.HandleFunc("POST /component-types/{id}/values", s.updateTypeValues)
	mux.HandleFunc("GET /steps", s.listSteps)
	mux.HandleFunc("GET /steps/new", s.newStep)
	mux.HandleFunc("GET /steps/modules", s.listModules)
	mux.HandleFunc("GET /steps/{id}", s.showStep)
	mux.HandleFunc("POST /steps", s.createStep)
	mux.HandleFunc("GET /steps/{id}/edit", s.editStep)
	mux.HandleFunc("POST /steps/{id}", s.updateStep)
	mux.HandleFunc("GET /steps/{id}/delete", s.confirmDeleteStep)
	mux.HandleFunc("POST /steps/{id}/delete", s.deleteStep)
	mux.HandleFunc("POST /steps/{id}/up", s.moveStep(-1))
	mux.HandleFunc("POST /steps/{id}/down", s.moveStep(1))
	mux.HandleFunc("POST /steps/{id}/restore", s.restoreStep)
	mux.HandleFunc("GET /shopping", s.listShopping)
	mux.HandleFunc("GET /shopping/new", s.newShoppingItem)
	mux.HandleFunc("POST /shopping", s.createShoppingItem)
	mux.HandleFunc("GET /shopping/{id}/edit", s.editShoppingItem)
	mux.HandleFunc("POST /shopping/{id}", s.updateShoppingItem)
	mux.HandleFunc("POST /shopping/{id}/bought", s.setShoppingItemBought(true))
	mux.HandleFunc("POST /shopping/{id}/open", s.setShoppingItemBought(false))
	mux.HandleFunc("GET /shopping/{id}/delete", s.confirmDeleteShoppingItem)
	mux.HandleFunc("POST /shopping/{id}/delete", s.deleteShoppingItem)
	mux.HandleFunc("GET /shopping/bought/delete", s.confirmDeleteBought)
	mux.HandleFunc("POST /shopping/bought/delete", s.deleteBought)
	mux.HandleFunc("GET /intervals", s.editIntervals)
	mux.HandleFunc("POST /intervals", s.updateIntervals)
	mux.HandleFunc("GET /bikes/{id}/maintenances/new", s.newMaintenance)
	mux.HandleFunc("POST /bikes/{id}/maintenances", s.startMaintenance)
	mux.HandleFunc("GET /maintenances/{id}", s.showMaintenance)
	mux.HandleFunc("POST /maintenances/{id}", s.saveMaintenance)
	mux.HandleFunc("POST /maintenances/{id}/reopen", s.reopenMaintenance)
	mux.HandleFunc("GET /maintenances/{id}/steps", s.addStepsForm)
	mux.HandleFunc("POST /maintenances/{id}/steps", s.addSteps)
	mux.HandleFunc("POST /maintenances/{id}/follow-up", s.addFollowUp)
	mux.HandleFunc("POST /maintenances/{id}/steps/{step}/dismiss", s.dismissStep)
	mux.HandleFunc("POST /maintenances/{id}/steps/{step}/remove", s.removeStep)
	mux.HandleFunc("GET /maintenances/{id}/edit", s.editMaintenance)
	mux.HandleFunc("POST /maintenances/{id}/edit", s.updateMaintenance)
	mux.HandleFunc("GET /maintenances/{id}/delete", s.confirmDeleteMaintenance)
	mux.HandleFunc("POST /maintenances/{id}/delete", s.deleteMaintenance)
	mux.HandleFunc("GET /rides", s.listRides)
	mux.HandleFunc("GET /rides/import", s.importRidesForm)
	mux.HandleFunc("POST /rides/import", s.previewRides)
	mux.HandleFunc("POST /rides/import/confirm", s.importRides)
	mux.HandleFunc("GET /rides/filtered/discard", s.confirmBulkRides("discard"))
	mux.HandleFunc("POST /rides/filtered/discard", s.bulkRides("discard"))
	mux.HandleFunc("GET /rides/filtered/delete", s.confirmBulkRides("delete"))
	mux.HandleFunc("POST /rides/filtered/delete", s.bulkRides("delete"))
	mux.HandleFunc("GET /rides/{id}/edit", s.editRide)
	mux.HandleFunc("POST /rides/{id}", s.updateRide)
	mux.HandleFunc("POST /rides/{id}/discard", s.setRideDiscarded(true))
	mux.HandleFunc("POST /rides/{id}/restore", s.setRideDiscarded(false))
	mux.HandleFunc("GET /rides/{id}/delete", s.confirmDeleteRide)
	mux.HandleFunc("POST /rides/{id}/delete", s.deleteRide)
	mux.HandleFunc("GET /rides/garmin", s.garminPage)
	mux.HandleFunc("GET /rides/garmin/connect", s.garminConnectForm)
	mux.HandleFunc("POST /rides/garmin/connect", s.garminConnect)
	mux.HandleFunc("POST /rides/garmin/sync", s.garminSync)
	mux.HandleFunc("GET /rides/garmin/disconnect", s.confirmGarminDisconnect)
	mux.HandleFunc("POST /rides/garmin/disconnect", s.garminDisconnect)
	mux.HandleFunc("GET /ride-gears", s.listGears)
	mux.HandleFunc("GET /ride-gears/{id}/edit", s.editGear)
	mux.HandleFunc("POST /ride-gears/{id}", s.updateGear)
	mux.HandleFunc("GET /ride-gears/{id}/delete", s.confirmDeleteGear)
	mux.HandleFunc("POST /ride-gears/{id}/delete", s.deleteGear)
	mux.HandleFunc("GET /account", s.showAccount)
	mux.HandleFunc("GET /account/password", s.passwordForm)
	mux.HandleFunc("POST /account/password", s.changePassword)
	mux.HandleFunc("GET /account/passkeys/new", s.newPasskey)
	mux.HandleFunc("POST /account/passkeys/begin", s.beginPasskeyRegistration)
	mux.HandleFunc("POST /account/passkeys/finish", s.finishPasskeyRegistration)
	mux.HandleFunc("GET /account/passkeys/{id}/delete", s.confirmDeletePasskey)
	mux.HandleFunc("POST /account/passkeys/{id}/delete", s.deletePasskey)
	mux.HandleFunc("GET /account/backup", s.exportBackup)
	mux.HandleFunc("GET /account/backup/import", s.importBackupForm)
	mux.HandleFunc("POST /account/backup/import", s.importBackup)
	mux.HandleFunc("GET /attachments/{id}", s.attachmentFile)
	mux.HandleFunc("GET /attachments/{id}/thumb", s.attachmentThumb)
	mux.HandleFunc("GET /attachments/{id}/edit", s.editAttachment)
	mux.HandleFunc("POST /attachments/{id}", s.updateAttachment)
	mux.HandleFunc("POST /attachments/{id}/unlink", s.unlinkAttachment)
	mux.HandleFunc("GET /attachments/{id}/delete", s.confirmDeleteAttachment)
	mux.HandleFunc("POST /attachments/{id}/delete", s.deleteAttachment)
	mux.HandleFunc("GET /bikes/{id}/attachments/new", s.newAttachments)
	mux.HandleFunc("POST /bikes/{id}/attachments", s.addAttachments)
	mux.HandleFunc("GET /bikes/{id}/attachments/link", s.linkAttachmentsForm)
	mux.HandleFunc("POST /bikes/{id}/attachments/link", s.linkAttachments)
	mux.HandleFunc("POST /bikes/{id}/cover", s.setCover)
	mux.HandleFunc("GET /components/{id}/attachments/new", s.newAttachments)
	mux.HandleFunc("POST /components/{id}/attachments", s.addAttachments)
	mux.HandleFunc("GET /components/{id}/attachments/link", s.linkAttachmentsForm)
	mux.HandleFunc("POST /components/{id}/attachments/link", s.linkAttachments)
	mux.HandleFunc("POST /logout", s.logout)

	// Pages without login; everything else requires it.
	public := http.NewServeMux()
	public.Handle("GET /static/", http.FileServerFS(web.Static))
	public.HandleFunc("GET /sw.js", s.serviceWorker)
	public.HandleFunc("GET /offline", s.offline)
	public.HandleFunc("GET /login", s.loginForm)
	public.HandleFunc("POST /login", s.login)
	public.HandleFunc("POST /login/passkey/begin", s.beginPasskeyLogin)
	public.HandleFunc("POST /login/passkey/finish", s.finishPasskeyLogin)
	public.HandleFunc("GET /setup", s.setupForm)
	public.HandleFunc("POST /setup", s.completeSetup)
	public.Handle("/", s.requireLogin(mux))

	// CSRF protection for all state-changing requests.
	return s.logRequests(s.secureHeaders(http.NewCrossOriginProtection().Handler(s.localize(public))))
}

// localize stores the Localizer for the request's preferred language.
func (s *Server) localize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := s.i18n.Localizer(r.Header.Get("Accept-Language"))
		w.Header().Set("Content-Language", l.Lang())
		w.Header().Add("Vary", "Accept-Language")
		next.ServeHTTP(w, r.WithContext(i18n.WithLocalizer(r.Context(), l)))
	})
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// data: images are the icons Pico CSS embeds; scripts stay 'self' only.
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		if s.secure {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// render executes a page. data gets "Today" and, unless set (e.g. to
// prefill an edit form), "Form": the submitted form, so fields keep their
// values after a validation error.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page string, data map[string]any) {
	buf, err := s.execute(r, page, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// execute renders a page into a buffer, so a template error doesn't leave a
// half page.
func (s *Server) execute(r *http.Request, page string, data map[string]any) (*bytes.Buffer, error) {
	l := i18n.FromContext(r.Context())
	if _, ok := data["Form"]; !ok {
		form := r.PostForm
		if form == nil {
			form = url.Values{}
		}
		data["Form"] = form
	}
	data["Today"] = time.Now().UTC().Format(time.DateOnly)
	data["Section"] = section(r.URL.Path)
	data["SourceURL"] = s.sourceURL
	if u, ok := currentUser(r.Context()); ok {
		data["User"] = u
		data["Version"] = s.version
	}

	// The parsed sets are never executed directly, so Clone always succeeds.
	tmpl := template.Must(s.pages[page].Clone()).Funcs(template.FuncMap{
		"t": l.T, "tn": l.N, "lang": l.Lang, "km": l.Distance, "date": l.Date, "money": l.Money,
		"value":    func(v store.Value) string { return formatValue(l, v) },
		"num":      l.Number,
		"gearName": func(key, name string) string { return gearName(l, key, name) },
	})
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		return nil, fmt.Errorf("render %s: %w", page, err)
	}
	return &buf, nil
}

// dict builds a map from key/value pairs, to pass several values to a
// template.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, errors.New("dict: odd number of arguments")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", pairs[i])
		}
		m[k] = pairs[i+1]
	}
	return m, nil
}

// section names the navigation entry a path belongs to.
func section(path string) string {
	for _, prefix := range []string{"/components", "/component-types", "/installations"} {
		if strings.HasPrefix(path, prefix) {
			return "components"
		}
	}
	if strings.HasPrefix(path, "/steps") || strings.HasPrefix(path, "/intervals") {
		return "steps"
	}
	if strings.HasPrefix(path, "/rides") || strings.HasPrefix(path, "/ride-gears") {
		return "rides"
	}
	if strings.HasPrefix(path, "/shopping") {
		return "shopping"
	}
	if strings.HasPrefix(path, "/account") {
		return "account"
	}
	return "bikes"
}

// fail answers with 404 for store.ErrNotFound and 500 otherwise.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	l := i18n.FromContext(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		s.errorPage(w, r, http.StatusNotFound, l.T("error.not_found"))
		return
	}
	s.log.Error("request", "err", err)
	s.errorPage(w, r, http.StatusInternalServerError, l.T("error.internal"))
}

// errorPage shows message on a page with the menu. If even that page fails,
// the message goes out as plain text.
func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, status int, message string) {
	buf, err := s.execute(r, "error.html", map[string]any{"Message": message})
	if err != nil {
		s.log.Error("request", "err", err)
		http.Error(w, message, status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// confirm shows a page asking to confirm a deletion; the form posts to action.
func (s *Server) confirm(w http.ResponseWriter, r *http.Request, message, action, cancel string) {
	s.render(w, r, http.StatusOK, "confirm.html", map[string]any{
		"Message": message, "Action": action, "Cancel": cancel,
	})
}
