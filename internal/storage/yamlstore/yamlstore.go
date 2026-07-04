// Package yamlstore implements storage.Store over the historical on-disk layout:
// one YAML file per vehicle (<id>.yml) plus a plain-text "current" file holding
// the default vehicle id, all under a single directory (~/.mileminder by
// default). It is the local, single-user backend; Phase 3 adds a sibling SQL
// implementation of the same interface.
//
// # Concurrency
//
// All access is guarded by an in-process sync.RWMutex, so concurrent HTTP
// requests (or a CLI command running while the server is up in the same process)
// cannot interleave a read-modify-write and lose an update. Writes go to a temp
// file in the target directory and are os.Rename'd into place, so a reader never
// observes a torn/partial file and a crash mid-encode cannot truncate an existing
// vehicle.
//
// Accepted risk: this does not guard against a separate CLI *process* writing
// while the server process writes (no cross-process flock). The exposure is a
// single user racing themselves across two processes; Phase 3's database is the
// real fix. flock is platform-fiddly and deliberately out of scope here.
package yamlstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jackiabishop/mileminder/internal/atomicfile"
	"github.com/jackiabishop/mileminder/internal/model"
	"github.com/jackiabishop/mileminder/internal/storage"
	"gopkg.in/yaml.v3"
)

// currentFile is the name of the plain-text file holding the default vehicle id.
const currentFile = "current"

// settingsFile holds the user-level preferences as YAML. Deliberately no .yml
// extension (matching the "current" pointer): ListVehicles treats every *.yml
// file in the directory as a vehicle document, so an extensionless name keeps
// settings out of the vehicle namespace with no reserved-id special case.
const settingsFile = "settings"

// Store is a storage.Store backed by per-vehicle YAML files in a directory.
type Store struct {
	dir string
	mu  sync.RWMutex
}

// New returns a Store rooted at dir. The directory is created lazily on first
// write.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// DefaultDir returns the historical store location, ~/.mileminder.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".mileminder"), nil
}

// vehiclePath returns the YAML path for a vehicle id.
func (s *Store) vehiclePath(id string) string {
	return filepath.Join(s.dir, id+".yml")
}

// readVehicle loads and parses one vehicle file. It maps a missing file to
// storage.ErrNotFound. Callers hold the appropriate lock.
func (s *Store) readVehicle(id string) (*model.VehicleData, error) {
	raw, err := os.ReadFile(s.vehiclePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("load vehicle %q: %w", id, storage.ErrNotFound)
		}
		return nil, fmt.Errorf("read vehicle %q: %w", id, err)
	}
	var data model.VehicleData
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse vehicle %q: %w", id, err)
	}
	return &data, nil
}

// writeVehicle atomically encodes data to the vehicle file, creating the store
// directory if needed. Callers hold the write lock.
func (s *Store) writeVehicle(id string, data *model.VehicleData) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	// 0644 matches the perms the previous os.Create/os.WriteFile paths produced.
	return atomicfile.Write(s.vehiclePath(id), 0644, func(f *os.File) error {
		enc := yaml.NewEncoder(f)
		if err := enc.Encode(data); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	})
}

// ListVehicles returns all vehicles, skipping non-.yml entries and any file that
// cannot be read or parsed.
func (s *Store) ListVehicles(ctx context.Context) ([]storage.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read store dir: %w", err)
	}

	var records []storage.Record
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yml")
		data, err := s.readVehicle(id)
		if err != nil {
			// Skip unreadable/unparseable vehicles rather than failing the list.
			continue
		}
		records = append(records, storage.Record{ID: id, Data: data})
	}
	return records, nil
}

// GetVehicle returns one vehicle, or storage.ErrNotFound.
func (s *Store) GetVehicle(ctx context.Context, id string) (*model.VehicleData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readVehicle(id)
}

// SaveVehicle upserts a whole vehicle document.
func (s *Store) SaveVehicle(ctx context.Context, id string, data *model.VehicleData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeVehicle(id, data)
}

// DeleteVehicle removes a vehicle, or returns storage.ErrNotFound.
func (s *Store) DeleteVehicle(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.vehiclePath(id)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("delete vehicle %q: %w", id, storage.ErrNotFound)
		}
		return fmt.Errorf("delete vehicle %q: %w", id, err)
	}
	return nil
}

// PutReading upserts one reading under the write lock, mapping a missing vehicle
// to storage.ErrNotFound.
func (s *Store) PutReading(ctx context.Context, id, date string, miles int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readVehicle(id)
	if err != nil {
		return err
	}
	if data.Readings == nil {
		data.Readings = map[string]int{}
	}
	data.Readings[date] = miles
	return s.writeVehicle(id, data)
}

// DeleteReading removes one reading, mapping a missing vehicle or missing reading
// to storage.ErrNotFound.
func (s *Store) DeleteReading(ctx context.Context, id, date string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readVehicle(id)
	if err != nil {
		return err
	}
	if _, ok := data.Readings[date]; !ok {
		return fmt.Errorf("delete reading %q on %q: %w", date, id, storage.ErrNotFound)
	}
	delete(data.Readings, date)
	return s.writeVehicle(id, data)
}

// GetCurrent returns the default vehicle id, or "" when unset.
func (s *Store) GetCurrent(ctx context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw, err := os.ReadFile(filepath.Join(s.dir, currentFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read current pointer: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// SetCurrent sets the default vehicle id, returning storage.ErrNotFound if that
// vehicle does not exist.
func (s *Store) SetCurrent(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.vehiclePath(id)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("set current %q: %w", id, storage.ErrNotFound)
		}
		return fmt.Errorf("stat vehicle %q: %w", id, err)
	}
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	path := filepath.Join(s.dir, currentFile)
	if err := atomicfile.Write(path, 0644, func(f *os.File) error {
		_, err := f.WriteString(id)
		return err
	}); err != nil {
		return fmt.Errorf("write current pointer: %w", err)
	}
	return nil
}

// GetSettings returns the saved preferences, defaults when the file is absent,
// and backfills any empty field from the defaults.
func (s *Store) GetSettings(ctx context.Context) (*model.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	defaults := model.DefaultSettings()
	raw, err := os.ReadFile(filepath.Join(s.dir, settingsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return &defaults, nil
		}
		return nil, fmt.Errorf("read settings: %w", err)
	}
	var settings model.Settings
	if err := yaml.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}
	if settings.Currency == "" {
		settings.Currency = defaults.Currency
	}
	if settings.DistanceUnit == "" {
		settings.DistanceUnit = defaults.DistanceUnit
	}
	return &settings, nil
}

// SaveSettings atomically replaces the preferences document.
func (s *Store) SaveSettings(ctx context.Context, settings *model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	path := filepath.Join(s.dir, settingsFile)
	if err := atomicfile.Write(path, 0644, func(f *os.File) error {
		enc := yaml.NewEncoder(f)
		if err := enc.Encode(settings); err != nil {
			enc.Close()
			return err
		}
		return enc.Close()
	}); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

// Compile-time assertion that Store satisfies storage.Store.
var _ storage.Store = (*Store)(nil)
