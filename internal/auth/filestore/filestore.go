// Package filestore is the interim, file-backed implementation of the auth
// stores used in hosted mode before Phase 3's Postgres. Users live in
// <root>/users.yml and sessions in <root>/sessions.yml, each a mutex-guarded
// whole-file document written atomically. The data is tiny (accounts + active
// sessions), so load-all / save-all per operation is more than adequate at the
// "me + a few testers" scale this phase targets; Phase 3 replaces it wholesale
// behind the same auth.UserStore / auth.SessionStore interfaces.
package filestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/jackiabishop/mileminder/internal/atomicfile"
	"github.com/jackiabishop/mileminder/internal/auth"
	"gopkg.in/yaml.v3"
)

// UserStore is a file-backed auth.UserStore.
type UserStore struct {
	path string
	mu   sync.Mutex
}

// NewUserStore returns a UserStore backed by <root>/users.yml. The directory is
// created lazily on first write.
func NewUserStore(root string) *UserStore {
	return &UserStore{path: filepath.Join(root, "users.yml")}
}

type usersDoc struct {
	Users []*auth.User `yaml:"users"`
}

// load reads the users file. A missing file is an empty set. Callers hold mu.
func (s *UserStore) load() ([]*auth.User, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read users file: %w", err)
	}
	var doc usersDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse users file: %w", err)
	}
	return doc.Users, nil
}

// save atomically writes the users file. Callers hold mu.
func (s *UserStore) save(users []*auth.User) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return atomicfile.Write(s.path, 0600, func(f *os.File) error {
		enc := yaml.NewEncoder(f)
		if err := enc.Encode(usersDoc{Users: users}); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	})
}

func (s *UserStore) CreateUser(ctx context.Context, email, passwordHash string) (*auth.User, error) {
	email = auth.NormalizeEmail(email)

	s.mu.Lock()
	defer s.mu.Unlock()

	users, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.Email == email {
			return nil, auth.ErrEmailTaken
		}
	}
	id, err := auth.NewUserID()
	if err != nil {
		return nil, err
	}
	u := &auth.User{ID: id, Email: email, PasswordHash: passwordHash, CreatedAt: time.Now().UTC()}
	if err := s.save(append(users, u)); err != nil {
		return nil, err
	}
	cp := *u
	return &cp, nil
}

func (s *UserStore) GetUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	email = auth.NormalizeEmail(email)

	s.mu.Lock()
	defer s.mu.Unlock()

	users, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.Email == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, auth.ErrNotFound
}

func (s *UserStore) GetUserByID(ctx context.Context, id string) (*auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	users, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, auth.ErrNotFound
}

func (s *UserStore) ListUsers(ctx context.Context) ([]*auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	users, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]*auth.User, 0, len(users))
	for _, u := range users {
		cp := *u
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *UserStore) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	users, err := s.load()
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.ID == userID {
			u.PasswordHash = passwordHash
			return s.save(users)
		}
	}
	return auth.ErrNotFound
}

// SessionStore is a file-backed auth.SessionStore.
type SessionStore struct {
	path string
	mu   sync.Mutex
}

// NewSessionStore returns a SessionStore backed by <root>/sessions.yml.
func NewSessionStore(root string) *SessionStore {
	return &SessionStore{path: filepath.Join(root, "sessions.yml")}
}

type sessionsDoc struct {
	Sessions []*auth.Session `yaml:"sessions"`
}

func (s *SessionStore) load() ([]*auth.Session, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions file: %w", err)
	}
	var doc sessionsDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse sessions file: %w", err)
	}
	return doc.Sessions, nil
}

func (s *SessionStore) save(sessions []*auth.Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return atomicfile.Write(s.path, 0600, func(f *os.File) error {
		enc := yaml.NewEncoder(f)
		if err := enc.Encode(sessionsDoc{Sessions: sessions}); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	})
}

// prune drops expired sessions so the file cannot grow without bound. It runs on
// write, which is the only place the set changes.
func prune(sessions []*auth.Session, now time.Time) []*auth.Session {
	kept := sessions[:0]
	for _, sess := range sessions {
		if now.Before(sess.ExpiresAt) {
			kept = append(kept, sess)
		}
	}
	return kept
}

func (s *SessionStore) CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.load()
	if err != nil {
		return err
	}
	sessions = prune(sessions, time.Now())
	sessions = append(sessions, &auth.Session{TokenHash: tokenHash, UserID: userID, ExpiresAt: expires})
	return s.save(sessions)
}

func (s *SessionStore) GetSession(ctx context.Context, tokenHash string) (*auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, sess := range sessions {
		if sess.TokenHash == tokenHash {
			if time.Now().After(sess.ExpiresAt) {
				return nil, auth.ErrNotFound
			}
			cp := *sess
			return &cp, nil
		}
	}
	return nil, auth.ErrNotFound
}

func (s *SessionStore) DeleteSession(ctx context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.load()
	if err != nil {
		return err
	}
	kept := make([]*auth.Session, 0, len(sessions))
	for _, sess := range sessions {
		if sess.TokenHash != tokenHash {
			kept = append(kept, sess)
		}
	}
	return s.save(kept)
}

func (s *SessionStore) DeleteUserSessions(ctx context.Context, userID, exceptTokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.load()
	if err != nil {
		return err
	}
	kept := make([]*auth.Session, 0, len(sessions))
	for _, sess := range sessions {
		if sess.UserID == userID && (exceptTokenHash == "" || sess.TokenHash != exceptTokenHash) {
			continue
		}
		kept = append(kept, sess)
	}
	return s.save(kept)
}

func (s *SessionStore) TouchSession(ctx context.Context, tokenHash string, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.load()
	if err != nil {
		return err
	}
	found := false
	for _, sess := range sessions {
		if sess.TokenHash == tokenHash {
			sess.ExpiresAt = expires
			found = true
			break
		}
	}
	if !found {
		return auth.ErrNotFound
	}
	return s.save(sessions)
}

// PasswordResetStore is a file-backed auth.PasswordResetStore.
type PasswordResetStore struct {
	path string
	mu   sync.Mutex
}

// NewPasswordResetStore returns a PasswordResetStore backed by <root>/resets.yml.
func NewPasswordResetStore(root string) *PasswordResetStore {
	return &PasswordResetStore{path: filepath.Join(root, "resets.yml")}
}

type resetsDoc struct {
	Resets []*auth.PasswordReset `yaml:"resets"`
}

func (s *PasswordResetStore) load() ([]*auth.PasswordReset, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read resets file: %w", err)
	}
	var doc resetsDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse resets file: %w", err)
	}
	return doc.Resets, nil
}

func (s *PasswordResetStore) save(resets []*auth.PasswordReset) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return atomicfile.Write(s.path, 0600, func(f *os.File) error {
		enc := yaml.NewEncoder(f)
		if err := enc.Encode(resetsDoc{Resets: resets}); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	})
}

func pruneResets(resets []*auth.PasswordReset, now time.Time) []*auth.PasswordReset {
	kept := resets[:0]
	for _, reset := range resets {
		if now.Before(reset.ExpiresAt) {
			kept = append(kept, reset)
		}
	}
	return kept
}

func (s *PasswordResetStore) CreateReset(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	resets, err := s.load()
	if err != nil {
		return err
	}
	resets = pruneResets(resets, time.Now())
	kept := make([]*auth.PasswordReset, 0, len(resets)+1)
	for _, reset := range resets {
		if reset.UserID != userID {
			kept = append(kept, reset)
		}
	}
	kept = append(kept, &auth.PasswordReset{TokenHash: tokenHash, UserID: userID, ExpiresAt: expires})
	return s.save(kept)
}

func (s *PasswordResetStore) ConsumeReset(ctx context.Context, tokenHash string) (*auth.PasswordReset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resets, err := s.load()
	if err != nil {
		return nil, err
	}
	var found *auth.PasswordReset
	kept := make([]*auth.PasswordReset, 0, len(resets))
	now := time.Now()
	for _, reset := range resets {
		if reset.TokenHash == tokenHash {
			found = reset
			continue
		}
		if now.Before(reset.ExpiresAt) {
			kept = append(kept, reset)
		}
	}
	if found == nil {
		return nil, auth.ErrNotFound
	}
	if err := s.save(kept); err != nil {
		return nil, err
	}
	if now.After(found.ExpiresAt) {
		return nil, auth.ErrNotFound
	}
	cp := *found
	return &cp, nil
}

func (s *PasswordResetStore) DeleteResetsForUser(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	resets, err := s.load()
	if err != nil {
		return err
	}
	resets = pruneResets(resets, time.Now())
	kept := make([]*auth.PasswordReset, 0, len(resets))
	for _, reset := range resets {
		if reset.UserID != userID {
			kept = append(kept, reset)
		}
	}
	return s.save(kept)
}

// Compile-time assertions.
var (
	_ auth.UserStore          = (*UserStore)(nil)
	_ auth.SessionStore       = (*SessionStore)(nil)
	_ auth.PasswordResetStore = (*PasswordResetStore)(nil)
)
