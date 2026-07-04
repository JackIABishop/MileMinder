# MileMinder 🚗
[![Go](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/github/license/JackIABishop/MileMinder)](LICENSE)

Web UI + CLI to keep your PCP / insurance mileage allowance on track — no spreadsheets, no drama.

## ✨ Features

### CLI Commands
- **`init`** – set term dates, start odo & yearly cap  
- **`add`** – upsert today's odometer reading  
- **`status`** – see delta vs ideal line (year & term left)  
- **`graph`** – ASCII chart of actual vs ideal miles  
- **`serve`** – launch the web UI dashboard
- Fleet commands: `cars`, `switch`, `fleet`, `reset`

### Web UI
- **Dashboard** – Visual gauge showing usage percentage, delta status, and projections
- **Add Mileage** – Quick-add buttons and full form for logging readings
- **Interactive Graph** – Chart.js powered visualization of actual vs ideal miles
- **Fleet Overview** – Card view of all vehicles with quick stats
- **History** – View, edit, and export your reading history as CSV

## 🚀 Installation

```bash
go install github.com/jackiabishop/mileminder@latest
# add GOPATH/bin to $PATH if needed
```

### Building from Source
```bash
git clone https://github.com/JackIABishop/MileMinder.git
cd MileMinder
make install  # Install Go and npm dependencies
make build    # Build web UI and Go binary
```

## 🏃 Quick-start

### CLI Usage
```bash
mileminder init --car my3LR         # interactive wizard
mileminder add 15321                # log today's odometer
mileminder status                   # usage snapshot
mileminder graph                    # ascii chart
```

### Web UI
```bash
mileminder serve                    # starts at http://localhost:8080
mileminder serve --port 3000        # custom port
mileminder serve --no-browser       # don't auto-open browser
```

## 📸 Screenshots

### CLI Status Output
```
📅 31 Jul 2025  | 🚗 Tesla Model 3
──────────────────────────────────────────────────
Actual Odo:     902 mi
Target Today:   1150 mi
Delta:          -255 mi  ✅ (78%)

Year left:      324 d   8 857 mi
Term left:      3y 324d  38 884 mi
Usage:   |████████░░| 78%
```

### Web UI Dashboard

<p align="center">
  <img src="docs/screenshots/HomeUI.png" alt="MileMinder Dashboard" width="800">
</p>

The web interface provides a modern, dark-themed dashboard with:
- Circular gauge showing percentage used
- Delta indicator (under/over allowance)
- Year-end projections based on current rate
- Quick-add buttons for common distances

### Mileage Graph

<p align="center">
  <img src="docs/screenshots/GraphUI.png" alt="MileMinder Graph" width="800">
</p>

- Interactive Chart.js visualization of actual vs allowance limit
- Fleet overview across all vehicles

## 🔧 Configuration

YAML lives per-car under `~/.mileminder/`:
```yaml
vehicle: tesla_model_3
plan:
  start: 2024-04-15
  end:   2028-04-14
  annual_allowance: 10000
  start_miles: 7
readings:
    "2025-07-20": 600
    "2025-07-21": 623
    "2025-07-26": 702
    "2025-07-31": 902
```

Both CLI and Web UI read/write to the same files, so changes sync automatically.

## 🛠️ Development

### Prerequisites
- Go 1.22+
- Node.js 20+ and npm

### Development Mode
Run the API server and web dev server separately for hot reloading:

```bash
# Terminal 1: API server
make dev-api

# Terminal 2: Web UI with hot reload
make dev-web
```

The web dev server runs at `http://localhost:5173` and proxies API requests to the Go server at port 8080.

### Project Structure
```
mileminder/
├── cmd/              # CLI commands (Cobra)
├── internal/
│   ├── api/          # REST API handlers
│   ├── model/        # Data models
│   └── web/          # Embedded web UI files
├── web/              # Svelte frontend source
│   ├── src/
│   │   ├── lib/      # Components & utilities
│   │   └── routes/   # SvelteKit pages
│   └── package.json
├── Makefile
└── main.go
```

### Building
```bash
make build      # Build both web UI and Go binary
make build-web  # Build web UI only
make build-go   # Build Go binary only
make clean      # Clean all build artifacts
```

## 💙 Contributing  
PRs, issues, and feature ideas are welcome! Open an issue, submit a PR, or drop me an email at **hello@jackbishop.co**.

## ☕️ Support  

If MileMinder saves you mileage-overage fees, [**buy me a coffee**](https://buymeacoffee.com/jackbishop) — caffeine → more late-night commits!

## 📜 License  
Released under the [MIT License](LICENSE).
