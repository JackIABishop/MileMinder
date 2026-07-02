package api

import (
	"context"

	"github.com/jackiabishop/mileminder/internal/storage"
)

// ctxKey is the unexported type for this package's context keys, so no other
// package can collide with them.
type ctxKey int

const (
	storeKey ctxKey = iota
	userIDKey
	tokenHashKey
)

// withStore returns a copy of ctx carrying the per-request storage.Store. The
// mode middleware sets it: single-user mode injects the process-wide store,
// hosted mode injects the authenticated user's scoped store.
func withStore(ctx context.Context, st storage.Store) context.Context {
	return context.WithValue(ctx, storeKey, st)
}

// storeFrom returns the storage.Store the mode middleware placed on the request.
// A handler reaching this without a store means it was wired without the store
// middleware — a programming error, so it panics rather than silently misbehave.
func storeFrom(ctx context.Context) storage.Store {
	st, ok := ctx.Value(storeKey).(storage.Store)
	if !ok {
		panic("api: no storage.Store in request context (missing store middleware)")
	}
	return st
}

// withSession carries the authenticated user's id and the session token hash.
// Set by requireSession in hosted mode; unset in single-user mode.
func withSession(ctx context.Context, userID, tokenHash string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	return context.WithValue(ctx, tokenHashKey, tokenHash)
}

// userIDFrom returns the authenticated user id, or "" in single-user mode.
func userIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(userIDKey).(string)
	return id
}

// tokenHashFrom returns the current session's token hash (for logout).
func tokenHashFrom(ctx context.Context) string {
	h, _ := ctx.Value(tokenHashKey).(string)
	return h
}
