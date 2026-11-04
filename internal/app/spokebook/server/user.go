package server

import (
	"context"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

type ctxKey int

const (
	userKey ctxKey = iota
	sessionKey
)

func withUser(ctx context.Context, u store.User, s store.Session) context.Context {
	return context.WithValue(context.WithValue(ctx, userKey, u), sessionKey, s)
}

// currentUser returns the logged-in user; ok is false on public pages
// without login.
func currentUser(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(userKey).(store.User)
	return u, ok
}

func currentSession(ctx context.Context) store.Session {
	s, _ := ctx.Value(sessionKey).(store.Session)
	return s
}

// ownerID returns the ID of the logged-in user, who owns all data the
// request touches.
func ownerID(ctx context.Context) string {
	u, ok := currentUser(ctx)
	if !ok {
		panic("server: no user in context")
	}
	return u.ID
}
