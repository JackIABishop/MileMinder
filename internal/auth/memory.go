package auth

import (
	"context"
	"sync"
	"time"
)

// MemoryUserStore is an in-memory UserStore for tests. It mirrors the file and
// (future) SQL stores' observable behaviour: normalised-email uniqueness,
// ErrEmailTaken, ErrNotFound.
type MemoryUserStore struct {
	mu      sync.Mutex
	byID    map[string]*User
	byEmail map[string]*User
}

// NewMemoryUserStore returns an empty in-memory UserStore.
func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{byID: map[string]*User{}, byEmail: map[string]*User{}}
}

func cloneUser(u *User) *User {
	cp := *u
	return &cp
}

func (m *MemoryUserStore) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	email = NormalizeEmail(email)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.byEmail[email]; ok {
		return nil, ErrEmailTaken
	}
	id, err := NewUserID()
	if err != nil {
		return nil, err
	}
	u := &User{ID: id, Email: email, PasswordHash: passwordHash, CreatedAt: time.Now().UTC()}
	m.byID[id] = u
	m.byEmail[email] = u
	return cloneUser(u), nil
}

func (m *MemoryUserStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, ok := m.byEmail[NormalizeEmail(email)]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneUser(u), nil
}

func (m *MemoryUserStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, ok := m.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneUser(u), nil
}

// MemorySessionStore is an in-memory SessionStore for tests.
type MemorySessionStore struct {
	mu     sync.Mutex
	byHash map[string]*Session
}

// NewMemorySessionStore returns an empty in-memory SessionStore.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{byHash: map[string]*Session{}}
}

func cloneSession(s *Session) *Session {
	cp := *s
	return &cp
}

func (m *MemorySessionStore) CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.byHash[tokenHash] = &Session{TokenHash: tokenHash, UserID: userID, ExpiresAt: expires}
	return nil
}

func (m *MemorySessionStore) GetSession(ctx context.Context, tokenHash string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.byHash[tokenHash]
	if !ok {
		return nil, ErrNotFound
	}
	if time.Now().After(s.ExpiresAt) {
		delete(m.byHash, tokenHash) // opportunistic cleanup of an expired session
		return nil, ErrNotFound
	}
	return cloneSession(s), nil
}

func (m *MemorySessionStore) DeleteSession(ctx context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.byHash, tokenHash)
	return nil
}

func (m *MemorySessionStore) TouchSession(ctx context.Context, tokenHash string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.byHash[tokenHash]
	if !ok {
		return ErrNotFound
	}
	s.ExpiresAt = expires
	return nil
}

// Compile-time assertions.
var (
	_ UserStore    = (*MemoryUserStore)(nil)
	_ SessionStore = (*MemorySessionStore)(nil)
)
