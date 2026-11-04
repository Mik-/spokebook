// Package auth handles accounts, passwords, sessions and passkeys.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/catalog"
	"github.com/spokebook/spokebook/internal/app/spokebook/components"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// FirstUserID is the ID of the first account. Data entered before login
// existed belongs to it.
const FirstUserID = "00000000-0000-7000-8000-000000000001"

// MaxNameLength is the maximum length of a login name in characters.
const MaxNameLength = 64

// Lockout: after MaxFailedLogins failed logins in a row, every further
// failure locks the account for LockDuration.
const (
	MaxFailedLogins = 5
	LockDuration    = 15 * time.Minute
)

var (
	ErrNameRequired = errors.New("auth: name required")
	ErrNameTooLong  = errors.New("auth: name too long")
	ErrNameTaken    = errors.New("auth: name taken")
	// ErrInvalidLogin means unknown name or wrong password.
	ErrInvalidLogin = errors.New("auth: invalid name or password")
	// ErrLocked means too many failed logins; try again later.
	ErrLocked = errors.New("auth: account locked")
)

// CreateUser creates an account and its built-in component types and
// steps. The first account gets FirstUserID.
func CreateUser(ctx context.Context, st store.Store, name, password string) (store.User, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return store.User{}, ErrNameRequired
	case utf8.RuneCountInString(name) > MaxNameLength:
		return store.User{}, ErrNameTooLong
	}
	if err := ValidatePassword(password); err != nil {
		return store.User{}, err
	}
	users, err := st.Users().ListUsers(ctx)
	if err != nil {
		return store.User{}, err
	}
	u := store.User{
		ID: uuid.Must(uuid.NewV7()).String(), Name: name, PasswordHash: hashPassword(password),
		CreatedAt: time.Now().UTC(),
	}
	if len(users) == 0 {
		u.ID = FirstUserID
	}
	if err := st.Users().CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return store.User{}, ErrNameTaken
		}
		return store.User{}, err
	}
	return u, Seed(ctx, st, u.ID)
}

// Seed adds the built-in component types and steps the user doesn't have
// yet, e.g. after an update brought new ones.
func Seed(ctx context.Context, st store.Store, userID string) error {
	if err := components.SeedTypes(ctx, st, userID); err != nil {
		return err
	}
	return catalog.SeedSteps(ctx, st, userID)
}

// Login checks name and password. Failures count towards the lockout; a
// success resets it.
func Login(ctx context.Context, st store.Store, name, password string) (store.User, error) {
	now := time.Now().UTC()
	u, err := st.Users().GetUserByName(ctx, strings.TrimSpace(name))
	if errors.Is(err, store.ErrNotFound) {
		checkPassword(dummyHash, password)
		return store.User{}, ErrInvalidLogin
	}
	if err != nil {
		return store.User{}, err
	}
	if u.LockedUntil != nil && now.Before(*u.LockedUntil) {
		return store.User{}, ErrLocked
	}
	if !checkPassword(u.PasswordHash, password) {
		failed := u.FailedLogins + 1
		var until *time.Time
		if failed >= MaxFailedLogins {
			t := now.Add(LockDuration)
			until = &t
		}
		if err := st.Users().SetLoginFailures(ctx, u.ID, failed, until); err != nil {
			return store.User{}, err
		}
		if until != nil {
			return store.User{}, ErrLocked
		}
		return store.User{}, ErrInvalidLogin
	}
	return u, resetFailures(ctx, st, u)
}

func resetFailures(ctx context.Context, st store.Store, u store.User) error {
	if u.FailedLogins == 0 && u.LockedUntil == nil {
		return nil
	}
	return st.Users().SetLoginFailures(ctx, u.ID, 0, nil)
}

// ChangePassword sets a new password after checking the current one and
// ends all other sessions of the user.
func ChangePassword(ctx context.Context, st store.Store, userID, sessionID, current, password string) error {
	u, err := st.Users().GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if !checkPassword(u.PasswordHash, current) {
		return ErrInvalidLogin
	}
	if err := ValidatePassword(password); err != nil {
		return err
	}
	return st.WithTx(ctx, func(tx store.Stores) error {
		if err := tx.Users().SetPassword(ctx, userID, hashPassword(password)); err != nil {
			return err
		}
		return tx.Users().DeleteUserSessions(ctx, userID, sessionID)
	})
}
