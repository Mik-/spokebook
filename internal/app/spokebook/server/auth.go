package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Mik-/spokebook/internal/app/spokebook/auth"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

const sessionCookie = "spokebook_session"

// maxJSONBody limits the browser's passkey answers.
const maxJSONBody = 64 << 10

// session returns the user and session of the request's cookie. ok is
// false without a valid session.
func (s *Server) session(w http.ResponseWriter, r *http.Request) (u store.User, sess store.Session, ok bool, err error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return store.User{}, store.Session{}, false, nil
	}
	u, sess, extended, err := auth.Authenticate(r.Context(), s.store, c.Value)
	if errors.Is(err, auth.ErrNoSession) {
		return store.User{}, store.Session{}, false, nil
	}
	if err != nil {
		return store.User{}, store.Session{}, false, err
	}
	if extended {
		s.setSessionCookie(w, c.Value)
	}
	return u, sess, true, nil
}

// requireLogin lets only logged-in users through; others go to the login
// page, or to setup while there is no account.
func (s *Server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, sess, ok, err := s.session(w, r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if ok {
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u, sess)))
			return
		}
		if s.setup.Pending() {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		target := "/login"
		if r.Method == http.MethodGet && r.URL.Path != "/" {
			target += "?next=" + url.QueryEscape(r.URL.RequestURI())
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(auth.SessionLifetime.Seconds()),
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	})
}

// startSession logs the user in, replacing a session the browser had.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID string) error {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := auth.EndSession(r.Context(), s.store, c.Value); err != nil {
			return err
		}
	}
	token, _, err := auth.StartSession(r.Context(), s.store, userID)
	if err != nil {
		return err
	}
	s.setSessionCookie(w, token)
	return nil
}

// safeNext returns next if it is a path on this site, else "/". Browsers drop
// tabs and newlines from a URL, so "/\t/evil.example" would lead away;
// url.Parse rejects such control characters.
func safeNext(next string) string {
	if _, err := url.Parse(next); err != nil ||
		!strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// authError translates errors of package auth; "" means unexpected.
func authError(l *i18n.Localizer, err error) string {
	switch {
	case errors.Is(err, auth.ErrInvalidLogin):
		return l.T("auth.error.invalid_login")
	case errors.Is(err, auth.ErrLocked):
		return l.T("auth.error.locked", int(auth.LockDuration.Minutes()))
	case errors.Is(err, auth.ErrNameRequired):
		return l.T("error.name_required")
	case errors.Is(err, auth.ErrNameTooLong):
		return l.T("auth.error.name_too_long", auth.MaxNameLength)
	case errors.Is(err, auth.ErrNameTaken):
		return l.T("auth.error.name_taken")
	case errors.Is(err, auth.ErrPasswordTooShort):
		return l.T("auth.error.password_too_short", auth.MinPasswordLength)
	case errors.Is(err, auth.ErrPasswordTooLong):
		return l.T("auth.error.password_too_long", auth.MaxPasswordLength)
	case errors.Is(err, auth.ErrSetupToken):
		return l.T("auth.error.setup_token")
	case errors.Is(err, auth.ErrPasskey):
		return l.T("auth.error.passkey")
	case errors.Is(err, auth.ErrBusy):
		return l.T("auth.error.too_many_attempts")
	}
	return ""
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	if _, _, ok, err := s.session(w, r); err != nil || ok {
		if err != nil {
			s.fail(w, r, err)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if s.setup.Pending() {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, r, http.StatusOK, "")
}

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	next := r.FormValue("next")
	s.render(w, r, status, "login.html", map[string]any{"Next": safeNext(next), "Error": errMsg})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	if !s.loginLimit.allow(clientIP(r, s.trustedProxies)) {
		s.renderLogin(w, r, http.StatusTooManyRequests, l.T("auth.error.too_many_attempts"))
		return
	}
	u, err := auth.Login(r.Context(), s.store, r.FormValue("name"), r.FormValue("password"))
	if msg := authError(l, err); msg != "" {
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrLocked) {
			status = http.StatusTooManyRequests
		}
		s.renderLogin(w, r, status, msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := auth.EndSession(r.Context(), s.store, c.Value); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) setupForm(w http.ResponseWriter, r *http.Request) {
	if !s.setup.Pending() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "setup.html", map[string]any{"Form": r.URL.Query(), "MinPassword": auth.MinPasswordLength})
}

func (s *Server) completeSetup(w http.ResponseWriter, r *http.Request) {
	l := i18n.FromContext(r.Context())
	if !s.setup.Pending() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	invalid := func(status int, msg string) {
		s.render(w, r, status, "setup.html", map[string]any{"Error": msg, "MinPassword": auth.MinPasswordLength})
	}
	if !s.loginLimit.allow(clientIP(r, s.trustedProxies)) {
		invalid(http.StatusTooManyRequests, l.T("auth.error.too_many_attempts"))
		return
	}
	if r.FormValue("password") != r.FormValue("password_confirm") {
		invalid(http.StatusUnprocessableEntity, l.T("auth.error.password_mismatch"))
		return
	}
	u, err := s.setup.Complete(r.Context(), s.store, r.FormValue("token"), r.FormValue("name"), r.FormValue("password"))
	if errors.Is(err, auth.ErrSetupDone) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if msg := authError(l, err); msg != "" {
		invalid(http.StatusUnprocessableEntity, msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.log.Info("account created", "name", u.Name)
	if err := s.startSession(w, r, u.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// writeJSON answers a passkey request; raw is already encoded.
func writeJSON(w http.ResponseWriter, status int, raw []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(raw) //nolint:gosec // G705: JSON from go-webauthn, not HTML
}

// jsonResult answers the end of a passkey ceremony: where to go next, or
// the error to show.
func (s *Server) jsonResult(w http.ResponseWriter, r *http.Request, redirect string, err error) {
	var res struct {
		Redirect string `json:"redirect,omitempty"`
		Error    string `json:"error,omitempty"`
	}
	status := http.StatusOK
	if err != nil {
		l := i18n.FromContext(r.Context())
		if res.Error = authError(l, err); res.Error == "" {
			s.log.Error("request", "err", err)
			res.Error = l.T("error.internal")
			status = http.StatusInternalServerError
		} else {
			s.log.Info("passkey", "err", err)
			status = http.StatusBadRequest
		}
	} else {
		res.Redirect = redirect
	}
	raw, _ := json.Marshal(res)
	writeJSON(w, status, raw)
}

func (s *Server) beginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimit.allow(clientIP(r, s.trustedProxies)) {
		s.jsonResult(w, r, "", auth.ErrBusy)
		return
	}
	raw, err := s.passkeys.BeginLogin()
	if err != nil {
		s.jsonResult(w, r, "", err)
		return
	}
	writeJSON(w, http.StatusOK, raw)
}

func (s *Server) finishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimit.allow(clientIP(r, s.trustedProxies)) {
		s.jsonResult(w, r, "", auth.ErrBusy)
		return
	}
	u, err := s.passkeys.FinishLogin(r.Context(), http.MaxBytesReader(w, r.Body, maxJSONBody))
	if err == nil {
		err = s.startSession(w, r, u.ID)
	}
	s.jsonResult(w, r, safeNext(r.URL.Query().Get("next")), err)
}
