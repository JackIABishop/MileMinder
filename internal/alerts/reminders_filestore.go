package alerts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/jackiabishop/mileminder/internal/atomicfile"
	"gopkg.in/yaml.v3"
)

// FileReminderSettingsStore persists reminder settings in
// <root>/reminder_settings.yml.
type FileReminderSettingsStore struct {
	path string
	mu   sync.Mutex
}

func NewFileReminderSettingsStore(root string) *FileReminderSettingsStore {
	return &FileReminderSettingsStore{path: filepath.Join(root, "reminder_settings.yml")}
}

type reminderSettingsDoc struct {
	Reminders []ReminderSettings `yaml:"reminders"`
}

func (s *FileReminderSettingsStore) load() ([]ReminderSettings, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reminder settings file: %w", err)
	}
	var doc reminderSettingsDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse reminder settings file: %w", err)
	}
	return doc.Reminders, nil
}

func (s *FileReminderSettingsStore) save(reminders []ReminderSettings) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	sort.Slice(reminders, func(i, j int) bool {
		if reminders[i].UserID == reminders[j].UserID {
			return reminders[i].VehicleID < reminders[j].VehicleID
		}
		return reminders[i].UserID < reminders[j].UserID
	})
	return atomicfile.Write(s.path, 0600, func(f *os.File) error {
		enc := yaml.NewEncoder(f)
		if err := enc.Encode(reminderSettingsDoc{Reminders: reminders}); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	})
}

func (s *FileReminderSettingsStore) GetReminder(ctx context.Context, userID, vehicleID string) (*ReminderSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reminders, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, r := range reminders {
		if r.UserID == userID && r.VehicleID == vehicleID {
			cp := r
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (s *FileReminderSettingsStore) PutReminder(ctx context.Context, r ReminderSettings) error {
	if r.UserID == "" || r.VehicleID == "" {
		return fmt.Errorf("reminder settings require user_id and vehicle_id")
	}
	if err := r.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reminders, err := s.load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range reminders {
		if reminders[i].UserID == r.UserID && reminders[i].VehicleID == r.VehicleID {
			reminders[i] = r
			replaced = true
			break
		}
	}
	if !replaced {
		reminders = append(reminders, r)
	}
	return s.save(reminders)
}

func (s *FileReminderSettingsStore) PruneUserReminders(ctx context.Context, userID string, keepVehicleIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	reminders, err := s.load()
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, id := range keepVehicleIDs {
		keep[id] = true
	}
	out := reminders[:0]
	for _, r := range reminders {
		if r.UserID == userID && !keep[r.VehicleID] {
			continue
		}
		out = append(out, r)
	}
	return s.save(out)
}

// FileReminderStateStore persists reminder send state in
// <root>/reminder_state.yml.
type FileReminderStateStore struct {
	path string
	mu   sync.Mutex
}

func NewFileReminderStateStore(root string) *FileReminderStateStore {
	return &FileReminderStateStore{path: filepath.Join(root, "reminder_state.yml")}
}

type reminderStateDoc struct {
	States []VehicleReminderState `yaml:"states"`
}

func (s *FileReminderStateStore) load() ([]VehicleReminderState, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reminder state file: %w", err)
	}
	var doc reminderStateDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse reminder state file: %w", err)
	}
	return doc.States, nil
}

func (s *FileReminderStateStore) save(states []VehicleReminderState) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	sort.Slice(states, func(i, j int) bool {
		if states[i].UserID == states[j].UserID {
			return states[i].VehicleID < states[j].VehicleID
		}
		return states[i].UserID < states[j].UserID
	})
	return atomicfile.Write(s.path, 0600, func(f *os.File) error {
		enc := yaml.NewEncoder(f)
		if err := enc.Encode(reminderStateDoc{States: states}); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	})
}

func (s *FileReminderStateStore) GetReminderState(ctx context.Context, userID, vehicleID string) (*VehicleReminderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, st := range states {
		if st.UserID == userID && st.VehicleID == vehicleID {
			cp := st
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (s *FileReminderStateStore) PutReminderState(ctx context.Context, st VehicleReminderState) error {
	if st.UserID == "" || st.VehicleID == "" {
		return fmt.Errorf("reminder state requires user_id and vehicle_id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range states {
		if states[i].UserID == st.UserID && states[i].VehicleID == st.VehicleID {
			states[i] = st
			replaced = true
			break
		}
	}
	if !replaced {
		states = append(states, st)
	}
	return s.save(states)
}

func (s *FileReminderStateStore) PruneUserReminderStates(ctx context.Context, userID string, keepVehicleIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, id := range keepVehicleIDs {
		keep[id] = true
	}
	out := states[:0]
	for _, st := range states {
		if st.UserID == userID && !keep[st.VehicleID] {
			continue
		}
		out = append(out, st)
	}
	return s.save(out)
}

var (
	_ ReminderSettingsStore = (*FileReminderSettingsStore)(nil)
	_ ReminderStateStore    = (*FileReminderStateStore)(nil)
)
