// Package atomicfile writes a file by streaming into a temp file in the same
// directory and renaming it into place. A reader therefore never observes a
// torn/partial file, and a crash mid-write cannot truncate an existing file.
//
// It is the shared primitive behind both the YAML vehicle store and the
// file-backed user/session stores, so the durable-write policy lives in one
// place.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write atomically writes path with the given permissions. write is responsible
// for producing the file's contents; it receives an open, truncated temp file.
// The parent directory must already exist.
func Write(path string, perm os.FileMode, write func(*os.File) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename.
	defer os.Remove(tmpName)

	// os.CreateTemp makes the file 0600; set the caller's perms explicitly.
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := write(tmp); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename into place %s: %w", path, err)
	}
	return nil
}
