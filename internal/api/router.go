package api

import (
	"io/fs"
	"net/http"
)

// NewRouter creates and configures the API router (for dev mode, API only)
func NewRouter(staticDir string) http.Handler {
	mux := http.NewServeMux()
	registerAPIRoutes(mux)

	// Serve static files from disk (for development)
	if staticDir != "" {
		fileServer := http.FileServer(http.Dir(staticDir))
		mux.Handle("/", spaHandlerDir{staticDir: staticDir, fileServer: fileServer})
	}

	return corsMiddleware(mux)
}

// NewRouterWithFS creates a router with embedded filesystem for production
func NewRouterWithFS(staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	registerAPIRoutes(mux)

	// Serve embedded static files
	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("/", spaHandlerFS{staticFS: staticFS, fileServer: fileServer})

	return corsMiddleware(mux)
}

// registerAPIRoutes registers all API endpoints
func registerAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/vehicles", HandleListVehicles)
	mux.HandleFunc("GET /api/vehicles/{id}", HandleGetVehicle)
	mux.HandleFunc("POST /api/vehicles", HandleCreateVehicle)
	mux.HandleFunc("POST /api/vehicles/{id}/readings", HandleAddReading)
	mux.HandleFunc("GET /api/vehicles/{id}/readings", HandleGetReadings)
	mux.HandleFunc("DELETE /api/vehicles/{id}/readings/{date}", HandleDeleteReading)
	mux.HandleFunc("GET /api/vehicles/{id}/graph", HandleGetGraphData)
	mux.HandleFunc("GET /api/vehicles/{id}/export", HandleExportCSV)
	mux.HandleFunc("GET /api/current", HandleGetCurrent)
	mux.HandleFunc("PUT /api/current", HandleSetCurrent)
	mux.HandleFunc("GET /api/fleet", HandleFleet)
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
