package alerts

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// MemoryReminderSettingsStore is an in-memory ReminderSettingsStore for tests.
type MemoryReminderSettingsStore struct {
	mu        sync.Mutex
	reminders map[string]ReminderSettings
}

func NewMemoryReminderSettingsStore() *MemoryReminderSettingsStore {
	return &MemoryReminderSettingsStore{reminders: map[string]ReminderSettings{}}
}

func (m *MemoryReminderSettingsStore) GetReminder(ctx context.Context, userID, vehicleID string) (*ReminderSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reminders[stateKey(userID, vehicleID)]
	if !ok {
		return nil, ErrNotFound
	}
	cp := r
	return &cp, nil
}

func (m *MemoryReminderSettingsStore) PutReminder(ctx context.Context, r ReminderSettings) error {
	if r.UserID == "" || r.VehicleID == "" {
		return fmt.Errorf("reminder settings require user_id and vehicle_id")
	}
	if err := r.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reminders[stateKey(r.UserID, r.VehicleID)] = r
	return nil
}

func (m *MemoryReminderSettingsStore) PruneUserReminders(ctx context.Context, userID string, keepVehicleIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := map[string]bool{}
	for _, id := range keepVehicleIDs {
		keep[id] = true
	}
	for _, r := range m.reminders {
		if r.UserID == userID && !keep[r.VehicleID] {
			delete(m.reminders, stateKey(r.UserID, r.VehicleID))
		}
	}
	return nil
}

// MemoryReminderStateStore is an in-memory ReminderStateStore for tests.
type MemoryReminderStateStore struct {
	mu     sync.Mutex
	states map[string]VehicleReminderState
}

func NewMemoryReminderStateStore() *MemoryReminderStateStore {
	return &MemoryReminderStateStore{states: map[string]VehicleReminderState{}}
}

func (m *MemoryReminderStateStore) GetReminderState(ctx context.Context, userID, vehicleID string) (*VehicleReminderState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.states[stateKey(userID, vehicleID)]
	if !ok {
		return nil, ErrNotFound
	}
	cp := st
	return &cp, nil
}

func (m *MemoryReminderStateStore) PutReminderState(ctx context.Context, st VehicleReminderState) error {
	if st.UserID == "" || st.VehicleID == "" {
		return fmt.Errorf("reminder state requires user_id and vehicle_id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[stateKey(st.UserID, st.VehicleID)] = st
	return nil
}

func (m *MemoryReminderStateStore) PruneUserReminderStates(ctx context.Context, userID string, keepVehicleIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := map[string]bool{}
	for _, id := range keepVehicleIDs {
		keep[id] = true
	}
	for _, st := range m.states {
		if st.UserID == userID && !keep[st.VehicleID] {
			delete(m.states, stateKey(st.UserID, st.VehicleID))
		}
	}
	return nil
}

// Snapshot returns states sorted by user then vehicle for tests.
func (m *MemoryReminderStateStore) Snapshot() []VehicleReminderState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]VehicleReminderState, 0, len(m.states))
	for _, st := range m.states {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UserID == out[j].UserID {
			return out[i].VehicleID < out[j].VehicleID
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

var (
	_ ReminderSettingsStore = (*MemoryReminderSettingsStore)(nil)
	_ ReminderStateStore    = (*MemoryReminderStateStore)(nil)
)
