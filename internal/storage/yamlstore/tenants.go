package yamlstore

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/storage"
)

// Tenants is a storage.Tenants backed by the historical YAML layout, one
// directory per user under <root>/users/<userID>. Because a per-user directory
// has exactly the layout of a single-user ~/.mileminder (vehicle <id>.yml files
// plus a "current" pointer), each scoped Store is a plain *Store rooted there and
// the single-tenant implementation is reused unchanged — the current pointer is
// isolated per user for free.
type Tenants struct {
	root string
}

// NewTenants returns a Tenants rooted at root. User directories are created
// lazily on first write by the underlying Store.
func NewTenants(root string) *Tenants {
	return &Tenants{root: root}
}

// ForUser returns a Store scoped to userID's directory. User ids are
// server-generated (crypto-random hex) and therefore path-safe, but ForUser
// validates the shape anyway as defence against directory traversal: a malformed
// id yields a Store whose every method fails rather than one that could escape
// the root.
func (t *Tenants) ForUser(userID string) storage.Store {
	if !validUserID(userID) {
		return errStore{id: userID}
	}
	return New(filepath.Join(t.root, "users", userID))
}

// validUserID accepts only non-empty ids of bounded length containing solely
// [A-Za-z0-9_-]. This excludes path separators, "." and "..", so a validated id
// cannot escape the users directory when joined.
func validUserID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

// errStore is a storage.Store that fails every operation with the same error. It
// is returned for a malformed user id so a traversal-shaped id can never resolve
// to a real directory.
type errStore struct {
	id string
}

func (e errStore) err() error {
	return fmt.Errorf("invalid user id %q", e.id)
}

func (e errStore) ListVehicles(context.Context) ([]storage.Record, error) {
	return nil, e.err()
}
func (e errStore) GetVehicle(context.Context, string) (*model.VehicleData, error) {
	return nil, e.err()
}
func (e errStore) SaveVehicle(context.Context, string, *model.VehicleData) error { return e.err() }
func (e errStore) DeleteVehicle(context.Context, string) error                   { return e.err() }
func (e errStore) PutReading(context.Context, string, string, int) error         { return e.err() }
func (e errStore) DeleteReading(context.Context, string, string) error           { return e.err() }
func (e errStore) GetCurrent(context.Context) (string, error)                    { return "", e.err() }
func (e errStore) SetCurrent(context.Context, string) error                      { return e.err() }

// Compile-time assertions.
var (
	_ storage.Tenants = (*Tenants)(nil)
	_ storage.Store   = errStore{}
)
