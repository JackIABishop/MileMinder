package storage

import "sync"

// Tenants hands out per-user scoped Stores. It is Phase 2's ownership seam: in
// hosted mode the authenticated middleware resolves a user id from the session
// and calls ForUser to obtain a Store isolated to that user's data. The
// single-tenant self-hosted path never touches a Tenants — it holds one
// process-wide Store directly, so no owner parameter leaks into handlers.
//
// ForUser returns a Store, not (Store, error): a valid id always yields a usable
// store, and implementations that must reject a malformed id return a Store whose
// every method fails rather than widening this signature. Callers depend only on
// the Store contract, so an implementation swap (Phase 3's SQL backend) is
// confined here.
type Tenants interface {
	// ForUser returns a Store scoped to userID. Data written through one user's
	// Store — vehicles, readings, and the current pointer — must be invisible
	// through any other user's Store.
	ForUser(userID string) Store
}

// MemoryTenants is an in-memory Tenants for multi-tenant tests. Each userID gets
// its own lazily-created Memory store, so writes under one user cannot be
// observed through another user's Store. It mirrors what yamlstore.Tenants does
// with per-user directories, without touching the filesystem.
type MemoryTenants struct {
	mu    sync.Mutex
	users map[string]*Memory
}

// NewMemoryTenants returns an empty in-memory Tenants.
func NewMemoryTenants() *MemoryTenants {
	return &MemoryTenants{users: map[string]*Memory{}}
}

// ForUser returns the per-user Memory store, creating it on first use.
func (t *MemoryTenants) ForUser(userID string) Store {
	t.mu.Lock()
	defer t.mu.Unlock()

	st, ok := t.users[userID]
	if !ok {
		st = NewMemory()
		t.users[userID] = st
	}
	return st
}

// Compile-time assertion that MemoryTenants satisfies Tenants.
var _ Tenants = (*MemoryTenants)(nil)
