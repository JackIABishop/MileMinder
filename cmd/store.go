package cmd

import (
	"context"
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

// defaultVehicleID resolves the vehicle id for commands that accept an optional
// --car flag and otherwise fall back to the stored default (status, graph). It
// returns an actionable error when neither is available.
func defaultVehicleID(ctx context.Context, st storage.Store, carFlag string) (string, error) {
	if carFlag != "" {
		return carFlag, nil
	}
	current, err := st.GetCurrent(ctx)
	if err != nil {
		return "", err
	}
	if current == "" {
		return "", fmt.Errorf("no vehicle specified and no default set; use --car or switch")
	}
	return current, nil
}
