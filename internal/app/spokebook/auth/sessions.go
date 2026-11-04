package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// SessionLifetime: a session ends when it isn't used for this long.
const SessionLifetime = 30 * 24 * time.Hour

// touchInterval limits how often a used session is extended.
const touchInterval = time.Hour

// ErrNoSession means the token belongs to no valid session.
var ErrNoSession = errors.New("auth: no session")

// sessionID derives the stored ID from the token, so the database never
// holds a token.
func sessionID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// StartSession creates a session for the user and returns the token for
// the cookie. Expired sessions are cleaned up on the way.
func StartSession(ctx context.Context, st store.Store, userID string) (string, store.Session, error) {
	now := time.Now().UTC()
	if err := st.Users().DeleteExpiredSessions(ctx, now); err != nil {
		return "", store.Session{}, err
	}
	token := rand.Text()
	s := store.Session{
		ID: sessionID(token), UserID: userID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(SessionLifetime),
	}
	return token, s, st.Users().CreateSession(ctx, s)
}

// Authenticate returns the user and session of a token and extends the
// session. extended reports whether the cookie should be renewed.
func Authenticate(ctx context.Context, st store.Store, token string) (u store.User, s store.Session, extended bool, err error) {
	if token == "" {
		return store.User{}, store.Session{}, false, ErrNoSession
	}
	now := time.Now().UTC()
	s, err = st.Users().GetSession(ctx, sessionID(token))
	if errors.Is(err, store.ErrNotFound) || err == nil && !now.Before(s.ExpiresAt) {
		return store.User{}, store.Session{}, false, ErrNoSession
	}
	if err != nil {
		return store.User{}, store.Session{}, false, err
	}
	u, err = st.Users().GetUser(ctx, s.UserID)
	if err != nil {
		return store.User{}, store.Session{}, false, err
	}
	if now.Sub(s.LastSeenAt) >= touchInterval {
		s.LastSeenAt, s.ExpiresAt = now, now.Add(SessionLifetime)
		if err := st.Users().TouchSession(ctx, s.ID, s.LastSeenAt, s.ExpiresAt); err != nil {
			return store.User{}, store.Session{}, false, err
		}
		extended = true
	}
	return u, s, extended, nil
}

// EndSession deletes the session of the token, if any.
func EndSession(ctx context.Context, st store.Store, token string) error {
	return st.Users().DeleteSession(ctx, sessionID(token))
}
