package server

import (
	"errors"
	"net/http"

	"github.com/Mik-/spokebook/internal/app/spokebook/auth"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
)

func (s *Server) showAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := currentUser(ctx)
	keys, err := s.store.Users().ListPasskeys(ctx, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "account.html", map[string]any{"Passkeys": keys})
}

func (s *Server) passwordForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "password_edit.html", map[string]any{"MinPassword": auth.MinPasswordLength})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	invalid := func(msg string) {
		s.render(w, r, http.StatusUnprocessableEntity, "password_edit.html", map[string]any{"Error": msg, "MinPassword": auth.MinPasswordLength})
	}
	if r.FormValue("password") != r.FormValue("password_confirm") {
		invalid(l.T("auth.error.password_mismatch"))
		return
	}
	err := auth.ChangePassword(ctx, s.store, ownerID(ctx), currentSession(ctx).ID,
		r.FormValue("current_password"), r.FormValue("password"))
	if errors.Is(err, auth.ErrInvalidLogin) {
		invalid(l.T("auth.error.wrong_password"))
		return
	}
	if msg := authError(l, err); msg != "" {
		invalid(msg)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (s *Server) newPasskey(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "passkey_new.html", map[string]any{})
}

func (s *Server) beginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := currentUser(ctx)
	raw, err := s.passkeys.BeginRegistration(ctx, u, r.FormValue("name"))
	if err != nil {
		s.jsonResult(w, r, "", err)
		return
	}
	writeJSON(w, http.StatusOK, raw)
}

func (s *Server) finishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, _ := currentUser(ctx)
	_, err := s.passkeys.FinishRegistration(ctx, u, http.MaxBytesReader(w, r.Body, maxJSONBody))
	s.jsonResult(w, r, "/account", err)
}

func (s *Server) confirmDeletePasskey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	k, err := s.store.Users().GetPasskey(ctx, ownerID(ctx), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := i18n.FromContext(ctx)
	s.confirm(w, r, l.T("account.passkey.delete.confirm", k.Name), "/account/passkeys/"+k.ID+"/delete", "/account")
}

func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.Users().DeletePasskey(ctx, ownerID(ctx), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
