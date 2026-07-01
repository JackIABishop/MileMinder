# MileMinder Roadmap & Ideas

Living document. Captures where MileMinder is heading, the rearchitecture that
direction requires, and a prioritised backlog of feature ideas. Nothing here is
committed scope — it's a thinking surface. Edit freely.

## Vision

Today MileMinder is a **local, single-user** Go CLI + embedded SvelteKit web
dashboard, storing YAML files in `~/.mileminder/`.

The intended direction is a **hosted, multi-user product**: people log in, and a
native **iPhone app** plus the existing web UI both talk to a shared
**MileMinder API**. Alerts are delivered through a real channel (email first,
push later) so users find out *before* they blow their allowance.

The self-hosted single-user mode (binary with embedded SPA) should survive as a
deployment tier, not be thrown away.

## Why rearchitecture is required

Three things in the current code specifically block the hosted/multi-client goal:

1. **Single-user data store.** `~/.mileminder/<id>.yml` + a `current` pointer
   assumes one user, one filesystem. No concept of an owner. Needs a user
   identity, per-user isolation, and a real datastore.
2. **Duplicated calculation, API not the source of truth.** `cmd/` carries its
   own copies of the status math; `computeStatus()` lives in
   `internal/api/handlers.go`. With a web SPA *and* an iOS app as clients, the
   calculation must live in one package the API owns.
3. **No auth, no API versioning, no contract stability.** Open CORS, unversioned
   `/api/vehicles/...`, embedded-SPA-on-localhost. A mobile client needs a
   versioned, stable, authenticated contract.

What stays: Go 1.22 `ServeMux` routing, the JSON struct shapes (already mirror
the TS client), the SvelteKit SPA (becomes one API client), and the embed mode
(kept as the self-hosted tier).

## Rearchitecture phases

| Phase | Goal | Risk | Notes |
|---|---|---|---|
| **0** | Extract `internal/calc` (or `internal/core`) as the single source of truth for `computeStatus`/`odometerAt`/projection. Both `cmd/` and `internal/api/` call it. | Low | Highest-leverage move; valuable even if hosting never ships. Do first. |
| **1** | Storage interface (repository pattern) behind today's YAML impl. Handlers stop calling `loadVehicle`/`saveVehicle` directly. | Low | No behaviour change. Unblocks swapping the backend. |
| **2** | Identity + auth (token/session; password or OAuth / Sign in with Apple) + per-user scoping. Introduce `/api/v1`. | Medium | Defines the public contract the iOS app will depend on. |
| **3** | Swap storage impl to Postgres or SQLite-per-user; stateless server for horizontal scale. | Medium | Repository interface from Phase 1 makes this localised. |
| **4** | Background scheduler + notification-channel abstraction. Periodic "recompute projection, fire if threshold crossed." Ship **email** first. | Medium | New infra: a job runner. Channel is abstracted so push slots in later. |
| **5** | Native iOS client against the public API. Add **APNs push** as a notification channel. | High | Web + iOS are peer clients of the same API. |

## Feature backlog

Tagged by fit against the *current* design. "Clean fit" = extends
`computeStatus` or adds one endpoint, no architectural change. Many of the bigger
items are unlocked by the phases above.

### Do first — high value, low effort, clean fit
- **Renewal countdown** — days to `plan.end` + final-mileage estimate. `computeStatus` extension.
- **Drivable-rate budget** — "you can drive X mi/day for the rest of the plan and stay legal." Inverse of the projection math.
- **Overage cost estimate** — store an `excess_rate` (pence/excess mile) on `Plan`; project the £ penalty. One model field.
- **Fleet roll-up insights** — household over/under, worst-offending car, comparative pace. Enriches existing `HandleFleet`.
- **Trend signal** — surface `recent_annual_mileage` vs `avg_annual_mileage` delta (accelerating / easing off). Already computed.

### High value, moderate effort
- **CSV / manual import** — bulk-add historical readings (export already exists). New endpoint + CLI flag.
- **Scenario / what-if** — "if I take a 600-mi trip next month, where do I land?" Read-only projection overlay.
- **PWA + mobile quick-add** — installable web app, one-tap "add today's reading." Manifest + service worker + fast-add UI. (Interim before the native app.)
- **Pace-breach alerts (CLI-first)** — `mileminder check` with a cron-friendly exit code, before any delivery channel exists. Becomes Phase 4 once a channel lands.
- **Backup / snapshot** — export all of `~/.mileminder/` as an archive. CLI-only.

### Worthwhile but touches the data model
- **Reading metadata** — optional note/source/tag per reading ("MOT", "service", "manual"). `Readings` value goes `int → struct`; touches *both* persistence copies (`cmd/` + `internal/api/`), API, and web. Cheaper to do after Phase 1.
- **Per-car identity** — colour / label / icon in the UI. Small, but needs a prefs field on the vehicle.

### Big swings — gated on the rearchitecture
- **Auth + multi-user / hosted mode** — Phase 2/3. The fork that turns this from a local tool into a service.
- **Read-only share link** — a vehicle's status without the editing UI (insurance/handover). Needs the auth model from Phase 2.
- **Shareable status report (PDF / static HTML)** — printable summary ("avg annual mileage: X"). Moderate; new render path.
- **Alerts delivery (email → push)** — Phase 4 (email) then Phase 5 (APNs).
- **Automated odometer capture** — OCR a dashboard photo, or pull from a connected-car API (e.g. Smartcar). High value, high effort, external dependency.

## Monetisation & data strategy

Direction — not final scope, but it anchors the Phase 2/3 decisions.

**Positioning.** MileMinder targets a specific gap — PCP/lease *allowance* breach —
that generic fuel/mileage-log apps don't serve. The value story is direct: the app
costs less than a single over-mileage penalty charge.

**Data & hosting.** The hosted path is chosen (web dashboard *and* sharing both
need a backend). Datastore: a single **managed Postgres** (Neon / Supabase / Fly)
with **row-level multi-tenancy** (a `user_id` on every row). The data is tiny, so
one small instance holds thousands of users and free tiers cover early growth at
~£0. Host the single Go binary (embedded SPA) on Fly.io/Render alongside it. The
Phase 1 storage interface keeps this swappable (SQLite/Turso later) — a low-regret
bet. A local-first-on-device model with the backend as an optional sync layer is
the zero-cost-per-user ideal, but adds sync/conflict complexity — kept as a later
option, not built first.

**Pricing — match the payment type to the cost type.** Freemium:
- **Free:** core tracking (readings, graph, single-car status). ~Zero ongoing cost;
  drives downloads and word-of-mouth in the niche.
- **One-time "Pro" unlock (~£8–12 IAP):** the analysis features (multi-car, fleet
  insights, projections, overage cost, alert config). These compute on-device, so a
  one-time price is honest and safe. Qualifies for Apple's Small Business Program (15%).
- **Small optional subscription (~£10–15/yr), later:** *only* the genuinely
  recurring-cost features — cloud sync across devices, web dashboard access, push alerts.

The rule underneath it: **never put a one-time price on a feature that costs money
forever.** One-time for the on-device brains; subscription only for the always-on
plumbing. Ship free + one-time Pro first; add the subscription tier only if the
hosted features prove wanted. No billing infrastructure before Phase 5.

## Open decisions

- **Datastore for hosted mode:** *direction set* — managed Postgres with row-level
  multi-tenancy (see Monetisation & data strategy). Kept swappable via the Phase 1
  storage interface; SQLite/Turso remains a fallback. Confirm at Phase 3.
- **Auth model:** email+password vs OAuth / Sign in with Apple (best for the iOS
  story) vs both.
- **First notification channel:** email confirmed as Phase 4 start; push (APNs)
  follows with the iOS client.
- **Self-hosted tier:** keep the embedded-SPA single-user binary as a supported
  deployment, or eventually drop it?
