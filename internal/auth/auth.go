// Package auth holds MileMinder's identity primitives for hosted mode: the User
// and Session models, the store interfaces behind which they live, password
// hashing, and opaque session-token generation. It is used only when the server
// runs with --hosted; the single-user self-hosted binary and the CLI never touch
// it.
//
// Like the storage package, the store interfaces here are SQL-shaped: the
// interim file-backed implementations (internal/auth/filestore) map directly
// onto the Postgres tables Phase 3 introduces, so swapping the backend does not
// touch the handlers.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor for password hashing. 12 is a common modern
// default; the cost is embedded in the hash string, so it can be raised later
// with a transparent rehash-on-next-login.
const bcryptCost = 12

var (
	// ErrNotFound is returned when a user or session does not exist, or when a
	// session has expired. Callers use errors.Is(err, ErrNotFound).
	ErrNotFound = errors.New("not found")

	// ErrEmailTaken is returned by CreateUser when the email is already
	// registered.
	ErrEmailTaken = errors.New("email already registered")
)

// User is an account. PasswordHash is a bcrypt hash — the only credential today,
// but the model is deliberately open to more (e.g. an Apple subject in Phase 5).
type User struct {
	ID           string    `yaml:"id" json:"id"`
	Email        string    `yaml:"email" json:"email"`
	PasswordHash string    `yaml:"password_hash" json:"-"`
	CreatedAt    time.Time `yaml:"created_at" json:"created_at"`
}

// Session is an opaque-token session. Only the SHA-256 of the token is stored,
// so a leaked session record does not yield a usable token.
type Session struct {
	TokenHash string    `yaml:"token_hash" json:"-"`
	UserID    string    `yaml:"user_id" json:"user_id"`
	ExpiresAt time.Time `yaml:"expires_at" json:"expires_at"`
}

// UserStore persists accounts. Emails are normalised (lower-cased, trimmed) by
// the implementation, so callers may pass raw input.
type UserStore interface {
	// CreateUser creates an account, returning ErrEmailTaken if the email is
	// already registered. The returned User has a server-generated ID.
	CreateUser(ctx context.Context, email, passwordHash string) (*User, error)
	// GetUserByEmail returns the account for an email, or ErrNotFound.
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	// GetUserByID returns the account for an id, or ErrNotFound.
	GetUserByID(ctx context.Context, id string) (*User, error)
}

// SessionStore persists sessions keyed by token hash.
type SessionStore interface {
	CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error
	// GetSession returns the session for a token hash, or ErrNotFound if it does
	// not exist or has expired.
	GetSession(ctx context.Context, tokenHash string) (*Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	// TouchSession extends a session's expiry (the sliding-window refresh).
	TouchSession(ctx context.Context, tokenHash string, expires time.Time) error
}

// NormalizeEmail lower-cases and trims an email so lookups and uniqueness are
// case-insensitive.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// HashPassword returns a bcrypt hash of password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}

// dummyHash is a valid bcrypt hash of an unguessable constant. CheckPassword
// compares against it when no account matches, so a login spends the same bcrypt
// time whether or not the email is registered — closing the user-enumeration
// timing oracle.
var dummyHash = mustHash("mileminder:no-such-account:1f3b8e2c")

func mustHash(s string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(s), bcryptCost)
	if err != nil {
		panic(fmt.Sprintf("auth: precompute dummy hash: %v", err))
	}
	return string(h)
}

// CheckPassword reports whether password matches hash. It always performs a
// bcrypt comparison: pass an empty hash for a missing account and it compares
// against a fixed dummy hash, so wall-clock time does not reveal whether the
// account exists. It never verifies successfully against an empty hash.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		hash = dummyHash
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewToken returns a fresh opaque session token and its hash. The raw token is
// handed to the client; only the hash is ever stored or looked up.
func NewToken() (token, tokenHash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken returns the hex SHA-256 of a token — the form stored and looked up.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewUserID returns a server-generated, path-safe user id (crypto-random hex).
// Being hex, it is safe as a directory name for yamlstore.Tenants.
func NewUserID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate user id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
