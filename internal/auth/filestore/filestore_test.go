package filestore_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackiabishop/mileminder/internal/auth"
	"github.com/jackiabishop/mileminder/internal/auth/authtest"
	"github.com/jackiabishop/mileminder/internal/auth/filestore"
)

func TestFileUserStoreConformance(t *testing.T) {
	authtest.RunUserStore(t, func(t *testing.T) auth.UserStore {
		return filestore.NewUserStore(t.TempDir())
	})
}

func TestFileSessionStoreConformance(t *testing.T) {
	authtest.RunSessionStore(t, func(t *testing.T) auth.SessionStore {
		return filestore.NewSessionStore(t.TempDir())
	})
}

// A user written by one store handle must be visible to a fresh handle over the
// same directory — i.e. the data actually persists to disk.
func TestFileUserStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	created, err := filestore.NewUserStore(dir).CreateUser(ctx, "alice@example.com", "hash1")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	reopened := filestore.NewUserStore(dir)
	got, err := reopened.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID after reopen: %v", err)
	}
	if got.Email != "alice@example.com" || got.PasswordHash != "hash1" {
		t.Fatalf("reopened user mismatch: %+v", got)
	}
}

func TestFileSessionStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	if err := filestore.NewSessionStore(dir).CreateSession(ctx, "hash-a", "user-1", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := filestore.NewSessionStore(dir).GetSession(ctx, "hash-a")
	if err != nil {
		t.Fatalf("GetSession after reopen: %v", err)
	}
	if got.UserID != "user-1" {
		t.Fatalf("reopened session mismatch: %+v", got)
	}
}
