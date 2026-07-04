// Package authtest provides shared conformance suites so every auth.UserStore
// and auth.SessionStore implementation is held to the same observable behaviour.
package authtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackiabishop/mileminder/internal/auth"
)

// RunUserStore exercises the auth.UserStore contract against a fresh store.
func RunUserStore(t *testing.T, newStore func(t *testing.T) auth.UserStore) {
	t.Helper()
	ctx := context.Background()

	t.Run("GetMissing", func(t *testing.T) {
		st := newStore(t)
		if _, err := st.GetUserByEmail(ctx, "nobody@example.com"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("GetUserByEmail missing: want ErrNotFound, got %v", err)
		}
		if _, err := st.GetUserByID(ctx, "nope"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("GetUserByID missing: want ErrNotFound, got %v", err)
		}
	})

	t.Run("CreateThenGet", func(t *testing.T) {
		st := newStore(t)
		u, err := st.CreateUser(ctx, "alice@example.com", "hash1")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if u.ID == "" {
			t.Fatal("CreateUser returned empty ID")
		}
		byEmail, err := st.GetUserByEmail(ctx, "alice@example.com")
		if err != nil {
			t.Fatalf("GetUserByEmail: %v", err)
		}
		byID, err := st.GetUserByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetUserByID: %v", err)
		}
		if byEmail.ID != u.ID || byID.Email != "alice@example.com" || byID.PasswordHash != "hash1" {
			t.Fatalf("round trip mismatch: byEmail=%+v byID=%+v", byEmail, byID)
		}
	})

	t.Run("DuplicateEmailRejected", func(t *testing.T) {
		st := newStore(t)
		if _, err := st.CreateUser(ctx, "bob@example.com", "h"); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		// Same email, different case — must still collide.
		if _, err := st.CreateUser(ctx, "BOB@Example.com", "h2"); !errors.Is(err, auth.ErrEmailTaken) {
			t.Fatalf("duplicate email: want ErrEmailTaken, got %v", err)
		}
	})

	t.Run("EmailIsCaseInsensitive", func(t *testing.T) {
		st := newStore(t)
		if _, err := st.CreateUser(ctx, "Carol@Example.com", "h"); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if _, err := st.GetUserByEmail(ctx, "carol@example.com"); err != nil {
			t.Fatalf("GetUserByEmail normalised: %v", err)
		}
	})

	t.Run("ListUsers", func(t *testing.T) {
		st := newStore(t)
		empty, err := st.ListUsers(ctx)
		if err != nil {
			t.Fatalf("ListUsers empty: %v", err)
		}
		if len(empty) != 0 {
			t.Fatalf("ListUsers empty returned %d users", len(empty))
		}

		alice, err := st.CreateUser(ctx, "alice@example.com", "hash1")
		if err != nil {
			t.Fatalf("CreateUser alice: %v", err)
		}
		bob, err := st.CreateUser(ctx, "bob@example.com", "hash2")
		if err != nil {
			t.Fatalf("CreateUser bob: %v", err)
		}
		users, err := st.ListUsers(ctx)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("ListUsers count = %d, want 2", len(users))
		}
		got := map[string]string{}
		for _, u := range users {
			got[u.ID] = u.Email
		}
		if got[alice.ID] != "alice@example.com" || got[bob.ID] != "bob@example.com" {
			t.Fatalf("ListUsers mismatch: %+v", users)
		}
	})

	t.Run("UpdatePassword", func(t *testing.T) {
		st := newStore(t)
		u, err := st.CreateUser(ctx, "alice@example.com", "hash1")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if err := st.UpdatePassword(ctx, u.ID, "hash2"); err != nil {
			t.Fatalf("UpdatePassword: %v", err)
		}
		got, err := st.GetUserByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("GetUserByID: %v", err)
		}
		if got.PasswordHash != "hash2" {
			t.Fatalf("password hash = %q, want hash2", got.PasswordHash)
		}
		if err := st.UpdatePassword(ctx, "missing", "hash3"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("UpdatePassword missing: want ErrNotFound, got %v", err)
		}
	})
}

// RunSessionStore exercises the auth.SessionStore contract against a fresh store.
func RunSessionStore(t *testing.T, newStore func(t *testing.T) auth.SessionStore) {
	t.Helper()
	ctx := context.Background()
	future := time.Now().Add(time.Hour)

	t.Run("GetMissing", func(t *testing.T) {
		st := newStore(t)
		if _, err := st.GetSession(ctx, "nope"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("GetSession missing: want ErrNotFound, got %v", err)
		}
	})

	t.Run("CreateThenGet", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateSession(ctx, "hash-a", "user-1", future); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		got, err := st.GetSession(ctx, "hash-a")
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got.UserID != "user-1" {
			t.Fatalf("session user mismatch: %+v", got)
		}
	})

	t.Run("ExpiredSessionIsNotFound", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateSession(ctx, "hash-exp", "user-1", time.Now().Add(-time.Minute)); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if _, err := st.GetSession(ctx, "hash-exp"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("expired session: want ErrNotFound, got %v", err)
		}
	})

	t.Run("TouchExtendsExpiry", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateSession(ctx, "hash-t", "user-1", time.Now().Add(time.Second)); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := st.TouchSession(ctx, "hash-t", future); err != nil {
			t.Fatalf("TouchSession: %v", err)
		}
		if _, err := st.GetSession(ctx, "hash-t"); err != nil {
			t.Fatalf("after touch: %v", err)
		}
		if err := st.TouchSession(ctx, "missing", future); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("TouchSession missing: want ErrNotFound, got %v", err)
		}
	})

	t.Run("DeleteRevokes", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateSession(ctx, "hash-d", "user-1", future); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := st.DeleteSession(ctx, "hash-d"); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}
		if _, err := st.GetSession(ctx, "hash-d"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("after delete: want ErrNotFound, got %v", err)
		}
		// Deleting an already-absent session is a no-op, not an error.
		if err := st.DeleteSession(ctx, "hash-d"); err != nil {
			t.Fatalf("idempotent delete: %v", err)
		}
	})

	t.Run("DeleteUserSessions", func(t *testing.T) {
		st := newStore(t)
		for _, sess := range []struct {
			hash string
			user string
		}{
			{"hash-keep", "user-1"},
			{"hash-delete", "user-1"},
			{"hash-other", "user-2"},
		} {
			if err := st.CreateSession(ctx, sess.hash, sess.user, future); err != nil {
				t.Fatalf("CreateSession %s: %v", sess.hash, err)
			}
		}
		if err := st.DeleteUserSessions(ctx, "user-1", "hash-keep"); err != nil {
			t.Fatalf("DeleteUserSessions except: %v", err)
		}
		if _, err := st.GetSession(ctx, "hash-keep"); err != nil {
			t.Fatalf("excepted session missing: %v", err)
		}
		if _, err := st.GetSession(ctx, "hash-delete"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("deleted user session: want ErrNotFound, got %v", err)
		}
		if _, err := st.GetSession(ctx, "hash-other"); err != nil {
			t.Fatalf("other user's session missing: %v", err)
		}
		if err := st.DeleteUserSessions(ctx, "user-1", ""); err != nil {
			t.Fatalf("DeleteUserSessions all: %v", err)
		}
		if _, err := st.GetSession(ctx, "hash-keep"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("delete all user sessions: want ErrNotFound, got %v", err)
		}
		if _, err := st.GetSession(ctx, "hash-other"); err != nil {
			t.Fatalf("other user's session after delete all missing: %v", err)
		}
	})
}

// RunPasswordResetStore exercises the auth.PasswordResetStore contract against a
// fresh store.
func RunPasswordResetStore(t *testing.T, newStore func(t *testing.T) auth.PasswordResetStore) {
	t.Helper()
	ctx := context.Background()
	future := time.Now().Add(time.Hour)

	t.Run("CreateConsumeRoundTrip", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateReset(ctx, "hash-a", "user-1", future); err != nil {
			t.Fatalf("CreateReset: %v", err)
		}
		got, err := st.ConsumeReset(ctx, "hash-a")
		if err != nil {
			t.Fatalf("ConsumeReset: %v", err)
		}
		if got.UserID != "user-1" {
			t.Fatalf("reset user mismatch: %+v", got)
		}
	})

	t.Run("SingleUse", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateReset(ctx, "hash-a", "user-1", future); err != nil {
			t.Fatalf("CreateReset: %v", err)
		}
		if _, err := st.ConsumeReset(ctx, "hash-a"); err != nil {
			t.Fatalf("first ConsumeReset: %v", err)
		}
		if _, err := st.ConsumeReset(ctx, "hash-a"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("second ConsumeReset: want ErrNotFound, got %v", err)
		}
	})

	t.Run("ExpiredIsNotFound", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateReset(ctx, "hash-exp", "user-1", time.Now().Add(-time.Minute)); err != nil {
			t.Fatalf("CreateReset: %v", err)
		}
		if _, err := st.ConsumeReset(ctx, "hash-exp"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("expired ConsumeReset: want ErrNotFound, got %v", err)
		}
	})

	t.Run("CreateReplacesPreviousForUser", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateReset(ctx, "hash-old", "user-1", future); err != nil {
			t.Fatalf("CreateReset old: %v", err)
		}
		if err := st.CreateReset(ctx, "hash-new", "user-1", future); err != nil {
			t.Fatalf("CreateReset new: %v", err)
		}
		if _, err := st.ConsumeReset(ctx, "hash-old"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("old reset: want ErrNotFound, got %v", err)
		}
		if _, err := st.ConsumeReset(ctx, "hash-new"); err != nil {
			t.Fatalf("new reset: %v", err)
		}
	})

	t.Run("DeleteResetsForUser", func(t *testing.T) {
		st := newStore(t)
		if err := st.CreateReset(ctx, "hash-a", "user-1", future); err != nil {
			t.Fatalf("CreateReset user-1: %v", err)
		}
		if err := st.CreateReset(ctx, "hash-b", "user-2", future); err != nil {
			t.Fatalf("CreateReset user-2: %v", err)
		}
		if err := st.DeleteResetsForUser(ctx, "user-1"); err != nil {
			t.Fatalf("DeleteResetsForUser: %v", err)
		}
		if _, err := st.ConsumeReset(ctx, "hash-a"); !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("deleted user's reset: want ErrNotFound, got %v", err)
		}
		if _, err := st.ConsumeReset(ctx, "hash-b"); err != nil {
			t.Fatalf("other user's reset missing: %v", err)
		}
	})
}
