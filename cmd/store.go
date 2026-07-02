package cmd

import (
	"fmt"

	"github.com/jackiabishop/mileminder/internal/storage"
	"github.com/jackiabishop/mileminder/internal/storage/yamlstore"
)

// openStore returns the storage.Store the CLI operates against: the historical
// YAML backend rooted at ~/.mileminder.
func openStore() (storage.Store, error) {
	dir, err := yamlstore.DefaultDir()
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	return yamlstore.New(dir), nil
}
