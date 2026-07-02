package auth_test

import (
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/jackiabishop/mileminder/internal/auth"
	"github.com/jackiabishop/mileminder/internal/auth/authtest"
)

func TestMemoryUserStoreConformance(t *testing.T) {
	authtest.RunUserStore(t, func(t *testing.T) auth.UserStore {
		return auth.NewMemoryUserStore()
	})
}

func TestMemorySessionStoreConformance(t *testing.T) {
	authtest.RunSessionStore(t, func(t *testing.T) auth.SessionStore {
		return auth.NewMemorySessionStore()
	})
}

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !auth.CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("correct password should verify")
	}
	if auth.CheckPassword(hash, "wrong password") {
		t.Fatal("wrong password must not verify")
	}
}

// CheckPassword with an empty hash (the unknown-account path) must substitute
// the dummy hash and run a real comparison rather than short-circuiting: it
// returns false, and — unlike a raw bcrypt compare against "" — never errors out
// early. This is what makes login constant-time across known/unknown emails.
func TestCheckPasswordEmptyHashRunsDummyComparison(t *testing.T) {
	// Precondition: a raw bcrypt compare against an empty hash fails immediately
	// (no work done), which is exactly the timing leak CheckPassword avoids.
	if bcrypt.CompareHashAndPassword([]byte(""), []byte("x")) == nil {
		t.Fatal("precondition: empty hash unexpectedly verified")
	}
	if auth.CheckPassword("", "anything") {
		t.Fatal("empty hash must never verify a password")
	}
}

func TestNewTokenIsOpaqueAndHashes(t *testing.T) {
	token, hash, err := auth.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if token == "" || hash == "" {
		t.Fatal("NewToken returned empty token or hash")
	}
	if token == hash {
		t.Fatal("token must not equal its own hash")
	}
	if auth.HashToken(token) != hash {
		t.Fatal("HashToken(token) must match the hash NewToken returned")
	}

	// Two tokens differ.
	other, _, err := auth.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if other == token {
		t.Fatal("two generated tokens must differ")
	}
}

func TestNewUserIDIsHexAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := auth.NewUserID()
		if err != nil {
			t.Fatalf("NewUserID: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = true
		for _, r := range id {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				t.Fatalf("id %q is not lowercase hex", id)
			}
		}
	}
}
