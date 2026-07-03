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

// FileStateStore persists alert state in <root>/alerts_state.yml.
type FileStateStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStateStore(root string) *FileStateStore {
	return &FileStateStore{path: filepath.Join(root, "alerts_state.yml")}
}

type stateDoc struct {
	States []VehicleAlertState `yaml:"states"`
}

func (s *FileStateStore) load() ([]VehicleAlertState, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read alert state file: %w", err)
	}
	var doc stateDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse alert state file: %w", err)
	}
	return doc.States, nil
}

func (s *FileStateStore) save(states []VehicleAlertState) error {
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
		if err := enc.Encode(stateDoc{States: states}); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	})
}

func (s *FileStateStore) GetState(ctx context.Context, userID, vehicleID string) (*VehicleAlertState, error) {
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

func (s *FileStateStore) PutState(ctx context.Context, st VehicleAlertState) error {
	if st.UserID == "" || st.VehicleID == "" {
		return fmt.Errorf("alert state requires user_id and vehicle_id")
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

func (s *FileStateStore) PruneUserStates(ctx context.Context, userID string, keepVehicleIDs []string) error {
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

// FilePrefsStore persists alert preferences in <root>/alert_prefs.yml.
type FilePrefsStore struct {
	path string
	mu   sync.Mutex
}

func NewFilePrefsStore(root string) *FilePrefsStore {
	return &FilePrefsStore{path: filepath.Join(root, "alert_prefs.yml")}
}

type prefsDoc struct {
	Prefs []Prefs `yaml:"prefs"`
}

func (s *FilePrefsStore) load() ([]Prefs, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read alert prefs file: %w", err)
	}
	var doc prefsDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse alert prefs file: %w", err)
	}
	return doc.Prefs, nil
}

func (s *FilePrefsStore) save(prefs []Prefs) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	sort.Slice(prefs, func(i, j int) bool { return prefs[i].UserID < prefs[j].UserID })
	return atomicfile.Write(s.path, 0600, func(f *os.File) error {
		enc := yaml.NewEncoder(f)
		if err := enc.Encode(prefsDoc{Prefs: prefs}); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	})
}

func (s *FilePrefsStore) GetPrefs(ctx context.Context, userID string) (*Prefs, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefs, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, p := range prefs {
		if p.UserID == userID {
			cp := p
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (s *FilePrefsStore) PutPrefs(ctx context.Context, p Prefs) error {
	if p.UserID == "" {
		return fmt.Errorf("alert prefs require user_id")
	}
	if p.Threshold <= 0 {
		return fmt.Errorf("alert threshold must be greater than 0")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prefs, err := s.load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range prefs {
		if prefs[i].UserID == p.UserID {
			prefs[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		prefs = append(prefs, p)
	}
	return s.save(prefs)
}

var (
	_ StateStore        = (*FileStateStore)(nil)
	_ PruningStateStore = (*FileStateStore)(nil)
	_ PrefsStore        = (*FilePrefsStore)(nil)
)
