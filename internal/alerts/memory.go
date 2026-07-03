package alerts

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// MemoryStateStore is an in-memory StateStore for tests.
type MemoryStateStore struct {
	mu     sync.Mutex
	states map[string]VehicleAlertState
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{states: map[string]VehicleAlertState{}}
}

func stateKey(userID, vehicleID string) string {
	return userID + "\x00" + vehicleID
}

func (m *MemoryStateStore) GetState(ctx context.Context, userID, vehicleID string) (*VehicleAlertState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.states[stateKey(userID, vehicleID)]
	if !ok {
		return nil, ErrNotFound
	}
	cp := st
	return &cp, nil
}

func (m *MemoryStateStore) PutState(ctx context.Context, st VehicleAlertState) error {
	if st.UserID == "" || st.VehicleID == "" {
		return fmt.Errorf("alert state requires user_id and vehicle_id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[stateKey(st.UserID, st.VehicleID)] = st
	return nil
}

func (m *MemoryStateStore) PruneUserStates(ctx context.Context, userID string, keepVehicleIDs []string) error {
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
func (m *MemoryStateStore) Snapshot() []VehicleAlertState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]VehicleAlertState, 0, len(m.states))
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

// MemoryPrefsStore is an in-memory PrefsStore for tests.
type MemoryPrefsStore struct {
	mu    sync.Mutex
	prefs map[string]Prefs
}

func NewMemoryPrefsStore() *MemoryPrefsStore {
	return &MemoryPrefsStore{prefs: map[string]Prefs{}}
}

func (m *MemoryPrefsStore) GetPrefs(ctx context.Context, userID string) (*Prefs, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.prefs[userID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := p
	return &cp, nil
}

func (m *MemoryPrefsStore) PutPrefs(ctx context.Context, p Prefs) error {
	if p.UserID == "" {
		return fmt.Errorf("alert prefs require user_id")
	}
	if p.Threshold <= 0 {
		return fmt.Errorf("alert threshold must be greater than 0")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prefs[p.UserID] = p
	return nil
}

var (
	_ StateStore        = (*MemoryStateStore)(nil)
	_ PruningStateStore = (*MemoryStateStore)(nil)
	_ PrefsStore        = (*MemoryPrefsStore)(nil)
)
