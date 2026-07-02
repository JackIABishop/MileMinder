# Architecture Decision Records

This directory holds **ADRs** — short, numbered records of the load-bearing,
hard-to-reverse architectural decisions behind MileMinder, and *why* they were
made (including the alternatives that were rejected).

## Why these exist

An ADR captures the **reasoning** that the code and the issue tracker don't:
why opaque sessions and not JWT, why keep the storage YAML-backed before
Postgres, why local-first. That "why we chose X and explicitly not Y" is the
thing future-you (or a contributor) most wants when revisiting a decision, and
it's the first thing lost once the discussion that produced it is gone.

## How ADRs fit with the other records

Each record has **one job** — this is what stops them drifting apart:

| Record | Job |
|---|---|
| `ROADMAP.md` | Direction — phases, principles, product/monetisation strategy |
| GitHub issues (e.g. #15, #30) | Actionable design + acceptance criteria for a piece of work |
| **ADRs (here)** | The *decision* + rationale + rejected alternatives, at a point in time |
| The code + PRs | The implementation truth |

## The one rule: ADRs are immutable

An ADR is a **point-in-time** record. When a decision changes, you **write a new
ADR that supersedes the old one** — you do *not* edit the old one to match
current reality. Mark the superseded ADR's status and link forward. This
immutability is exactly what keeps the log honest instead of drifting.

## Convention

- Filename: `NNNN-short-kebab-title.md` (zero-padded, sequential).
- Status: `Proposed` → `Accepted` → (later) `Deprecated` / `Superseded by ADR-NNNN`.
- Keep them short (≈1 page). Only for decisions that are load-bearing and costly
  to reverse — not every feature. Quick-win features are captured by their issues.
- Add a line to the index below when you add one.

## Index

- [ADR-0001](0001-local-first-offline-architecture.md) — Local-first, offline architecture
- [ADR-0002](0002-single-source-of-truth-calculation-engine.md) — Single source of truth for the calculation engine
- [ADR-0003](0003-storage-abstraction-repository-pattern.md) — Storage abstraction via the repository pattern
- [ADR-0004](0004-authentication-and-session-model.md) — Authentication & session model
