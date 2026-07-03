#!/usr/bin/env python3
"""Generate MileMinder scenario test vehicles into ~/.mileminder/.

Each scenario exercises a distinct dashboard/graph state. Only writes test-*
files (and testcar); never touches real vehicles like "Tesla Model 3".
"""
import os
import random
from datetime import date, timedelta

random.seed(42)
TODAY = date(2026, 6, 28)
HOME = os.path.expanduser("~/.mileminder")


def daterange_samples(start, end):
    """Sample reading dates ~every 14 days with jitter and the odd big gap."""
    dates = [start]
    d = start
    while True:
        step = random.choice([12, 14, 14, 16, 21, 30, 45])  # mostly ~2wk, some gaps
        d = d + timedelta(days=step)
        if d >= end:
            break
        dates.append(d)
    if dates[-1] != end:
        dates.append(end)
    return dates


def piecewise_rate(profile):
    """profile: list of (start_day, end_day, miles_per_day). Returns rate(day)."""
    def rate(day):
        for s, e, r in profile:
            if s <= day < e:
                return r
        return profile[-1][2]
    return rate


def gen(vehicle, plan_start, plan_end, allowance, start_miles, profile,
        sample_end=None, max_readings=None):
    sample_end = min(sample_end or TODAY, plan_end, TODAY)
    rate = piecewise_rate(profile)
    dates = daterange_samples(plan_start, sample_end)
    if max_readings:
        dates = dates[:max_readings]

    readings = {}
    odo = float(start_miles)
    prev = plan_start
    for d in dates:
        days = (d - prev).days
        for i in range(days):
            day_idx = (prev - plan_start).days + i
            odo += rate(day_idx) * random.uniform(0.7, 1.3)  # daily noise
        prev = d
        readings[d.isoformat()] = int(round(odo))
    readings[plan_start.isoformat()] = start_miles  # exact baseline
    return {
        "vehicle": vehicle,
        "start": plan_start,
        "end": plan_end,
        "allowance": allowance,
        "start_miles": start_miles,
        "readings": dict(sorted(readings.items())),
    }


def write_yaml(file_id, v):
    path = os.path.join(HOME, file_id + ".yml")
    lines = []
    lines.append(f"vehicle: {v['vehicle']}")
    lines.append("plan:")
    lines.append(f"    start: {v['start'].isoformat()}T00:00:00Z")
    lines.append(f"    end: {v['end'].isoformat()}T00:00:00Z")
    lines.append(f"    annual_allowance: {v['allowance']}")
    lines.append(f"    start_miles: {v['start_miles']}")
    lines.append("readings:")
    for k, m in v["readings"].items():
        lines.append(f'    "{k}": {m}')
    with open(path, "w") as f:
        f.write("\n".join(lines) + "\n")
    print(f"wrote {path} ({len(v['readings'])} readings)")


def write_plain_yaml(file_id, vehicle, start, start_miles, daily_rate, count=10):
    path = os.path.join(HOME, file_id + ".yml")
    readings = {}
    odo = start_miles
    for i in range(count):
        d = start + timedelta(days=i * 21)
        if i > 0:
            odo += int(round(daily_rate * 21 * random.uniform(0.75, 1.25)))
        readings[d.isoformat()] = odo

    lines = [f"vehicle: {vehicle}", "readings:"]
    for k, m in sorted(readings.items()):
        lines.append(f'    "{k}": {m}')
    with open(path, "w") as f:
        f.write("\n".join(lines) + "\n")
    print(f"wrote {path} ({len(readings)} readings, no plan)")


scenarios = {
    # Crosses from under -> over the ideal line; spans 2 year-boundaries.
    "testcar": gen(
        "testcar (crosses over)", date(2024, 1, 1), date(2028, 1, 1),
        10000, 20000,
        [(0, 365, 18), (365, 730, 28), (730, 99999, 50)],
    ),
    # Light driver, comfortably under allowance.
    "test-under": gen(
        "Daily Commuter (under)", date(2024, 6, 1), date(2027, 6, 1),
        12000, 5000,
        [(0, 99999, 17)],
    ),
    # Heavy driver, clearly projected over.
    "test-over": gen(
        "Road Warrior (over)", date(2025, 1, 1), date(2028, 1, 1),
        8000, 30000,
        [(0, 99999, 38)],
    ),
    # Tracks almost exactly on the ideal line (10000/yr ~= 27.4/day).
    "test-onpace": gen(
        "Steady Eddie (on pace)", date(2024, 3, 1), date(2028, 3, 1),
        10000, 12000,
        [(0, 99999, 27.4)],
    ),
    # Brand-new plan, only a few readings (tests 90-day / early-window edges).
    "test-newplan": gen(
        "Fresh Start (new plan)", date(2026, 6, 1), date(2030, 6, 1),
        10000, 100,
        [(0, 99999, 30)], max_readings=4,
    ),
}

for file_id, v in scenarios.items():
    write_yaml(file_id, v)

write_plain_yaml(
    "test-plain",
    "Owned Runabout (tracking only)",
    date(2025, 12, 1),
    42000,
    24,
)
