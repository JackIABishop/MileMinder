package api

import (
	"io/fs"
	"net/http"

	"golang.org/x/time/rate"

	"github.com/jackiabishop/mileminder/internal/alerts"
	"github.com/jackiabishop/mileminder/internal/auth"
	"github.com/jackiabishop/mileminder/internal/notify"
	"github.com/jackiabishop/mileminder/internal/storage"
)

// HostedConfig configures a multi-tenant (hosted) server: the auth stores, the
// per-user Tenants factory, and cookie hardening.
type HostedConfig struct {
	Users    auth.UserStore
	Sessions auth.SessionStore
	Resets   auth.PasswordResetStore
	Tenants  storage.Tenants

	// Notifier is wired in hosted mode for flows that need outbound messages.
	// Alert scheduling uses it outside the router; password reset uses it here.
	Notifier notify.Channel
	BaseURL  string

	AlertPrefs alerts.PrefsStore

	// SecureCookies sets the Secure flag on session cookies. True in real hosted
	// deployments (TLS terminated at the edge); left false for plain-HTTP tests.
	SecureCookies bool

	// CheckPassword overrides the password comparison. Nil uses auth.CheckPassword;
	// tests inject a counting hook to assert login's constant-time property.
	CheckPassword func(hash, password string) bool

	// AuthRatePerSec and AuthRateBurst override the per-IP auth-endpoint rate
	// limiter. Zero values use the production defaults. Tests raise the burst so
	// unrelated auth calls don't trip it, and the rate-limit test lowers it.
	AuthRatePerSec float64
	AuthRateBurst  int
}

// NewHostedRouter builds the hosted, multi-tenant handler serving the embedded
// SPA. Every data endpoint sits behind a session; signup/login are open but
// rate-limited. CORS is not opened: the SPA is served same-origin and native
// clients do not need it.
func NewHostedRouter(cfg HostedConfig, staticFS fs.FS) http.Handler {
	mux := hostedMux(cfg)
	mountStaticFS(mux, staticFS)
	return securityHeaders(mux)
}

// NewHostedRouterDir builds the hosted handler serving static files from disk,
// or API-only when staticDir is "" (dev and tests).
func NewHostedRouterDir(cfg HostedConfig, staticDir string) http.Handler {
	mux := hostedMux(cfg)
	if staticDir != "" {
		mountStaticDir(mux, staticDir)
	}
	return securityHeaders(mux)
}

// hostedMux wires the hosted API surface: open meta + rate-limited auth
// endpoints, and session-gated introspection + data routes.
func hostedMux(cfg HostedConfig) *http.ServeMux {
	check := cfg.CheckPassword
	if check == nil {
		check = auth.CheckPassword
	}
	a := &authAPI{
		users:         cfg.Users,
		sessions:      cfg.Sessions,
		resets:        cfg.Resets,
		notifier:      cfg.Notifier,
		baseURL:       cfg.BaseURL,
		checkPassword: check,
		secureCookies: cfg.SecureCookies,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", handleMeta(modeHosted))

	// Open, rate-limited auth endpoints: ~0.2 req/s with a burst of 5 per IP is
	// ample for a human and painful for a brute-forcer.
	limitPerSec, burst := rate.Limit(0.2), 5
	if cfg.AuthRatePerSec > 0 {
		limitPerSec = rate.Limit(cfg.AuthRatePerSec)
	}
	if cfg.AuthRateBurst > 0 {
		burst = cfg.AuthRateBurst
	}
	limiter := newIPRateLimiter(limitPerSec, burst)
	mux.Handle("POST /api/v1/auth/signup", limiter.wrap(http.HandlerFunc(a.HandleSignup)))
	mux.Handle("POST /api/v1/auth/login", limiter.wrap(http.HandlerFunc(a.HandleLogin)))
	if cfg.Resets != nil && cfg.Notifier != nil {
		mux.Handle("POST /api/v1/auth/forgot", limiter.wrap(http.HandlerFunc(a.HandleForgotPassword)))
		mux.Handle("POST /api/v1/auth/reset", limiter.wrap(http.HandlerFunc(a.HandleResetPassword)))
	}

	// Session-required routes: auth introspection + all data.
	sess := requireSession(cfg.Sessions, cfg.Tenants, cfg.SecureCookies)
	mux.Handle("POST /api/v1/auth/logout", sess(http.HandlerFunc(a.HandleLogout)))
	mux.Handle("GET /api/v1/auth/me", sess(http.HandlerFunc(a.HandleMe)))
	mux.Handle("POST /api/v1/auth/password", sess(http.HandlerFunc(a.HandleChangePassword)))
	if cfg.AlertPrefs != nil {
		alertsAPI := &alertPrefsAPI{prefs: cfg.AlertPrefs}
		mux.Handle("GET /api/v1/alerts/prefs", sess(http.HandlerFunc(alertsAPI.HandleGetPrefs)))
		mux.Handle("PUT /api/v1/alerts/prefs", sess(http.HandlerFunc(alertsAPI.HandlePutPrefs)))
	}
	registerDataRoutes(mux, NewServer(), sess)

	return mux
}
