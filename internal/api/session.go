package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackiabishop/mileminder/internal/auth"
	"github.com/jackiabishop/mileminder/internal/storage"
)

const (
	// sessionCookie is the name of the session cookie set for the web SPA.
	sessionCookie = "mm_session"

	// sessionTTL is a session's lifetime; sessionRefresh bounds how often the
	// sliding expiry is written (at most once per this interval per session).
	sessionTTL     = 30 * 24 * time.Hour
	sessionRefresh = time.Hour
)

// tokenFromRequest extracts the session token, preferring the Authorization
// Bearer header (native/CLI clients) over the cookie (web SPA). fromCookie
// reports which transport supplied it, because the CSRF check applies only to
// the ambient-credential cookie path.
func tokenFromRequest(r *http.Request) (token string, fromCookie bool) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")), false
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value, true
	}
	return "", false
}

// setSessionCookie writes the session cookie. HttpOnly keeps the token out of
// JS (no XSS theft); SameSite=Lax blocks it on cross-site subrequests; Secure is
// set in hosted mode (TLS terminated at the platform edge).
func setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL / time.Second),
	})
}

// clearSessionCookie expires the session cookie (logout, or a stale token).
func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// csrfOK reports whether a cookie-authenticated, state-changing request is
// allowed by the Sec-Fetch-Site metadata header, which browsers send
// automatically and page JS cannot forge. A missing header (older browser or a
// non-browser client) is allowed — such clients should use Bearer auth, which
// skips this check entirely. Present-and-cross-site is rejected.
func csrfOK(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	if site == "" {
		return true
	}
	return site == "same-origin" || site == "none"
}

// requireSession is the hosted-mode data middleware: it authenticates the
// request from its session token, then injects the user's scoped store (plus the
// user id and token hash) so the mode-blind data handlers run against exactly
// that user's data. No/invalid session → 401.
func requireSession(sessions auth.SessionStore, tenants storage.Tenants, secure bool) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, fromCookie := tokenFromRequest(r)
			if token == "" {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			if fromCookie && !isSafeMethod(r.Method) && !csrfOK(r) {
				http.Error(w, "cross-site request blocked", http.StatusForbidden)
				return
			}

			hash := auth.HashToken(token)
			sess, err := sessions.GetSession(r.Context(), hash)
			if err != nil {
				if errors.Is(err, auth.ErrNotFound) {
					if fromCookie {
						clearSessionCookie(w, secure)
					}
					http.Error(w, "authentication required", http.StatusUnauthorized)
					return
				}
				http.Error(w, "session lookup failed", http.StatusInternalServerError)
				return
			}

			// Sliding expiry: extend only once the last write is older than
			// sessionRefresh, to bound writes on a busy session.
			if time.Until(sess.ExpiresAt) < sessionTTL-sessionRefresh {
				_ = sessions.TouchSession(r.Context(), hash, time.Now().Add(sessionTTL))
			}

			ctx := withStore(r.Context(), tenants.ForUser(sess.UserID))
			ctx = withSession(ctx, sess.UserID, hash)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
