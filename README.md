# MileMinder 🚗⚡
[![Go](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/github/license/JackIABishop/mileminder)](LICENSE)

Tiny CLI to keep your PCP / insurance mileage allowance on track — no spreadsheets, no drama.

## ✨ Features
- **`init`** – set term dates, start odo & yearly cap  
- **`add`** – upsert today’s odometer reading  
- **`status`** – see delta vs ideal line (year & term left)  
- **`graph`** – ASCII chart of actual vs ideal miles  
- Fleet commands: `cars`, `switch`, `fleet`, `reset`

## 🚀 Installation
```bash
go install github.com/jackiabishop/mileminder@latest
# add GOPATH/bin to $PATH if needed
```

## 🏃 Quick-start
```bash
mileminder init --car my3LR         # interactive wizard
mileminder add 15321                # log today's odometer
mileminder status                   # usage snapshot
mileminder graph                    # ascii chart
```

## 📸 Status Output (example)
```
📅 31 Jul 2025  | 🚗 Tesla Model 3
──────────────────────────────────────────────────
Actual Odo:     902 mi
Target Today:   1150 mi
Delta:          -255 mi  ✅ (78%)

Year left:      324 d   8 857 mi
Term left:      3y 324d  38 884 mi
Usage:   |████████░░| 78%
```


## 🔧 Configuration
YAML lives per-car under ~/.mileminder/:
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

## 💙 Contributing  
PRs, issues, and feature ideas are welcome!  
Open an issue, submit a PR, or drop me an email at **hello@jackbishop.co**.

## ☕️ Support  

If MileMinder saves you mileage-overage fees,  
[**buy me a coffee**](https://buymeacoffee.com/jackbishop) — caffeine → more late-night commits!

## 📜 License  
Released under the [MIT License](LICENSE).
