package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackiabishop/mileminder/internal/auth"
)

// minPasswordLen is the signup floor. Deliberately low — a length gate stops the
// weakest passwords without pretending to be a strength meter.
const minPasswordLen = 8

// authAPI holds the hosted-mode auth dependencies. checkPassword is a seam: it
// defaults to auth.CheckPassword but tests substitute a counting wrapper to
// prove login always runs exactly one comparison (the constant-time property).
type authAPI struct {
	users         auth.UserStore
	sessions      auth.SessionStore
	checkPassword func(hash, password string) bool
	secureCookies bool
}

// credentials is the shared signup/login request body.
type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// authResponse is the signup/login success body. The token is also set as a
// cookie; it is returned here too so native/CLI clients (which don't use the
// cookie) can store it in the keychain.
type authResponse struct {
	Token string     `json:"token"`
	User  *auth.User `json:"user"`
}

// validEmail is a deliberately loose sanity check: a single @ with non-empty
// local and domain parts. Real deliverability verification is the Phase 4 email
// channel's job, not a regex's.
func validEmail(email string) bool {
	local, domain, ok := strings.Cut(email, "@")
	return ok && local != "" && domain != "" && !strings.Contains(domain, "@")
}

// HandleSignup creates an account and starts a session. It validates input
// (email shape, password length) with a 400, and reports an already-registered
// email with a 409.
func (a *authAPI) HandleSignup(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Email = auth.NormalizeEmail(req.Email)

	if !validEmail(req.Email) {
		http.Error(w, "a valid email address is required", http.StatusBadRequest)
		return
	}
	if len(req.Password) < minPasswordLen {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "could not create account", http.StatusInternalServerError)
		return
	}

	user, err := a.users.CreateUser(r.Context(), req.Email, hash)
	if err != nil {
		if errors.Is(err, auth.ErrEmailTaken) {
			http.Error(w, "email already registered", http.StatusConflict)
			return
		}
		http.Error(w, "could not create account", http.StatusInternalServerError)
		return
	}

	a.startSessionResponse(w, r, user, http.StatusCreated)
}

// HandleLogin verifies credentials and starts a session. It runs a bcrypt
// comparison on every attempt — against a dummy hash when the email is unknown
// (see auth.CheckPassword) — so response time does not reveal whether an account
// exists. Both unknown-email and wrong-password yield the same generic 401.
func (a *authAPI) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Email = auth.NormalizeEmail(req.Email)

	user, err := a.users.GetUserByEmail(r.Context(), req.Email)
	if err != nil && !errors.Is(err, auth.ErrNotFound) {
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}

	// Always compare exactly once, whether or not the user exists: an empty hash
	// makes CheckPassword compare against its dummy hash, keeping timing flat.
	hash := ""
	if user != nil {
		hash = user.PasswordHash
	}
	ok := a.checkPassword(hash, req.Password)

	if user == nil || !ok {
		http.Error(w, "invalid email or password", http.StatusUnauthorized)
		return
	}

	a.startSessionResponse(w, r, user, http.StatusOK)
}

// HandleLogout revokes the current session and clears the cookie.
func (a *authAPI) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if hash := tokenHashFrom(r.Context()); hash != "" {
		if err := a.sessions.DeleteSession(r.Context(), hash); err != nil {
			http.Error(w, "logout failed", http.StatusInternalServerError)
			return
		}
	}
	clearSessionCookie(w, a.secureCookies)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

// HandleMe returns the authenticated user.
func (a *authAPI) HandleMe(w http.ResponseWriter, r *http.Request) {
	user, err := a.users.GetUserByID(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		// The session referenced a user that no longer exists.
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// startSessionResponse mints a session, sets the cookie, and writes the token +
// user body with the given status.
func (a *authAPI) startSessionResponse(w http.ResponseWriter, r *http.Request, user *auth.User, status int) {
	token, hash, err := auth.NewToken()
	if err != nil {
		http.Error(w, "could not start session", http.StatusInternalServerError)
		return
	}
	if err := a.sessions.CreateSession(r.Context(), hash, user.ID, time.Now().Add(sessionTTL)); err != nil {
		http.Error(w, "could not start session", http.StatusInternalServerError)
		return
	}

	setSessionCookie(w, token, a.secureCookies)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(authResponse{Token: token, User: user})
}
