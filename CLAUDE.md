# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

MileMinder is a CLI + web dashboard for tracking vehicle mileage, with an optional PCP/lease/insurance allowance policy. Module path: `github.com/jackiabishop/mileminder`.

## Commands

```bash
make build        # build web UI then Go binary -> ./mileminder
make build-web    # cd web && npm install && npm run build, then copy web/dist -> internal/web/dist
make build-go     # go build -o mileminder .
make install      # go mod download + npm install
make test         # go test ./...   (the suite lives in internal/calc)
make clean        # remove binary, web/dist, web/node_modules, web/.svelte-kit, internal/web/dist/*
```

Run the calc tests directly: `go test ./internal/calc`. Run a single test: `go test ./internal/calc -run TestName`.

### Running the app
```bash
./mileminder serve                 # production: serves embedded UI on :8080, opens browser
./mileminder serve --port 8081 --no-browser
make dev-api                       # go run . serve --dev  (API only, :8080)
make dev-web                       # cd web && npm run dev  (Vite frontend with hot reload)
```
Dev mode runs the Go API and the Vite frontend as **two separate processes**; production serves the prebuilt SPA embedded in the binary.

## Critical build coupling

The web UI is embedded into the Go binary via `go:embed all:dist` in `internal/web/embed.go`. **The embedded copy at `internal/web/dist/` is a separate, committed copy of `web/dist/`.** Editing Svelte source alone changes nothing the binary serves. After any `web/src` change you must:

```bash
cd web && npm run build
rm -rf internal/web/dist/* && cp -r web/dist/* internal/web/dist/   # (this is `make build-web`)
go build -o mileminder .
```

`web/dist/` is gitignored; `internal/web/dist/` is **tracked** because `go:embed` reads from source, so `go install` needs it present. Commit rebuilt embedded assets separately from source for reviewable diffs. The `mileminder` binary itself is gitignored — do not commit it.

## Architecture

The layers share one data model **and one calculation package** — `internal/calc` is the single source of truth for the status/projection/pace math; both `internal/api/` and `cmd/` delegate to it:

- **`internal/calc/`** — the canonical status/projection/pace calculator. `calc.ComputeStatus(...)` produces all status figures, `calc.OdometerAt(...)` is the shared interpolation primitive, and `calc.AllowanceMiles(...)` is the shared allowance-line primitive (`annual_allowance * daysElapsed / 365`). The `Status` result type lives here too; `api.VehicleStatus` is a type alias for it. **Do not reintroduce a private copy of this math anywhere** — both other layers must call into `internal/calc`.
- **`cmd/`** — Cobra CLI. Each command is its own file (`add`, `status`, `graph`, `fleet`, `cars`, `switch`, `init`, `reset`, `serve`). `serve` is the bridge to the web layer. CLI commands read/write YAML directly but delegate all status math to `internal/calc`.
- **`internal/api/`** — HTTP layer. `router.go` wires a stdlib `http.ServeMux` (Go 1.22 method+path patterns like `GET /api/v1/vehicles/{id}`) with an SPA fallback handler (unknown paths serve `index.html` for client-side routing). All routes are under **`/api/v1`**. `handlers.go` holds all endpoint logic and delegates status math to `calc.ComputeStatus` (the graph handler's ideal line uses `calc.AllowanceMiles`). Handlers are **store-agnostic**: they read a per-request `storage.Store` via `storeFrom(r.Context())`, injected by the *mode middleware* (`middleware.go`), so the same handler serves the whole single-user store or one hosted user's scoped store. CORS wildcard is now **dev-mode only** (`NewRouter` with a live Vite frontend); production/hosted are same-origin.
- **`internal/auth/`** — hosted-mode identity (used only under `serve --hosted`). `User`/`Session` models, `UserStore`/`SessionStore` interfaces (SQL-shaped), bcrypt hashing (with a fixed dummy hash so login is constant-time across known/unknown emails), and opaque session tokens (SHA-256 stored, never the raw token). `internal/auth/filestore` is the interim file-backed impl; `internal/auth/authtest` is the shared conformance suite. **Never imports `internal/calc`.**
- **`web/`** — SvelteKit SPA (`@sveltejs/adapter-static`), Tailwind, Chart.js. `src/lib/api.ts` is the typed API client (`API_BASE = '/api/v1'`); its interfaces mirror the Go JSON structs. `src/lib/session.ts` (state) + `src/lib/auth.ts` (actions) hold auth state; on boot the layout fetches `/api/v1/meta` and, in hosted mode, redirects unauthenticated users to `/login`. Routes under `src/routes/` (dashboard `+page.svelte`, `graph/`, `add/`, `history/`, `fleet/`, `settings/`, `login/`).

### Server modes (Phase 2)
The same binary runs single-user (default) or hosted multi-tenant (`serve --hosted --data-dir …`). See [docs/hosted-mode.md](docs/hosted-mode.md). The **only fork** is one middleware: `singleUser(store)` injects the process-wide store; `requireSession(sessions, tenants, secure)` authenticates the request and injects `tenants.ForUser(userID)`. **`storage.Tenants` (the `ForUser(userID)` factory) is the Phase 1 ownership seam** — `yamlstore.Tenants` gives each user a directory (byte-compatible with `~/.mileminder`), `storage.MemoryTenants` is the test fake. Extend via this seam; do not add owner parameters to the `storage.Store` interface. `internal/atomicfile` is the shared atomic temp-file+rename writer used by both the YAML and file-backed auth stores.

**Data store:** plain YAML files in `~/.mileminder/`, one per vehicle (`<id>.yml`, where the filename is the vehicle id). A `current` file holds the default vehicle id. There is no database. `internal/model/model.go` defines the shape: optional `Plan *Plan` (start, end, annual_allowance, start_miles; nil means plain tracking) plus `Readings` as a `map["YYYY-MM-DD"]int` of odometer values. `loadVehicle`/`saveVehicle` exist independently in both `cmd/` and `internal/api/` — changes to persistence may need updating in both.

### Calculation model (the heart of the app)
Policy vehicles (`Plan != nil`) compare "miles used" relative to `plan.start_miles`. The **ideal/allowance line** is `annual_allowance * daysElapsed / 365` from plan start — a straight line in real time. Status compares actual miles-driven against this line (`delta`, `percent_used`).

Plain vehicles (`Plan == nil`) use the earliest reading as the baseline. They still compute `daily_rate`, `avg_annual_mileage`, `recent_annual_mileage`, and `pace_trend`, but allowance fields remain zero and `Status.HasPlan`/`has_plan` is false. Clients must gate allowance UI on `has_plan`, not on zero values.

`calc.ComputeStatus()` in `internal/calc/calc.go` is the source of truth and produces several distinct pace figures — keep them straight:
- **`daily_rate`** ("current pace"): miles over the *current allowance year only*. Computed by interpolating the odometer exactly at the year boundary via `calc.OdometerAt()` so pre-year driving doesn't leak in. Drives the year-end projection.
- **`avg_annual_mileage`**: lifetime miles since plan start, annualised — the stable figure to quote for insurance.
- **`recent_annual_mileage`**: trailing 90-day pace, annualised.
- **Allowance "years" / segments**: a plan spans multiple 1-year segments from the plan start date (not calendar years); `segmentStart`/`segmentEnd` logic recurs across status, year-left, and projection calculations.

`calc.ComputeFleetInsights()` includes all vehicles in total annual mileage, but allowance aggregates (`CountOver`, `CountUnder`, `NetDelta`, `AvgPercentUsed`, worst offender) only consider policy vehicles. Use the additive `policy_vehicles` and `plain_vehicles` counts for roll-up labels.

`calc.OdometerAt(readings, date)` (linear interpolation between bracketing readings, clamped at the ends) is the shared primitive behind the year-boundary pace and the 90-day window. Reuse it for any "odometer at an arbitrary date" need. The public `calc.ComputeStatus` wraps an unexported core that takes an injected `now` — `internal/calc/calc_test.go` drives it with a fixed clock for deterministic assertions.

The **graph** uses a real time x-axis (Chart.js time scale via `chartjs-adapter-date-fns`), not per-reading spacing — readings sit at their true dates so slope reflects real time. The allowance and projection lines are sampled weekly so a nearest-on-x tooltip can read off any date.

## Test data

`scripts/seed_testdata.py` (seeded RNG) writes scenario vehicles into `~/.mileminder/` — under/over/on-pace/crossing/new-plan plus `test-plain` for tracking-only UI states. It only writes `test-*` and `testcar` files; it never touches real vehicles.

## Conventions

Conventional commits (`feat:`, `fix:`, `chore:`, `refactor:`). Wrap Go errors with context (`fmt.Errorf(... %w)`). Go tests live in `internal/calc/calc_test.go` (`go test ./internal/calc`) and cover `OdometerAt`, the year-boundary `daily_rate`, and `ComputeStatus` across under/over/on-pace/new-plan cases — extend them when you touch the calculation.
