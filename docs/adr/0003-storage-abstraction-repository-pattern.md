# ADR-0003: Storage abstraction via the repository pattern

**Status:** Accepted
**Date:** 2026-07-02
**Deciders:** Jack Bishop

## Context

Persistence (`loadVehicle`/`saveVehicle`, the `current` pointer) was **duplicated**
between `internal/api/handlers.go` and inline across five `cmd/` files, each
doing its own `os`/`yaml` calls. MileMinder is heading toward multi-tenant hosting
on Postgres (ADR-0004, Phase 3) while keeping the local YAML store for the
single-user/CLI case. Handlers and commands must not be coupled to *how* data is
stored.

## Decision

Introduce `internal/storage.Store` — a single interface over vehicles, readings,
and the `current` pointer — with the YAML behaviour behind it (`yamlstore`).
`context.Context` on every method; a single `storage.ErrNotFound` sentinel;
reading-granular `PutReading`/`DeleteReading` alongside document-level
`SaveVehicle`. Per-user scoping is added later via a `Tenants{ ForUser(id) Store }`
seam (ADR-0004) **without changing the `Store` interface**. `yamlstore` uses an
in-process mutex + atomic temp-file-and-rename writes; byte-identical on-disk YAML
is preserved.

## Options Considered

### A. Leave persistence inline / duplicated
**Rejected.** Duplication (the ADR-0002 lesson, now for I/O) and no way to swap
the backend for Postgres without rewriting every handler and command.

### B. Single `Store` interface — **chosen**
One implementation today (`yamlstore`), one consumer pattern; Postgres becomes a
sibling implementation in Phase 3 with zero handler/command churn. `context.Context`
now avoids a second whole-codebase touch when auth/DB arrive.

### C. Separate repositories per aggregate (Vehicle / Reading / Current)
**Rejected.** Ceremony with one implementation and one consumer; the three are a
cohesive aggregate (readings live inside a vehicle; `current` references a
vehicle). Trivially splittable later since callers depend on the interface.

## Consequences

- Backend is swappable: Phase 3 adds `sqlstore` as a sibling package.
- Per-user scoping (Phase 2/ADR-0004) threads through `Tenants.ForUser`, which
  reuses `yamlstore` per-user directories — **no `Store` changes**.
- Testable via an in-memory fake + a shared conformance suite run against both
  implementations.
- `ListVehicles` returns full documents today — a documented trade-off; a
  lightweight summary variant can be added for a SQL backend without touching
  callers.

**Refs:** Phase 1, issue #30, PR #32.
