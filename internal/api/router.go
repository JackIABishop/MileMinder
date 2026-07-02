package api

import (
	"io/fs"
	"net/http"

	"github.com/jackiabishop/mileminder/internal/storage"
)

// NewRouter creates and configures the API router (for dev mode, API only),
// backed by store.
func NewRouter(store storage.Store, staticDir string) http.Handler {
	mux := http.NewServeMux()
	registerAPIRoutes(mux, NewServer(store))

	// Serve static files from disk (for development)
	if staticDir != "" {
		fileServer := http.FileServer(http.Dir(staticDir))
		mux.Handle("/", spaHandlerDir{staticDir: staticDir, fileServer: fileServer})
	}

	return corsMiddleware(mux)
}

// NewRouterWithFS creates a router with embedded filesystem for production,
// backed by store.
func NewRouterWithFS(store storage.Store, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	registerAPIRoutes(mux, NewServer(store))

	// Serve embedded static files
	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("/", spaHandlerFS{staticFS: staticFS, fileServer: fileServer})

	return corsMiddleware(mux)
}

// registerAPIRoutes registers all API endpoints against s.
func registerAPIRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET /api/vehicles", s.HandleListVehicles)
	mux.HandleFunc("GET /api/vehicles/{id}", s.HandleGetVehicle)
	mux.HandleFunc("POST /api/vehicles", s.HandleCreateVehicle)
	mux.HandleFunc("PATCH /api/vehicles/{id}", s.HandleUpdatePlan)
	mux.HandleFunc("POST /api/vehicles/{id}/readings", s.HandleAddReading)
	mux.HandleFunc("GET /api/vehicles/{id}/readings", s.HandleGetReadings)
	mux.HandleFunc("DELETE /api/vehicles/{id}/readings/{date}", s.HandleDeleteReading)
	mux.HandleFunc("GET /api/vehicles/{id}/graph", s.HandleGetGraphData)
	mux.HandleFunc("GET /api/vehicles/{id}/export", s.HandleExportCSV)
	mux.HandleFunc("GET /api/current", s.HandleGetCurrent)
	mux.HandleFunc("PUT /api/current", s.HandleSetCurrent)
	mux.HandleFunc("GET /api/fleet", s.HandleFleet)
}

// spaHandlerDir serves the SPA from disk, falling back to index.html for client-side routing
type spaHandlerDir struct {
	staticDir  string
	fileServer http.Handler
}

func (h spaHandlerDir) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if file exists
	if _, err := http.Dir(h.staticDir).Open(r.URL.Path); err != nil {
		// File doesn't exist, serve index.html for SPA routing
		r.URL.Path = "/"
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
		// File doesn't exist, serve index.html for SPA routing
		r.URL.Path = "/index.html"
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
