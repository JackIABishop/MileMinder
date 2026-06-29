# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

MileMinder is a CLI + web dashboard for keeping a car's odometer under a PCP/insurance mileage allowance. Module path: `github.com/jackiabishop/mileminder`.

## Commands

```bash
make build        # build web UI then Go binary -> ./mileminder
make build-web    # cd web && npm install && npm run build, then copy web/dist -> internal/web/dist
make build-go     # go build -o mileminder .
make install      # go mod download + npm install
make test         # go test ./...   (no Go test files exist yet)
make clean        # remove binary, web/dist, web/node_modules, web/.svelte-kit, internal/web/dist/*
```

Run a single Go test once tests exist: `go test ./internal/api -run TestName`.

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

Three layers share one data model but **do not share calculation code**:

- **`cmd/`** — Cobra CLI. Each command is its own file (`add`, `status`, `graph`, `fleet`, `cars`, `switch`, `init`, `reset`, `serve`). `serve` is the bridge to the web layer. CLI commands read/write YAML directly and have their own copies of the status math.
- **`internal/api/`** — HTTP layer. `router.go` wires a stdlib `http.ServeMux` (Go 1.22 method+path patterns like `GET /api/vehicles/{id}`) with CORS and an SPA fallback handler (unknown paths serve `index.html` for client-side routing). `handlers.go` holds all endpoint logic **and `computeStatus()`**, the canonical status/projection calculator.
- **`web/`** — SvelteKit SPA (`@sveltejs/adapter-static`), Tailwind, Chart.js. `src/lib/api.ts` is the typed API client; its interfaces mirror the Go JSON structs. Routes under `src/routes/` (dashboard `+page.svelte`, `graph/`, `add/`, `history/`, `fleet/`, `settings/`).

**Data store:** plain YAML files in `~/.mileminder/`, one per vehicle (`<id>.yml`, where the filename is the vehicle id). A `current` file holds the default vehicle id. There is no database. `internal/model/model.go` defines the shape: a `Plan` (start, end, annual_allowance, start_miles) plus `Readings` as a `map["YYYY-MM-DD"]int` of odometer values. `loadVehicle`/`saveVehicle` exist independently in both `cmd/` and `internal/api/` — changes to persistence may need updating in both.

### Calculation model (the heart of the app)
"Miles used" is always relative to `start_miles`. The **ideal/allowance line** is `annual_allowance * daysElapsed / 365` from plan start — a straight line in real time. Status compares actual miles-driven against this line (`delta`, `percent_used`).

`computeStatus()` in `handlers.go` is the source of truth and produces several distinct pace figures — keep them straight:
- **`daily_rate`** ("current pace"): miles over the *current allowance year only*. Computed by interpolating the odometer exactly at the year boundary via `odometerAt()` so pre-year driving doesn't leak in. Drives the year-end projection.
- **`avg_annual_mileage`**: lifetime miles since plan start, annualised — the stable figure to quote for insurance.
- **`recent_annual_mileage`**: trailing 90-day pace, annualised.
- **Allowance "years" / segments**: a plan spans multiple 1-year segments from the plan start date (not calendar years); `segmentStart`/`segmentEnd` logic recurs across status, year-left, and projection calculations.

`odometerAt(readings, date)` (linear interpolation between bracketing readings, clamped at the ends) is the shared primitive behind the year-boundary pace and the 90-day window. Reuse it for any "odometer at an arbitrary date" need.

The **graph** uses a real time x-axis (Chart.js time scale via `chartjs-adapter-date-fns`), not per-reading spacing — readings sit at their true dates so slope reflects real time. The allowance and projection lines are sampled weekly so a nearest-on-x tooltip can read off any date.

## Test data

`scripts/seed_testdata.py` (seeded RNG) writes scenario vehicles into `~/.mileminder/` — under/over/on-pace/crossing/new-plan cases for eyeballing UI states. It only writes `test-*` and `testcar` files; it never touches real vehicles.

## Conventions

Conventional commits (`feat:`, `fix:`, `chore:`, `refactor:`). Wrap Go errors with context (`fmt.Errorf(... %w)`). The repo currently has **no Go tests** — `computeStatus`/`odometerAt` are prime candidates if adding coverage.
