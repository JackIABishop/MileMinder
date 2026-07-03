# ADR-0005: Optional allowance plan

**Status:** Accepted
**Date:** 2026-07-03
**Deciders:** Jack Bishop

## Context

MileMinder started as an allowance tracker: every vehicle had a `Plan` with a
start date, end date, annual allowance, and start miles. That excludes vehicles
you simply own and want to track without a PCP, lease, or insurance mileage cap.

This decision lands before Phase 3 so the Postgres schema can model optional
allowance data directly instead of migrating to it later.

## Decision

- `model.VehicleData.Plan` is `*model.Plan`; `nil` means plain tracking.
- Existing YAML files with a `plan:` key still unmarshal to non-nil plans and
  save with the same policy shape.
- Status JSON adds `has_plan`; allowance-specific status fields remain flat and
  zero-valued for plain vehicles.
- Plain vehicles use the earliest odometer reading as their baseline. No
  top-level `tracking_start` or `start_miles` duplicate is added.
- Fleet allowance aggregates (`delta`, percent used, worst offender) consider
  policy vehicles only. Pace aggregates include every vehicle.

## Options Considered

| Option | Outcome |
|---|---|
| `*Plan` where nil means plain | Chosen: maps directly to absent YAML and SQL optionality with no second discriminator |
| `tracking_mode` enum | Rejected: can disagree with the data and adds a new YAML field |
| Separate plain vehicle type | Rejected: would infect storage, API, calc, and UI consumers with parallel shapes |
| Pointer status fields for N/A values | Rejected for now: churns the API contract; clients can gate on `has_plan` |

## Phase 3 Mapping

The pointer maps cleanly to either nullable plan columns on `vehicles` with an
all-null-or-all-set check, or to a zero-or-one `plans` child table. No mode enum
is needed in SQL.

## Consequences

- Policy vehicles keep the existing YAML and status behavior.
- Plain vehicles retain readings, graphs, daily pace, average annual mileage,
  and recent pace, but suppress allowance-specific UI and API interpretation.
- Plain-to-policy conversion is additive and supported. Policy-to-plain
  conversion is intentionally deferred because it discards allowance data and
  needs a confirmation/history decision.
