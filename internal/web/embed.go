package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedFiles embed.FS

// GetFS returns the embedded filesystem for the web UI
// Returns nil if the dist folder doesn't exist (development mode)
func GetFS() fs.FS {
	distFS, err := fs.Sub(embeddedFiles, "dist")
	if err != nil {
		return nil
	}
	return distFS
}
