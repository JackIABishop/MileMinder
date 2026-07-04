package api

import (
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/jackiabishop/mileminder/internal/storage"
)

func init() {
	// Go's mime package has no built-in mapping for .webmanifest, so it falls
	// back to content sniffing and serves it as text/plain. Register the
	// correct type so the web app manifest is served as the browsers expect.
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// Server mode, reported at GET /api/v1/meta so one SPA build can serve both:
// single-user hides all auth UI; hosted shows login and gates data on a session.
const (
	modeSingleUser = "single-user"
	modeHosted     = "hosted"
)

// metaResponse is the GET /api/v1/meta envelope.
type metaResponse struct {
	Mode string `json:"mode"`
}

// handleMeta reports the server mode. It requires no auth — the SPA calls it on
// boot, before any login, to decide whether to show the login flow.
func handleMeta(mode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metaResponse{Mode: mode})
	}
}

// NewRouter creates the single-user API router serving static files from disk,
// or API-only when staticDir is "" (the serve --dev path). This is the only
// constructor that opens CORS: dev runs the Vite frontend on a separate origin.
func NewRouter(store storage.Store, staticDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", handleMeta(modeSingleUser))
	registerDataRoutes(mux, NewServer(), singleUser(store))
	if staticDir != "" {
		mountStaticDir(mux, staticDir)
	}
	return corsMiddleware(securityHeaders(mux))
}

// NewRouterWithFS creates the single-user API router serving the embedded SPA
// (production self-hosted binary). No CORS: the SPA is same-origin.
func NewRouterWithFS(store storage.Store, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", handleMeta(modeSingleUser))
	registerDataRoutes(mux, NewServer(), singleUser(store))
	mountStaticFS(mux, staticFS)
	return securityHeaders(mux)
}

// mountStaticDir serves the SPA from disk with client-side-routing fallback.
func mountStaticDir(mux *http.ServeMux, staticDir string) {
	fileServer := http.FileServer(http.Dir(staticDir))
	mux.Handle("/", spaHandlerDir{staticDir: staticDir, fileServer: fileServer})
}

// mountStaticFS serves the SPA from an embedded filesystem with fallback.
func mountStaticFS(mux *http.ServeMux, staticFS fs.FS) {
	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("/", spaHandlerFS{staticFS: staticFS, fileServer: fileServer})
}

// registerDataRoutes registers the vehicle/current/fleet data endpoints against
// s, each wrapped by the mode middleware (data). The mode middleware injects the
// storage.Store the handler reads via storeFrom: the process-wide store in
// single-user mode, or the authenticated user's scoped store in hosted mode.
// Handlers themselves are mode-blind.
func registerDataRoutes(mux *http.ServeMux, s *Server, data middleware) {
	d := func(h http.HandlerFunc) http.Handler { return data(h) }
	mux.Handle("GET /api/v1/vehicles", d(s.HandleListVehicles))
	mux.Handle("GET /api/v1/vehicles/{id}", d(s.HandleGetVehicle))
	mux.Handle("POST /api/v1/vehicles", d(s.HandleCreateVehicle))
	mux.Handle("PATCH /api/v1/vehicles/{id}", d(s.HandleUpdateVehicle))
	mux.Handle("POST /api/v1/vehicles/{id}/readings", d(s.HandleAddReading))
	mux.Handle("GET /api/v1/vehicles/{id}/readings", d(s.HandleGetReadings))
	mux.Handle("DELETE /api/v1/vehicles/{id}/readings/{date}", d(s.HandleDeleteReading))
	mux.Handle("GET /api/v1/vehicles/{id}/graph", d(s.HandleGetGraphData))
	mux.Handle("GET /api/v1/vehicles/{id}/export", d(s.HandleExportCSV))
	mux.Handle("GET /api/v1/vehicles/{id}/profile", d(s.HandleExportProfile))
	mux.Handle("POST /api/v1/vehicles/{id}/import", d(s.HandleImportCSV))
	mux.Handle("GET /api/v1/current", d(s.HandleGetCurrent))
	mux.Handle("PUT /api/v1/current", d(s.HandleSetCurrent))
	mux.Handle("GET /api/v1/fleet", d(s.HandleFleet))
	mux.Handle("GET /api/v1/settings", d(s.HandleGetSettings))
	mux.Handle("PUT /api/v1/settings", d(s.HandlePutSettings))
}

// spaHandlerDir serves the SPA from disk, falling back to index.html for client-side routing
type spaHandlerDir struct {
	staticDir  string
	fileServer http.Handler
}

func (h spaHandlerDir) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if file exists
	if _, err := http.Dir(h.staticDir).Open(r.URL.Path); err != nil {
		// File doesn't exist: serve index.html directly for client-side routing,
		// rather than rewriting r.URL.Path and delegating to the generic file
		// server. net/http's serveFile 301-redirects to "./" whenever the
		// *request's* URL path ends in "/index.html" (to canonicalize direct
		// index.html requests) — rewriting the path would spuriously trigger
		// that for every unknown route (e.g. a fresh navigation to /quick-add),
		// bouncing it back to "/" instead of serving the SPA shell.
		http.ServeFile(w, r, filepath.Join(h.staticDir, "index.html"))
		return
	}
	h.fileServer.ServeHTTP(w, r)
}

// spaHandlerFS serves the SPA from embedded filesystem
type spaHandlerFS struct {
	staticFS   fs.FS
	fileServer http.Handler
}

func (h spaHandlerFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if file exists in embedded FS
	path := r.URL.Path
	if path == "/" {
		path = "index.html"
	} else if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	if _, err := fs.Stat(h.staticFS, path); err != nil {
		// File doesn't exist: serve index.html directly (see the identical
		// comment in spaHandlerDir for why we don't rewrite r.URL.Path and
		// delegate to the generic file server here).
		http.ServeFileFS(w, r, h.staticFS, "index.html")
		return
	}
	h.fileServer.ServeHTTP(w, r)
}

// corsMiddleware adds CORS headers for development
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
