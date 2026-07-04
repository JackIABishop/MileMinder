// Package storage defines the persistence contract for MileMinder vehicles and
// isolates all data I/O behind a single interface. The calculation layer
// (internal/calc) stays pure; storage does the reading and writing.
//
// # One Store, one aggregate
//
// Vehicles, their readings, and the "current" default-vehicle pointer are one
// cohesive aggregate: readings live inside a vehicle document, and the pointer
// references a vehicle id. They are modelled as a single Store rather than
// separate repositories because there is one implementation and one consumer
// pattern; callers depend on the interface, so splitting later is mechanical.
//
// # Forward compatibility (design intent, not built here)
//
// Ownership (Phase 2 — hosted, multi-user): the interface is deliberately
// single-tenant today, a faithful description of the local YAML store. When
// per-user scoping lands, the authenticated middleware will resolve the user and
// obtain a *scoped* Store — either via a ForUser(userID) factory on the
// multi-tenant implementation, or by threading an owner argument at that point.
// Either way the change is confined to the Store implementations plus router
// middleware, because handlers and commands only ever see this interface. The
// "current" pointer is naturally per-user state and scopes the same way. No
// owner parameters are added now: an unused parameter through every call site for
// two phases is churn without payoff.
//
// SQL backend (Phase 3): every method maps directly onto SQL — ListVehicles to a
// SELECT, Get/Save/DeleteVehicle to vehicle-table CRUD, Put/DeleteReading to
// readings-table row operations, the pointer to a per-user column. ErrNotFound
// maps from sql.ErrNoRows. No method leans on filesystem semantics (no paths, no
// fs.FS in the interface), so a sibling internal/storage/sqlstore slots in
// without touching callers.
//
// # context.Context
//
// Every method takes a context.Context even though the YAML implementation
// ignores it. API handlers already have r.Context() and cobra commands have
// cmd.Context(); wiring it now means Phase 2 auth deadlines and Phase 3
// database/sql cancellation do not require a second whole-codebase touch.
package storage

import (
	"context"
	"errors"

	"github.com/jackiabishop/mileminder/internal/model"
)

// ErrNotFound is returned when a vehicle, or a reading within a vehicle, does not
// exist. Implementations wrap it with context (fmt.Errorf(...%w)); callers use
// errors.Is(err, ErrNotFound). A single sentinel is enough: "vehicle missing" and
// "reading missing" both map to a 404, so distinguishing them via separate
// sentinels buys nothing — the wrap message carries the detail.
var ErrNotFound = errors.New("not found")

// Record pairs a vehicle id (today: the YAML filename stem) with its data.
type Record struct {
	ID   string
	Data *model.VehicleData
}

// Store is the persistence contract for vehicles, their readings, and the
// default-vehicle pointer.
type Store interface {
	// ListVehicles returns every vehicle. Entries that cannot be read or parsed
	// are skipped rather than failing the whole call.
	//
	// It returns full vehicle documents because today's only listing consumers
	// (the API vehicle-list and fleet handlers, the CLI cars/fleet commands)
	// immediately load every vehicle anyway. This over-fetches for a plain id/name
	// listing under a future SQL backend; if that cost ever matters, a lightweight
	// summary variant (e.g. ListVehicleSummaries) can be added alongside this
	// method without touching existing callers.
	ListVehicles(ctx context.Context) ([]Record, error)

	// GetVehicle returns one vehicle by id, or ErrNotFound if it does not exist.
	GetVehicle(ctx context.Context, id string) (*model.VehicleData, error)

	// SaveVehicle upserts a whole vehicle document (create or full replace). It is
	// the write path for creating a plan and for plan-level updates. Because it
	// overwrites, callers that must not clobber an existing vehicle (e.g. create)
	// guard with a prior GetVehicle/ErrNotFound check.
	SaveVehicle(ctx context.Context, id string, data *model.VehicleData) error

	// DeleteVehicle removes a vehicle, or returns ErrNotFound if it does not exist.
	DeleteVehicle(ctx context.Context, id string) error

	// PutReading upserts a single odometer reading on a vehicle, returning
	// ErrNotFound if the vehicle does not exist. This reading-granular write (vs.
	// load-mutate-SaveVehicle at the call site) keeps the read-modify-write inside
	// the Store where locking lives, and maps onto an INSERT ... ON CONFLICT under
	// a future SQL backend.
	PutReading(ctx context.Context, id, date string, miles int) error

	// DeleteReading removes one reading by date, returning ErrNotFound if either
	// the vehicle or that reading does not exist.
	DeleteReading(ctx context.Context, id, date string) error

	// GetCurrent returns the default vehicle id, or "" with a nil error when no
	// default is set.
	GetCurrent(ctx context.Context) (string, error)

	// SetCurrent sets the default vehicle id, returning ErrNotFound if that vehicle
	// does not exist. The referential check lives in the Store.
	SetCurrent(ctx context.Context, id string) error

	// GetSettings returns the user-level preferences. When none have been saved
	// it returns model.DefaultSettings() with a nil error (mirroring GetCurrent's
	// ""-when-unset), and implementations backfill any empty field from the
	// defaults so a partial document from an older version stays valid. Settings
	// scope like the current pointer: per-user in hosted mode, global in
	// single-user mode.
	GetSettings(ctx context.Context) (*model.Settings, error)

	// SaveSettings replaces the user-level preferences document. Validation
	// (e.g. supported currencies) is the caller's job; the Store only persists.
	SaveSettings(ctx context.Context, s *model.Settings) error
}
