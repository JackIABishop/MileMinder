package alerts

import (
	"context"
	"fmt"
	"time"
)

// Reminder frequency identifiers. Custom uses CustomInterval + CustomUnit.
const (
	FrequencyDaily     = "daily"
	FrequencyWeekly    = "weekly"
	FrequencyQuarterly = "quarterly"
	FrequencyCustom    = "custom"
)

// Custom interval units. Months is a documented 30-day approximation so the
// interval stays a whole number of days for the flat-interval sweep.
const (
	UnitDays   = "days"
	UnitWeeks  = "weeks"
	UnitMonths = "months"
)

// ReminderSettings is a user's per-vehicle reading-reminder configuration. It is
// stored outside the vehicle document (in reminder_settings.yml), so the core
// model.VehicleData and internal/calc stay untouched.
type ReminderSettings struct {
	UserID         string `yaml:"user_id" json:"user_id"`
	VehicleID      string `yaml:"vehicle_id" json:"vehicle_id"`
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	Frequency      string `yaml:"frequency" json:"frequency"`
	CustomInterval int    `yaml:"custom_interval,omitempty" json:"custom_interval,omitempty"`
	CustomUnit     string `yaml:"custom_unit,omitempty" json:"custom_unit,omitempty"`
}

// DefaultReminderSettings is what an unconfigured vehicle reports: reminders off
// (issue #52: "Default to notifications off for existing cars"), weekly cadence
// as the suggested default once enabled.
func DefaultReminderSettings(userID, vehicleID string) ReminderSettings {
	return ReminderSettings{
		UserID:    userID,
		VehicleID: vehicleID,
		Enabled:   false,
		Frequency: FrequencyWeekly,
	}
}

// IntervalDays resolves the reminder cadence to whole days. Quarterly is 91 days
// (13 weeks); custom multiplies the interval by the unit's day count. Returns 0
// for an unrecognised frequency — callers Validate before relying on this.
func (r ReminderSettings) IntervalDays() int {
	switch r.Frequency {
	case FrequencyDaily:
		return 1
	case FrequencyWeekly:
		return 7
	case FrequencyQuarterly:
		return 91
	case FrequencyCustom:
		return r.CustomInterval * unitDays(r.CustomUnit)
	default:
		return 0
	}
}

func unitDays(unit string) int {
	switch unit {
	case UnitDays:
		return 1
	case UnitWeeks:
		return 7
	case UnitMonths:
		return 30
	default:
		return 0
	}
}

// Validate reports whether the settings are internally consistent. Custom
// frequency requires a positive interval and a known unit; the standard
// frequencies need neither.
func (r ReminderSettings) Validate() error {
	switch r.Frequency {
	case FrequencyDaily, FrequencyWeekly, FrequencyQuarterly:
		return nil
	case FrequencyCustom:
		if r.CustomInterval < 1 {
			return fmt.Errorf("custom reminder requires a custom_interval of at least 1")
		}
		if unitDays(r.CustomUnit) == 0 {
			return fmt.Errorf("custom reminder requires custom_unit of days, weeks or months")
		}
		return nil
	default:
		return fmt.Errorf("unknown reminder frequency %q", r.Frequency)
	}
}

// VehicleReminderState is the scheduler-written state for one user vehicle: when
// the last reminder was sent, so repeat nagging fires at most once per interval.
type VehicleReminderState struct {
	UserID         string    `yaml:"user_id" json:"user_id"`
	VehicleID      string    `yaml:"vehicle_id" json:"vehicle_id"`
	LastRemindedAt time.Time `yaml:"last_reminded_at,omitempty" json:"last_reminded_at,omitempty"`
}

// ReminderSettingsStore persists user-editable per-vehicle reminder settings.
type ReminderSettingsStore interface {
	GetReminder(ctx context.Context, userID, vehicleID string) (*ReminderSettings, error)
	PutReminder(ctx context.Context, r ReminderSettings) error
	PruneUserReminders(ctx context.Context, userID string, keepVehicleIDs []string) error
}

// ReminderStateStore persists scheduler-written reminder send state.
type ReminderStateStore interface {
	GetReminderState(ctx context.Context, userID, vehicleID string) (*VehicleReminderState, error)
	PutReminderState(ctx context.Context, st VehicleReminderState) error
	PruneUserReminderStates(ctx context.Context, userID string, keepVehicleIDs []string) error
}
