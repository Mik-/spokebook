package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"sync"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

var (
	// ErrSetupDone means an account exists already.
	ErrSetupDone = errors.New("auth: setup done")
	// ErrSetupToken means the setup token is wrong.
	ErrSetupToken = errors.New("auth: invalid setup token")
)

// Setup creates the first account. Until then, anyone who knows the token
// (written to the log at startup) may do so.
type Setup struct {
	mu    sync.Mutex
	token string // empty once an account exists
}

// NewSetup returns a Setup with a fresh token if there is no account yet.
func NewSetup(ctx context.Context, st store.Store) (*Setup, error) {
	users, err := st.Users().ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	s := &Setup{}
	if len(users) == 0 {
		s.token = rand.Text()
	}
	return s, nil
}

// Token returns the setup token, or "" if setup is done.
func (s *Setup) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Pending reports whether no account exists yet.
func (s *Setup) Pending() bool { return s.Token() != "" }

// Complete creates the first account.
func (s *Setup) Complete(ctx context.Context, st store.Store, token, name, password string) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == "" {
		return store.User{}, ErrSetupDone
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
		return store.User{}, ErrSetupToken
	}
	u, err := CreateUser(ctx, st, name, password)
	if err != nil {
		return store.User{}, err
	}
	s.token = ""
	return u, nil
}
