# MileMinder Roadmap & Ideas

Living document. Captures where MileMinder is heading, the rearchitecture that
direction requires, and a prioritised backlog of feature ideas. Nothing here is
committed scope — it's a thinking surface. Edit freely.

## Vision

Today MileMinder is a **local, single-user** Go CLI + embedded SvelteKit web
dashboard, storing YAML files in `~/.mileminder/`.

The intended direction is a **local-first, multi-user product** with an optional
hosted layer. **Guiding principle: the app works fully offline — the device is the
source of truth, and you never need a connection to log a reading or see your
status.** (You use this standing next to the car, sometimes with no signal — an
app that needed the network to save an odometer number would be unusable there.)
A native **iPhone app** holds its own on-device store; a shared **MileMinder API**
+ web dashboard are an *optional* sync, backup, and web-access layer — not required
for the app to work. Alerts are delivered through a real channel (email first,
push later) so users find out *before* they blow their allowance.

Two consequences of offline-first, decided early because they shape the build:
- **Calculation runs on-device.** The same `internal/calc` engine is reused on iOS
  (via `gomobile`), never reimplemented in Swift — so there stays one source of
  truth for the math (server, CLI, and app), not the three-way divergence Phase 0
  killed. Keeping `internal/calc` pure and dependency-free is what makes this work.
- **Sync is deliberately simple.** The data is tiny and append-mostly (readings
  keyed by date), so **last-write-wins per record** is sufficient — no CRDTs, no
  heavy sync framework.

The self-hosted single-user mode (binary with embedded SPA) survives as a
deployment tier; the CLI is already local-first by nature.

## Why rearchitecture is required

Three things in the current code specifically block the hosted/multi-client goal:

1. **Single-user data store.** `~/.mileminder/<id>.yml` + a `current` pointer
   assumes one user, one filesystem. No concept of an owner. Needs a user
   identity, per-user isolation, and a real datastore.
2. **Duplicated calculation, API not the source of truth.** `cmd/` carries its
   own copies of the status math; `computeStatus()` lives in
   `internal/api/handlers.go`. With a web SPA, the CLI, *and* an on-device iOS app
   (offline use means the math can't be server-only) all needing it, the
   calculation must live in one pure package reused everywhere — the server, and
   the iOS app via `gomobile`.
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
| **2** | Identity + auth (email+password, opaque sessions) + per-user scoping. Introduce `/api/v1`. **Implemented** — see [Hosted mode](docs/hosted-mode.md). | Medium | **Auth gates sync/web only — the app works offline with no account; you sign in solely to enable sync, backup, and web access. No forced signup.** Defines the contract the iOS client syncs against. Interim file-backed user/session stores + per-user YAML dirs; Phase 3 swaps in Postgres behind the same interfaces. Password reset deferred to Phase 4 (#33, needs the email channel). |
| **3** | Server storage impl → managed **Postgres, multi-tenant**. This is the **sync/backup + web-read store**, *not* the app's primary store (the device is). Stateless server. | Medium | Repository interface from Phase 1 makes this localised. |
| **4** | Background scheduler + notification-channel abstraction. Periodic "recompute projection, fire if threshold crossed." Ship **email** first. | Medium | New infra: a job runner. Channel is abstracted so push slots in later. Runs server-side against synced data. |
| **5** | Native iOS client: **local on-device store**, `internal/calc` reused via **`gomobile`**, **last-write-wins sync** to the Postgres backend, APNs push. Fully usable offline; syncs when connected. | High | The local-first payoff. Own-store↔Postgres sync (not CloudKit) so web + iOS share one dataset. |

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

**Data & hosting — local-first.** The app is **offline-first: the device is the
source of truth** and needs no connection to work. The backend is an *optional*
sync / backup / web-access layer. On the server, data lives in a single **managed
Postgres** (Neon / Supabase / Fly) with **row-level multi-tenancy** (a `user_id`
on every row) — the sync target and what the web dashboard reads. Data is tiny, so
one small instance holds thousands of users and free tiers cover early growth at
~£0; host the single Go binary (embedded SPA) on Fly.io/Render alongside it. On
iOS the store is local (SQLite / SwiftData) with the shared `internal/calc` engine
via `gomobile`; sync is last-write-wins per record. The Phase 1 storage interface
keeps the server datastore swappable (SQLite/Turso later) — a low-regret bet.

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
- **Auth model:** *decided (Phase 2)* — email+password with opaque, server-side
  session tokens (HttpOnly cookie for the web SPA; `Authorization: Bearer` for
  future native/CLI clients). Sign in with Apple is deferred to Phase 5 alongside
  the iOS client. Auth gates *sync/web only* — never required for offline app use.
- **Sync approach:** *direction set* — own local-store ↔ Postgres sync (so web +
  iOS share one dataset), **not** CloudKit (iOS-only). Last-write-wins per record;
  tiny date-keyed data keeps conflicts rare and resolution trivial.
- **On-device calc:** *direction set* — reuse Go `internal/calc` on iOS via
  `gomobile`, **not** a Swift reimplementation (which would recreate the cmd/api
  divergence Phase 0 killed, three-way).
- **First notification channel:** email confirmed as Phase 4 start; push (APNs)
  follows with the iOS client.
- **Self-hosted tier:** *kept (Phase 2)* — `mileminder serve` remains the
  no-auth, single-user, embedded-SPA binary and is a first-class mode. Hosted
  multi-user is opt-in via `serve --hosted` (see [docs/hosted-mode.md](docs/hosted-mode.md)).
  Same binary, two modes; the single-user path is unchanged behaviour.
- **Password reset:** deferred to Phase 4 (#33) — self-service reset needs the
  email channel. Until then, hosted operators recover a locked-out user manually
  (documented in [docs/hosted-mode.md](docs/hosted-mode.md)).
