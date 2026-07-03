package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackiabishop/mileminder/internal/auth"
	"github.com/jackiabishop/mileminder/internal/notify"
)

// minPasswordLen is the signup floor. Deliberately low — a length gate stops the
// weakest passwords without pretending to be a strength meter.
const minPasswordLen = 8

const passwordResetTTL = time.Hour

// authAPI holds the hosted-mode auth dependencies. checkPassword is a seam: it
// defaults to auth.CheckPassword but tests substitute a counting wrapper to
// prove login always runs exactly one comparison (the constant-time property).
type authAPI struct {
	users         auth.UserStore
	sessions      auth.SessionStore
	resets        auth.PasswordResetStore
	notifier      notify.Channel
	baseURL       string
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

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// validEmail is a deliberately loose sanity check: a single @ with non-empty
// local and domain parts. Real deliverability verification is the Phase 4 email
// channel's job, not a regex's.
func validEmail(email string) bool {
	local, domain, ok := strings.Cut(email, "@")
	return ok && local != "" && domain != "" && !strings.Contains(domain, "@")
}

func validatePassword(password string) string {
	if len(password) < minPasswordLen {
		return "password must be at least 8 characters"
	}
	return ""
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
	if msg := validatePassword(req.Password); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
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

// HandleChangePassword changes the authenticated user's password. The current
// session survives; every other session and outstanding reset link is revoked.
func (a *authAPI) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	userID := userIDFrom(r.Context())
	user, err := a.users.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if !a.checkPassword(user.PasswordHash, req.CurrentPassword) {
		http.Error(w, "current password is incorrect", http.StatusUnauthorized)
		return
	}
	if msg := validatePassword(req.NewPassword); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, "could not change password", http.StatusInternalServerError)
		return
	}
	if err := a.users.UpdatePassword(r.Context(), userID, hash); err != nil {
		http.Error(w, "could not change password", http.StatusInternalServerError)
		return
	}
	if err := a.sessions.DeleteUserSessions(r.Context(), userID, tokenHashFrom(r.Context())); err != nil {
		http.Error(w, "could not revoke old sessions", http.StatusInternalServerError)
		return
	}
	if a.resets != nil {
		if err := a.resets.DeleteResetsForUser(r.Context(), userID); err != nil {
			http.Error(w, "could not revoke reset links", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "password changed"})
}

// HandleForgotPassword accepts an email address and, for existing accounts,
// sends a reset link asynchronously. Valid-looking emails always receive the
// same response so the endpoint does not reveal account existence.
func (a *authAPI) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	email := auth.NormalizeEmail(req.Email)
	if !validEmail(email) {
		http.Error(w, "a valid email address is required", http.StatusBadRequest)
		return
	}
	if a.resets == nil || a.notifier == nil {
		http.Error(w, "password reset is not configured", http.StatusInternalServerError)
		return
	}

	go a.sendPasswordReset(email)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *authAPI) sendPasswordReset(email string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	user, err := a.users.GetUserByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, auth.ErrNotFound) {
			log.Printf("password reset lookup failed for %q: %v", email, err)
		}
		return
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		log.Printf("password reset token generation failed for %q: %v", email, err)
		return
	}
	if err := a.resets.CreateReset(ctx, tokenHash, user.ID, time.Now().Add(passwordResetTTL)); err != nil {
		log.Printf("password reset store failed for %q: %v", email, err)
		return
	}
	msg, err := renderPasswordResetMessage(a.baseURL, token)
	if err != nil {
		log.Printf("password reset render failed for %q: %v", email, err)
		return
	}
	if err := a.notifier.Send(ctx, notify.Recipient{Email: user.Email}, msg); err != nil {
		log.Printf("password reset send failed for %q: %v", email, err)
	}
}

// HandleResetPassword consumes a reset token, updates the password and revokes
// every session for the account. It deliberately does not start a session.
func (a *authAPI) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if a.resets == nil {
		http.Error(w, "password reset is not configured", http.StatusInternalServerError)
		return
	}
	if msg := validatePassword(req.NewPassword); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	reset, err := a.resets.ConsumeReset(r.Context(), auth.HashToken(req.Token))
	if err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			http.Error(w, "invalid or expired reset link", http.StatusBadRequest)
			return
		}
		http.Error(w, "could not reset password", http.StatusInternalServerError)
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, "could not reset password", http.StatusInternalServerError)
		return
	}
	if err := a.users.UpdatePassword(r.Context(), reset.UserID, hash); err != nil {
		http.Error(w, "invalid or expired reset link", http.StatusBadRequest)
		return
	}
	if err := a.sessions.DeleteUserSessions(r.Context(), reset.UserID, ""); err != nil {
		http.Error(w, "could not revoke sessions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "password reset"})
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
