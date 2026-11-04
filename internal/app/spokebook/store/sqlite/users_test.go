package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

func TestUsersSessionsPasskeys(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	us := s.Users()
	now := time.Now().UTC().Truncate(time.Second)

	if err := us.CreateUser(ctx, store.User{ID: "u1", Name: "Rider", PasswordHash: "h", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := us.CreateUser(ctx, store.User{ID: "u2", Name: "rider", PasswordHash: "h", CreatedAt: now}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("same name in other case: want ErrConflict, got %v", err)
	}
	u, err := us.GetUserByName(ctx, "RIDER")
	if err != nil || u.ID != "u1" || u.Name != "Rider" || !u.CreatedAt.Equal(now) {
		t.Fatalf("by name: %+v, %v", u, err)
	}
	until := now.Add(time.Minute)
	if err := us.SetLoginFailures(ctx, "u1", 5, &until); err != nil {
		t.Fatal(err)
	}
	if err := us.SetPassword(ctx, "u1", "h2"); err != nil {
		t.Fatal(err)
	}
	u, _ = us.GetUser(ctx, "u1")
	if u.FailedLogins != 5 || u.LockedUntil == nil || !u.LockedUntil.Equal(until) || u.PasswordHash != "h2" {
		t.Fatalf("after updates: %+v", u)
	}
	if err := us.SetPassword(ctx, "nobody", "h"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown user: want ErrNotFound, got %v", err)
	}

	for _, id := range []string{"s1", "s2", "old"} {
		exp := now.Add(time.Hour)
		if id == "old" {
			exp = now.Add(-time.Second)
		}
		if err := us.CreateSession(ctx, store.Session{ID: id, UserID: "u1", CreatedAt: now, LastSeenAt: now, ExpiresAt: exp}); err != nil {
			t.Fatal(err)
		}
	}
	if err := us.TouchSession(ctx, "s1", now.Add(time.Minute), now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ss, err := us.GetSession(ctx, "s1"); err != nil || !ss.ExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("touched session: %+v, %v", ss, err)
	}
	if err := us.DeleteExpiredSessions(ctx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := us.GetSession(ctx, "old"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expired session: want ErrNotFound, got %v", err)
	}
	if err := us.DeleteUserSessions(ctx, "u1", "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := us.GetSession(ctx, "s2"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other session: want ErrNotFound, got %v", err)
	}
	if _, err := us.GetSession(ctx, "s1"); err != nil {
		t.Fatalf("kept session: %v", err)
	}

	k := store.Passkey{ID: "k1", UserID: "u1", CredentialID: []byte{1, 2, 3}, Name: "Phone", Credential: []byte(`{}`), CreatedAt: now}
	if err := us.CreatePasskey(ctx, k); err != nil {
		t.Fatal(err)
	}
	k.ID = "k2"
	if err := us.CreatePasskey(ctx, k); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("same credential ID: want ErrConflict, got %v", err)
	}
	if err := us.UpdatePasskeyCredential(ctx, "u1", "k1", []byte(`{"a":1}`), now); err != nil {
		t.Fatal(err)
	}
	got, err := us.GetPasskeyByCredentialID(ctx, []byte{1, 2, 3})
	if err != nil || got.ID != "k1" || string(got.Credential) != `{"a":1}` || got.LastUsedAt == nil {
		t.Fatalf("by credential ID: %+v, %v", got, err)
	}
	if _, err := us.GetPasskey(ctx, "u2", "k1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other user: want ErrNotFound, got %v", err)
	}
	if err := us.DeletePasskey(ctx, "u2", "k1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete as other user: want ErrNotFound, got %v", err)
	}
	if err := us.DeletePasskey(ctx, "u1", "k1"); err != nil {
		t.Fatal(err)
	}
	if keys, _ := us.ListPasskeys(ctx, "u1"); len(keys) != 0 {
		t.Fatalf("after delete: %d passkeys", len(keys))
	}
}
