// Package alerts implements hosted-mode mileage alerting: preferences, persisted
// crossing state, message rendering and the scheduler core.
package alerts

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

// VehicleAlertState is the persisted edge-trigger state for one user vehicle.
type VehicleAlertState struct {
	UserID        string    `yaml:"user_id" json:"user_id"`
	VehicleID     string    `yaml:"vehicle_id" json:"vehicle_id"`
	Breached      bool      `yaml:"breached" json:"breached"`
	LastAlertedAt time.Time `yaml:"last_alerted_at,omitempty" json:"last_alerted_at,omitempty"`
}

// StateStore persists alert edge state.
type StateStore interface {
	GetState(ctx context.Context, userID, vehicleID string) (*VehicleAlertState, error)
	PutState(ctx context.Context, st VehicleAlertState) error
}

// PruningStateStore is optionally implemented by stores that can remove state
// for vehicles no longer present in a user's garage.
type PruningStateStore interface {
	PruneUserStates(ctx context.Context, userID string, keepVehicleIDs []string) error
}

// Prefs stores a user's alert settings.
type Prefs struct {
	UserID    string  `yaml:"user_id" json:"user_id"`
	Enabled   bool    `yaml:"enabled" json:"enabled"`
	Threshold float64 `yaml:"threshold" json:"threshold"`
}

// DefaultPrefs returns the default preference set for a user.
func DefaultPrefs(userID string) Prefs {
	return Prefs{UserID: userID, Enabled: true, Threshold: 100}
}

// PrefsStore persists user alert settings.
type PrefsStore interface {
	GetPrefs(ctx context.Context, userID string) (*Prefs, error)
	PutPrefs(ctx context.Context, p Prefs) error
}
