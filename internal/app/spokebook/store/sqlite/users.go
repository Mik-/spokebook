package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite/db"
)

type userStore struct{ q *db.Queries }

func toUser(r db.User) (store.User, error) {
	u := store.User{ID: r.ID, Name: r.Name, PasswordHash: r.PasswordHash, FailedLogins: int(r.FailedLogins)}
	var err error
	if u.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
		return store.User{}, err
	}
	if u.LockedUntil, err = timePtr(r.LockedUntil); err != nil {
		return store.User{}, err
	}
	return u, nil
}

func (s userStore) CreateUser(ctx context.Context, u store.User) error {
	return mapErr(s.q.CreateUser(ctx, db.CreateUserParams{
		ID: u.ID, Name: u.Name, PasswordHash: u.PasswordHash, CreatedAt: formatTime(u.CreatedAt),
	}))
}

func (s userStore) GetUser(ctx context.Context, id string) (store.User, error) {
	r, err := s.q.GetUser(ctx, id)
	if err != nil {
		return store.User{}, mapErr(err)
	}
	return toUser(r)
}

func (s userStore) GetUserByName(ctx context.Context, name string) (store.User, error) {
	r, err := s.q.GetUserByName(ctx, name)
	if err != nil {
		return store.User{}, mapErr(err)
	}
	return toUser(r)
}

func (s userStore) ListUsers(ctx context.Context) ([]store.User, error) {
	rows, err := s.q.ListUsers(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.User, 0, len(rows))
	for _, r := range rows {
		u, err := toUser(r)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

func (s userStore) SetPassword(ctx context.Context, id, hash string) error {
	n, err := s.q.SetPassword(ctx, db.SetPasswordParams{PasswordHash: hash, ID: id})
	return rowsOrNotFound(n, err)
}

func (s userStore) SetLoginFailures(ctx context.Context, id string, failed int, lockedUntil *time.Time) error {
	n, err := s.q.SetLoginFailures(ctx, db.SetLoginFailuresParams{
		FailedLogins: int64(failed), LockedUntil: nullTime(lockedUntil), ID: id,
	})
	return rowsOrNotFound(n, err)
}

func toSession(r db.Session) (store.Session, error) {
	ss := store.Session{ID: r.ID, UserID: r.UserID}
	var err error
	if ss.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
		return store.Session{}, err
	}
	if ss.LastSeenAt, err = parseTime(r.LastSeenAt); err != nil {
		return store.Session{}, err
	}
	if ss.ExpiresAt, err = parseTime(r.ExpiresAt); err != nil {
		return store.Session{}, err
	}
	return ss, nil
}

func (s userStore) CreateSession(ctx context.Context, ss store.Session) error {
	return mapErr(s.q.CreateSession(ctx, db.CreateSessionParams{
		ID: ss.ID, UserID: ss.UserID, CreatedAt: formatTime(ss.CreatedAt),
		LastSeenAt: formatTime(ss.LastSeenAt), ExpiresAt: formatTime(ss.ExpiresAt),
	}))
}

func (s userStore) GetSession(ctx context.Context, id string) (store.Session, error) {
	r, err := s.q.GetSession(ctx, id)
	if err != nil {
		return store.Session{}, mapErr(err)
	}
	return toSession(r)
}

func (s userStore) TouchSession(ctx context.Context, id string, lastSeen, expires time.Time) error {
	n, err := s.q.TouchSession(ctx, db.TouchSessionParams{
		LastSeenAt: formatTime(lastSeen), ExpiresAt: formatTime(expires), ID: id,
	})
	return rowsOrNotFound(n, err)
}

func (s userStore) DeleteSession(ctx context.Context, id string) error {
	return mapErr(s.q.DeleteSession(ctx, id))
}

func (s userStore) DeleteUserSessions(ctx context.Context, userID, keepID string) error {
	return mapErr(s.q.DeleteUserSessions(ctx, db.DeleteUserSessionsParams{UserID: userID, ID: keepID}))
}

func (s userStore) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	return mapErr(s.q.DeleteExpiredSessions(ctx, formatTime(now)))
}

func toPasskey(r db.Passkey) (store.Passkey, error) {
	p := store.Passkey{
		ID: r.ID, UserID: r.UserID, CredentialID: r.CredentialID, Name: r.Name, Credential: r.Credential,
	}
	var err error
	if p.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
		return store.Passkey{}, err
	}
	if p.LastUsedAt, err = timePtr(r.LastUsedAt); err != nil {
		return store.Passkey{}, err
	}
	return p, nil
}

func (s userStore) CreatePasskey(ctx context.Context, p store.Passkey) error {
	return mapErr(s.q.CreatePasskey(ctx, db.CreatePasskeyParams{
		ID: p.ID, UserID: p.UserID, CredentialID: p.CredentialID, Name: p.Name,
		Credential: p.Credential, CreatedAt: formatTime(p.CreatedAt),
	}))
}

func (s userStore) GetPasskey(ctx context.Context, userID, id string) (store.Passkey, error) {
	r, err := s.q.GetPasskey(ctx, db.GetPasskeyParams{UserID: userID, ID: id})
	if err != nil {
		return store.Passkey{}, mapErr(err)
	}
	return toPasskey(r)
}

func (s userStore) GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (store.Passkey, error) {
	r, err := s.q.GetPasskeyByCredentialID(ctx, credentialID)
	if err != nil {
		return store.Passkey{}, mapErr(err)
	}
	return toPasskey(r)
}

func (s userStore) ListPasskeys(ctx context.Context, userID string) ([]store.Passkey, error) {
	rows, err := s.q.ListPasskeys(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.Passkey, 0, len(rows))
	for _, r := range rows {
		p, err := toPasskey(r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s userStore) UpdatePasskeyCredential(ctx context.Context, userID, id string, credential []byte, usedAt time.Time) error {
	n, err := s.q.UpdatePasskeyCredential(ctx, db.UpdatePasskeyCredentialParams{
		Credential: credential, LastUsedAt: sql.NullString{String: formatTime(usedAt), Valid: true},
		UserID: userID, ID: id,
	})
	return rowsOrNotFound(n, err)
}

func (s userStore) DeletePasskey(ctx context.Context, userID, id string) error {
	n, err := s.q.DeletePasskey(ctx, db.DeletePasskeyParams{UserID: userID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s userStore) DeleteOwnerData(ctx context.Context, ownerID string) error {
	for _, del := range []func(context.Context, string) error{
		s.q.DeleteOwnerRides, s.q.DeleteOwnerRideGears, s.q.DeleteOwnerBikes, s.q.DeleteOwnerComponents, s.q.DeleteOwnerSteps, s.q.DeleteOwnerComponentTypes,
		s.q.DeleteOwnerLevelIntervals, s.q.DeleteOwnerAttachments, s.q.DeleteOwnerShoppingItems,
	} {
		if err := del(ctx, ownerID); err != nil {
			return mapErr(err)
		}
	}
	return nil
}
