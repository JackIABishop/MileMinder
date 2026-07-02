# ADR-0002: Single source of truth for the calculation engine

**Status:** Accepted
**Date:** 2026-07-02
**Deciders:** Jack Bishop

## Context

The status/projection/pace math (miles-used vs the allowance line, `daily_rate`,
projections) originally lived as **duplicated copies**: `computeStatus()` in
`internal/api/handlers.go` *and* independent inline copies across several `cmd/`
files. The copies had **silently drifted with real bugs** (a `start_miles`-off
delta in `status`, an inverted delta sign in `fleet`). With a web SPA, the CLI,
and a future on-device iOS app all needing this math, duplication is untenable.

## Decision

One **pure** `internal/calc` package is the single source of truth. Both
`internal/api/` and `cmd/` delegate to it. `internal/calc` does **no I/O** and
has no dependency on the HTTP or storage layers. On iOS it is reused via
`gomobile`, **never** reimplemented in Swift.

## Options Considered

### A. Keep per-layer copies
**Rejected.** Already proven to diverge and produce bugs. Cost of a third
(Swift) copy would be worse.

### B. Single pure `internal/calc` package — **chosen**
One place to change, one place to test. Purity (no I/O) is what makes it reusable
by the CLI, the API, and a `gomobile` iOS binary alike.

### C. Server-only calc (clients call the API to compute)
**Rejected.** Breaks offline use (ADR-0001) — the phone couldn't compute status
without a connection.

## Consequences

- The math is defined and tested in exactly one place; the year-boundary
  `daily_rate` bug and the CLI divergences were fixed *as part of* the extraction.
- Offline on-device calculation becomes possible precisely because the package is
  pure — this is a hard constraint to protect going forward (no I/O creep into
  `internal/calc`).
- Enables the storage abstraction (ADR-0003): calc and persistence are cleanly
  separable because calc never touches storage.

**Refs:** Phase 0, issue #22, PR #23.
