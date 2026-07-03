# ADR-0006: Notification channel and alert scheduler

**Status:** Accepted
**Date:** 2026-07-03
**Deciders:** Jack Bishop

## Context

Hosted mode can hold multiple users' vehicle data, but allowance breaches are
only visible when a user opens the dashboard or runs the local CLI. MileMinder
needs a server-side loop that detects new allowance crossings and notifies the
account email, without replacing the offline `mileminder check` mechanism.

The calculation package remains pure: breach evaluation is math over
`calc.Status`. Scheduling, delivery, preferences, and persisted deduplication are
hosted infrastructure.

## Decision

- Add a channel-neutral `notify.Channel` with `Message` and `Recipient`.
- Use `notify.LogChannel` by default when SMTP is not configured; use a stdlib
  SMTP implementation when `MILEMINDER_SMTP_*` env vars are set.
- Run an in-process alert scheduler goroutine only in `serve --hosted`.
- Keep the scheduler testable through `RunOnce`, injected stores, and an
  injected clock.
- Persist alert preferences and edge state in hosted-root YAML files:
  `alert_prefs.yml` and `alerts_state.yml`.
- Send only on OK-to-breached crossings. First observation seeds state silently
  so enabling alerts on existing hosted data does not cause a burst.
- Put the reusable breach predicate in `internal/calc` so CLI and hosted alerts
  agree.

## Options Considered

| Option | Outcome |
|---|---|
| In-process hosted goroutine | Chosen: no new deployment unit; extraction later is wiring around `RunOnce` |
| Separate daemon / cron job | Rejected for now: more infrastructure than a single-instance hosted deployment needs |
| SMTP now | Chosen: password reset needs the same channel next, and log fallback keeps local setup zero-config |
| Stub channel only | Rejected: would defer deployability and SMTP risk |
| Re-alert while still breached | Rejected for v1: noisier default; can be added as prefs over the same state |

## Consequences

- Single-user mode and the offline CLI stay unchanged.
- Existing already-breached hosted vehicles are not emailed until they clear and
  breach again.
- SMTP secrets live only in environment variables, never in files or flags.
- APNs can be added as a sibling `notify.Channel` implementation later without
  changing alert rendering or scheduler state.
