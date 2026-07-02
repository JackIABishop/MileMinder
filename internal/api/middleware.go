package api

import (
	"net/http"

	"github.com/jackiabishop/mileminder/internal/storage"
)

// middleware wraps a handler with cross-cutting behaviour. The data-route group
// is wrapped by exactly one of these — the mode fork — so handlers below the
// line are mode-blind.
type middleware func(http.Handler) http.Handler

// singleUser is the self-hosted / single-user mode: it injects the one
// process-wide store into every request and does no authentication. Behaviour is
// identical to before Phase 2 — no login, no user concept.
func singleUser(store storage.Store) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(withStore(r.Context(), store)))
		})
	}
}
