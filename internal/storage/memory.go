package storage

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/jackiabishop/mileminder/internal/model"
)

// Memory is an in-memory storage.Store for fast, filesystem-free tests of the
// api and cmd layers. It mirrors yamlstore's observable semantics (ErrNotFound
// behaviour, "" for an unset current pointer) but keeps everything in maps.
type Memory struct {
	mu       sync.RWMutex
	vehicles map[string]*model.VehicleData
	current  string
}

// NewMemory returns an empty in-memory Store.
func NewMemory() *Memory {
	return &Memory{vehicles: map[string]*model.VehicleData{}}
}

// clone deep-copies a vehicle so callers cannot mutate stored state through a
// returned pointer (the filesystem store hands back fresh parses each time).
func clone(data *model.VehicleData) *model.VehicleData {
	cp := *data
	cp.Readings = make(map[string]int, len(data.Readings))
	for k, v := range data.Readings {
		cp.Readings[k] = v
	}
	return &cp
}

func (m *Memory) ListVehicles(ctx context.Context) ([]Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.vehicles))
	for id := range m.vehicles {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	records := make([]Record, 0, len(ids))
	for _, id := range ids {
		records = append(records, Record{ID: id, Data: clone(m.vehicles[id])})
	}
	return records, nil
}

func (m *Memory) GetVehicle(ctx context.Context, id string) (*model.VehicleData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.vehicles[id]
	if !ok {
		return nil, fmt.Errorf("load vehicle %q: %w", id, ErrNotFound)
	}
	return clone(data), nil
}

func (m *Memory) SaveVehicle(ctx context.Context, id string, data *model.VehicleData) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.vehicles[id] = clone(data)
	return nil
}

func (m *Memory) DeleteVehicle(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.vehicles[id]; !ok {
		return fmt.Errorf("delete vehicle %q: %w", id, ErrNotFound)
	}
	delete(m.vehicles, id)
	return nil
}

func (m *Memory) PutReading(ctx context.Context, id, date string, miles int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.vehicles[id]
	if !ok {
		return fmt.Errorf("put reading on %q: %w", id, ErrNotFound)
	}
	if data.Readings == nil {
		data.Readings = map[string]int{}
	}
	data.Readings[date] = miles
	return nil
}

func (m *Memory) DeleteReading(ctx context.Context, id, date string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.vehicles[id]
	if !ok {
		return fmt.Errorf("delete reading on %q: %w", id, ErrNotFound)
	}
	if _, ok := data.Readings[date]; !ok {
		return fmt.Errorf("delete reading %q on %q: %w", date, id, ErrNotFound)
	}
	delete(data.Readings, date)
	return nil
}

func (m *Memory) GetCurrent(ctx context.Context) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current, nil
}

func (m *Memory) SetCurrent(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.vehicles[id]; !ok {
		return fmt.Errorf("set current %q: %w", id, ErrNotFound)
	}
	m.current = id
	return nil
}

// Compile-time assertion that Memory satisfies Store.
var _ Store = (*Memory)(nil)
