package auth_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/auth"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

const password = "correct horse battery"

func newStore(t *testing.T) store.Store {
	t.Helper()
	st, err := sqlite.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCreateUser(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	for _, tc := range []struct {
		name, password string
		want           error
	}{
		{" ", password, auth.ErrNameRequired},
		{string(make([]rune, auth.MaxNameLength+1)), password, auth.ErrNameTooLong},
		{"rider", "short", auth.ErrPasswordTooShort},
		{"rider", string(make([]byte, auth.MaxPasswordLength+1)), auth.ErrPasswordTooLong},
	} {
		if _, err := auth.CreateUser(ctx, st, tc.name, tc.password); !errors.Is(err, tc.want) {
			t.Errorf("%q/%q: want %v, got %v", tc.name, tc.password, tc.want, err)
		}
	}

	u, err := auth.CreateUser(ctx, st, " Rider ", password)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != auth.FirstUserID || u.Name != "Rider" {
		t.Errorf("first user: %+v", u)
	}
	steps, err := st.Steps().ListSteps(ctx, u.ID)
	if err != nil || len(steps) == 0 {
		t.Errorf("built-in steps not seeded: %d, %v", len(steps), err)
	}
	if _, err := auth.CreateUser(ctx, st, "rider", password); !errors.Is(err, auth.ErrNameTaken) {
		t.Errorf("taken name: %v", err)
	}
	second, err := auth.CreateUser(ctx, st, "second", password)
	if err != nil || second.ID == auth.FirstUserID {
		t.Errorf("second user: %+v, %v", second, err)
	}
}

func TestLoginAndLockout(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if _, err := auth.CreateUser(ctx, st, "rider", password); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Login(ctx, st, "nobody", password); !errors.Is(err, auth.ErrInvalidLogin) {
		t.Errorf("unknown name: %v", err)
	}
	if u, err := auth.Login(ctx, st, "RIDER", password); err != nil || u.ID != auth.FirstUserID {
		t.Fatalf("login: %+v, %v", u, err)
	}

	for i := 1; i < auth.MaxFailedLogins; i++ {
		if _, err := auth.Login(ctx, st, "rider", "wrong password"); !errors.Is(err, auth.ErrInvalidLogin) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	// A success in between resets the count.
	if _, err := auth.Login(ctx, st, "rider", password); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < auth.MaxFailedLogins; i++ {
		_, _ = auth.Login(ctx, st, "rider", "wrong password")
	}
	if _, err := auth.Login(ctx, st, "rider", "wrong password"); !errors.Is(err, auth.ErrLocked) {
		t.Fatalf("failure %d: want ErrLocked, got %v", auth.MaxFailedLogins, err)
	}
	if _, err := auth.Login(ctx, st, "rider", password); !errors.Is(err, auth.ErrLocked) {
		t.Fatalf("right password while locked: %v", err)
	}

	// After the lock, the right password works and resets the lock.
	past := time.Now().UTC().Add(-time.Second)
	if err := st.Users().SetLoginFailures(ctx, auth.FirstUserID, auth.MaxFailedLogins, &past); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Login(ctx, st, "rider", password); err != nil {
		t.Fatalf("after lock: %v", err)
	}
	if u, _ := st.Users().GetUser(ctx, auth.FirstUserID); u.FailedLogins != 0 || u.LockedUntil != nil {
		t.Errorf("not reset: %+v", u)
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	u, err := auth.CreateUser(ctx, st, "rider", password)
	if err != nil {
		t.Fatal(err)
	}
	token, s, err := auth.StartSession(ctx, st, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == token {
		t.Error("the token is stored as session ID")
	}
	got, _, extended, err := auth.Authenticate(ctx, st, token)
	if err != nil || got.ID != u.ID || extended {
		t.Fatalf("authenticate: %+v, %v, %v", got, extended, err)
	}
	for _, tok := range []string{"", "unknown"} {
		if _, _, _, err := auth.Authenticate(ctx, st, tok); !errors.Is(err, auth.ErrNoSession) {
			t.Errorf("token %q: %v", tok, err)
		}
	}

	// A session used after a while is extended.
	old := time.Now().UTC().Add(-2 * time.Hour)
	if err := st.Users().TouchSession(ctx, s.ID, old, old.Add(auth.SessionLifetime)); err != nil {
		t.Fatal(err)
	}
	_, s2, extended, err := auth.Authenticate(ctx, st, token)
	if err != nil || !extended || !s2.ExpiresAt.After(old.Add(auth.SessionLifetime)) {
		t.Fatalf("extend: %+v, %v, %v", s2, extended, err)
	}

	// An expired session is rejected.
	if err := st.Users().TouchSession(ctx, s.ID, old, old); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := auth.Authenticate(ctx, st, token); !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("expired: %v", err)
	}

	token, _, _ = auth.StartSession(ctx, st, u.ID)
	if err := auth.EndSession(ctx, st, token); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := auth.Authenticate(ctx, st, token); !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("after logout: %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	u, _ := auth.CreateUser(ctx, st, "rider", password)
	current, s, _ := auth.StartSession(ctx, st, u.ID)
	other, _, _ := auth.StartSession(ctx, st, u.ID)

	if err := auth.ChangePassword(ctx, st, u.ID, s.ID, "wrong password", "new password 123"); !errors.Is(err, auth.ErrInvalidLogin) {
		t.Errorf("wrong current: %v", err)
	}
	if err := auth.ChangePassword(ctx, st, u.ID, s.ID, password, "short"); !errors.Is(err, auth.ErrPasswordTooShort) {
		t.Errorf("short: %v", err)
	}
	if err := auth.ChangePassword(ctx, st, u.ID, s.ID, password, "new password 123"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Login(ctx, st, "rider", "new password 123"); err != nil {
		t.Errorf("new password: %v", err)
	}
	if _, _, _, err := auth.Authenticate(ctx, st, current); err != nil {
		t.Errorf("current session ended: %v", err)
	}
	if _, _, _, err := auth.Authenticate(ctx, st, other); !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("other session kept: %v", err)
	}
}

func TestSetup(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	setup, err := auth.NewSetup(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	token := setup.Token()
	if token == "" || !setup.Pending() {
		t.Fatal("no setup token without accounts")
	}
	if _, err := setup.Complete(ctx, st, "wrong", "rider", password); !errors.Is(err, auth.ErrSetupToken) {
		t.Errorf("wrong token: %v", err)
	}
	if _, err := setup.Complete(ctx, st, token, "rider", "short"); !errors.Is(err, auth.ErrPasswordTooShort) || !setup.Pending() {
		t.Errorf("short password: %v", err)
	}
	u, err := setup.Complete(ctx, st, token, "rider", password)
	if err != nil || u.ID != auth.FirstUserID {
		t.Fatalf("complete: %+v, %v", u, err)
	}
	if setup.Pending() {
		t.Error("still pending")
	}
	if _, err := setup.Complete(ctx, st, token, "again", password); !errors.Is(err, auth.ErrSetupDone) {
		t.Errorf("second setup: %v", err)
	}
	if setup, _ := auth.NewSetup(ctx, st); setup.Pending() {
		t.Error("pending after restart with an account")
	}
}

func TestPasskeys(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	u, _ := auth.CreateUser(ctx, st, "rider", password)
	pk, err := auth.NewPasskeys(st, "https://bikes.example.com")
	if err != nil {
		t.Fatal(err)
	}
	a := newAuthenticator(t, "https://bikes.example.com")

	if _, err := pk.BeginRegistration(ctx, u, " "); !errors.Is(err, auth.ErrNameRequired) {
		t.Errorf("empty name: %v", err)
	}
	opts, err := pk.BeginRegistration(ctx, u, " Phone ")
	if err != nil {
		t.Fatal(err)
	}
	answer := a.create(opts)
	k, err := pk.FinishRegistration(ctx, u, bytes.NewReader(answer))
	if err != nil {
		t.Fatal(err)
	}
	if k.Name != "Phone" || !bytes.Equal(k.CredentialID, a.credID) {
		t.Errorf("passkey: %+v", k)
	}
	// Each challenge works once.
	if _, err := pk.FinishRegistration(ctx, u, bytes.NewReader(answer)); !errors.Is(err, auth.ErrPasskey) {
		t.Errorf("replayed registration: %v", err)
	}

	opts, err = pk.BeginLogin()
	if err != nil {
		t.Fatal(err)
	}
	answer = a.get(opts)
	got, err := pk.FinishLogin(ctx, bytes.NewReader(answer))
	if err != nil || got.ID != u.ID {
		t.Fatalf("login: %+v, %v", got, err)
	}
	if _, err := pk.FinishLogin(ctx, bytes.NewReader(answer)); !errors.Is(err, auth.ErrPasskey) {
		t.Errorf("replayed login: %v", err)
	}
	keys, _ := st.Users().ListPasskeys(ctx, u.ID)
	if len(keys) != 1 || keys[0].LastUsedAt == nil {
		t.Errorf("after login: %+v", keys)
	}

	// An authenticator from another origin is rejected.
	opts, _ = pk.BeginLogin()
	a.origin = "https://evil.example.com"
	if _, err := pk.FinishLogin(ctx, bytes.NewReader(a.get(opts))); !errors.Is(err, auth.ErrPasskey) {
		t.Errorf("other origin: %v", err)
	}
	// A deleted passkey no longer works.
	a.origin = "https://bikes.example.com"
	if err := st.Users().DeletePasskey(ctx, u.ID, k.ID); err != nil {
		t.Fatal(err)
	}
	opts, _ = pk.BeginLogin()
	if _, err := pk.FinishLogin(ctx, bytes.NewReader(a.get(opts))); !errors.Is(err, auth.ErrPasskey) {
		t.Errorf("deleted passkey: %v", err)
	}
	if _, err := pk.FinishLogin(ctx, bytes.NewReader([]byte("{}"))); !errors.Is(err, auth.ErrPasskey) {
		t.Errorf("garbage: %v", err)
	}
}
