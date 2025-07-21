# Mileage CLI

## 1 Purpose
CLI tool for individual drivers to track odometer readings against the linear annual-mileage allowance of a PCP plan (or similar insurance caps).

## 2 Scope
* **Single user** running locally.
* **Multiple vehicles** supported; one active at a time.
* **Command-line only**—no GUI, no network APIs.
* Runs on macOS, Linux, Windows.

## 3 Definitions
| Term            | Meaning                                                                    |
|-----------------|----------------------------------------------------------------------------|
| **Vehicle ID**  | Friendly name or registration (string).                                    |
| **Plan**        | `start date`, `end date`, `annual_allowance` (mi), `start_miles`.          |
| **Reading**     | `date` (ISO 8601, Europe/London) + `miles` (int). One per day (upsert).    |
| **Slack/Delta** | `target_today – miles_used` (≤ 0 ⇒ under quota).                           |

## 4 Data Storage (YAML)
* Directory: `~/.mileage-cli/`
* **One file per vehicle** → git-friendly.

```yaml
vehicle: my3LR
plan:
  start: 2024-04-15
  end:   2028-04-14
  annual_allowance: 10000
  start_miles: 123
readings:
  "2024-04-15": 123
  "2025-07-20": 15321   # UPSERT same-day
```
## 5 CLI Commands
```
mileage init   --car <id>
mileage add    <miles> [--date YYYY-MM-DD] [--car <id>]
mileage status                [--car <id>] [--plot]
mileage cars
mileage switch <id>
mileage fleet
mileage reset  --car <id>
```

## 6 Computation
```
days_elapsed = today − plan.start
target_today = plan.start_miles + plan.annual_allowance × (days_elapsed / 365)
miles_used   = latest_miles − plan.start_miles
delta        = target_today − miles_used
```

## 7 Status Output (example)
```
📅 20 Jul 2025 | 🚗 my3LR
──────────────────────────────────────────────
Actual Odo:     15 321 mi
Target Today:   14 880 mi
Delta:          +441 mi  ⚠️ (3 % over)

Year left:      165 d   1 843 mi
Term left:      2 y 80 d   11 214 mi
Progress: |███████░░░| 55 %
──────────────────────────────────────────────
```

## 8 Error Handling
• Reading must be numeric and ≥ previous (unless --force).
• Graceful messages for missing car, invalid dates, etc.

## 9 Dependencies
• Go ≥ 1.22
• gopkg.in/yaml.v3

## 10 Future Nice-To-Haves
• SQLite backend for large fleets
• CSV/JSON export
• Web/API dashboard
• Kilometre-unit toggle
