package alerts

import (
	"context"
	"errors"
	"testing"
)

func TestReminderIntervalDays(t *testing.T) {
	cases := []struct {
		name string
		r    ReminderSettings
		want int
	}{
		{"daily", ReminderSettings{Frequency: FrequencyDaily}, 1},
		{"weekly", ReminderSettings{Frequency: FrequencyWeekly}, 7},
		{"quarterly", ReminderSettings{Frequency: FrequencyQuarterly}, 91},
		{"custom days", ReminderSettings{Frequency: FrequencyCustom, CustomInterval: 3, CustomUnit: UnitDays}, 3},
		{"custom weeks", ReminderSettings{Frequency: FrequencyCustom, CustomInterval: 2, CustomUnit: UnitWeeks}, 14},
		{"custom months", ReminderSettings{Frequency: FrequencyCustom, CustomInterval: 2, CustomUnit: UnitMonths}, 60},
		{"unknown", ReminderSettings{Frequency: "yearly"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.IntervalDays(); got != tc.want {
				t.Fatalf("IntervalDays() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestReminderValidate(t *testing.T) {
	cases := []struct {
		name    string
		r       ReminderSettings
		wantErr bool
	}{
		{"daily ok", ReminderSettings{Frequency: FrequencyDaily}, false},
		{"weekly ok", ReminderSettings{Frequency: FrequencyWeekly}, false},
		{"quarterly ok", ReminderSettings{Frequency: FrequencyQuarterly}, false},
		{"custom ok", ReminderSettings{Frequency: FrequencyCustom, CustomInterval: 1, CustomUnit: UnitDays}, false},
		{"custom zero interval", ReminderSettings{Frequency: FrequencyCustom, CustomInterval: 0, CustomUnit: UnitDays}, true},
		{"custom bad unit", ReminderSettings{Frequency: FrequencyCustom, CustomInterval: 2, CustomUnit: "years"}, true},
		{"unknown frequency", ReminderSettings{Frequency: "monthly"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.r.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDefaultReminderSettings(t *testing.T) {
	d := DefaultReminderSettings("u1", "golf")
	if d.Enabled {
		t.Fatalf("default reminders should be off, got enabled")
	}
	if d.Frequency != FrequencyWeekly {
		t.Fatalf("default frequency = %q, want weekly", d.Frequency)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("default settings should validate: %v", err)
	}
}

// reminderSettingsStores runs the shared assertions against both the memory and
// file-backed settings stores.
func reminderSettingsStores(t *testing.T) map[string]ReminderSettingsStore {
	return map[string]ReminderSettingsStore{
		"memory": NewMemoryReminderSettingsStore(),
		"file":   NewFileReminderSettingsStore(t.TempDir()),
	}
}

func TestReminderSettingsStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	for name, store := range reminderSettingsStores(t) {
		t.Run(name, func(t *testing.T) {
			if _, err := store.GetReminder(ctx, "u1", "golf"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("unset GetReminder: want ErrNotFound, got %v", err)
			}
			r := ReminderSettings{UserID: "u1", VehicleID: "golf", Enabled: true, Frequency: FrequencyWeekly}
			if err := store.PutReminder(ctx, r); err != nil {
				t.Fatalf("PutReminder: %v", err)
			}
			got, err := store.GetReminder(ctx, "u1", "golf")
			if err != nil {
				t.Fatalf("GetReminder: %v", err)
			}
			if got.Frequency != FrequencyWeekly || !got.Enabled {
				t.Fatalf("round-trip mismatch: %+v", got)
			}
			// Upsert replaces.
			r.Frequency = FrequencyDaily
			if err := store.PutReminder(ctx, r); err != nil {
				t.Fatalf("PutReminder update: %v", err)
			}
			got, _ = store.GetReminder(ctx, "u1", "golf")
			if got.Frequency != FrequencyDaily {
				t.Fatalf("update not persisted: %+v", got)
			}
		})
	}
}

func TestReminderSettingsStoreValidationAndIDs(t *testing.T) {
	ctx := context.Background()
	for name, store := range reminderSettingsStores(t) {
		t.Run(name, func(t *testing.T) {
			if err := store.PutReminder(ctx, ReminderSettings{VehicleID: "golf", Frequency: FrequencyDaily}); err == nil {
				t.Fatal("PutReminder without user_id: want error")
			}
			if err := store.PutReminder(ctx, ReminderSettings{UserID: "u1", Frequency: FrequencyDaily}); err == nil {
				t.Fatal("PutReminder without vehicle_id: want error")
			}
			bad := ReminderSettings{UserID: "u1", VehicleID: "golf", Frequency: FrequencyCustom, CustomInterval: 0, CustomUnit: UnitDays}
			if err := store.PutReminder(ctx, bad); err == nil {
				t.Fatal("PutReminder with invalid custom interval: want error")
			}
		})
	}
}

func TestReminderSettingsStorePrune(t *testing.T) {
	ctx := context.Background()
	for name, store := range reminderSettingsStores(t) {
		t.Run(name, func(t *testing.T) {
			for _, id := range []string{"golf", "polo"} {
				if err := store.PutReminder(ctx, ReminderSettings{UserID: "u1", VehicleID: id, Frequency: FrequencyWeekly}); err != nil {
					t.Fatalf("PutReminder %s: %v", id, err)
				}
			}
			// Other user's settings must survive a prune scoped to u1.
			if err := store.PutReminder(ctx, ReminderSettings{UserID: "u2", VehicleID: "golf", Frequency: FrequencyWeekly}); err != nil {
				t.Fatalf("PutReminder u2: %v", err)
			}
			if err := store.PruneUserReminders(ctx, "u1", []string{"golf"}); err != nil {
				t.Fatalf("PruneUserReminders: %v", err)
			}
			if _, err := store.GetReminder(ctx, "u1", "polo"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("pruned polo still present: %v", err)
			}
			if _, err := store.GetReminder(ctx, "u1", "golf"); err != nil {
				t.Fatalf("kept golf missing: %v", err)
			}
			if _, err := store.GetReminder(ctx, "u2", "golf"); err != nil {
				t.Fatalf("other user's golf pruned: %v", err)
			}
		})
	}
}

func TestFileReminderStorePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s1 := NewFileReminderSettingsStore(dir)
	if err := s1.PutReminder(ctx, ReminderSettings{UserID: "u1", VehicleID: "golf", Enabled: true, Frequency: FrequencyQuarterly}); err != nil {
		t.Fatalf("PutReminder: %v", err)
	}
	s2 := NewFileReminderSettingsStore(dir)
	got, err := s2.GetReminder(ctx, "u1", "golf")
	if err != nil {
		t.Fatalf("GetReminder after reopen: %v", err)
	}
	if got.Frequency != FrequencyQuarterly || !got.Enabled {
		t.Fatalf("persisted mismatch: %+v", got)
	}
}

func TestReminderStateStoreRoundTripAndPrune(t *testing.T) {
	ctx := context.Background()
	stores := map[string]ReminderStateStore{
		"memory": NewMemoryReminderStateStore(),
		"file":   NewFileReminderStateStore(t.TempDir()),
	}
	for name, store := range stores {
		t.Run(name, func(t *testing.T) {
			if _, err := store.GetReminderState(ctx, "u1", "golf"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("unset state: want ErrNotFound, got %v", err)
			}
			now := alertDate("2025-04-11")
			if err := store.PutReminderState(ctx, VehicleReminderState{UserID: "u1", VehicleID: "golf", LastRemindedAt: now}); err != nil {
				t.Fatalf("PutReminderState: %v", err)
			}
			got, err := store.GetReminderState(ctx, "u1", "golf")
			if err != nil {
				t.Fatalf("GetReminderState: %v", err)
			}
			if !got.LastRemindedAt.Equal(now) {
				t.Fatalf("state timestamp = %v, want %v", got.LastRemindedAt, now)
			}
			if err := store.PruneUserReminderStates(ctx, "u1", nil); err != nil {
				t.Fatalf("PruneUserReminderStates: %v", err)
			}
			if _, err := store.GetReminderState(ctx, "u1", "golf"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("pruned state still present: %v", err)
			}
		})
	}
}
