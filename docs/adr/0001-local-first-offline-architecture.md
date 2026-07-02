# ADR-0001: Local-first, offline architecture

**Status:** Accepted
**Date:** 2026-07-02
**Deciders:** Jack Bishop

## Context

MileMinder is used *standing next to a car* — in a garage, an underground car
park, a rural driveway — often with no signal. It is heading toward a native
iPhone app plus a web dashboard. The foundational question: does the app require
connectivity to work?

## Decision

**Local-first.** The **device is the source of truth**; the app works fully
offline and never needs a connection to log a reading or see status. The backend
API + web dashboard are an **optional** sync / backup / web-access layer, not
required for the app to function. **Authentication gates sync/web only — it is
never required for offline use (no forced signup).**

## Options Considered

### A. Hosted / thin-client (server is the source of truth)
**Rejected.** Every action (even logging one odometer number) needs a network
round-trip — unusable in the exact place the app is used. Also incurs per-user
recurring cost, which fights a one-time purchase model.

### B. Local-first with optional sync — **chosen**
Device owns the data; sync is background and optional. Keeps the app instant and
offline, while still enabling web + cross-device for those who opt in.

### C. Local-only, no backend ever
**Rejected.** Precludes the web dashboard and sharing, both of which are wanted
(see `#24`, and the sharing goals in ROADMAP).

## Consequences

- **The calculation must run on-device** → the single pure calc engine (ADR-0002)
  is reused on iOS via `gomobile`, not reimplemented in Swift.
- **A sync engine is needed** (Phase 5): last-write-wins per record — the data is
  tiny and append-mostly (readings keyed by date), so no CRDTs.
- **Auth is optional** → the server runs in two modes (single-user default /
  `--hosted`), see ADR-0004.
- **Monetisation aligns cleanly**: one-time purchase for on-device features
  (zero ongoing cost), subscription only for the always-on plumbing (sync/web).
- Own-store↔Postgres sync is chosen over CloudKit so web + iOS share one dataset.

**Refs:** `ROADMAP.md` (Vision), epic #20.
