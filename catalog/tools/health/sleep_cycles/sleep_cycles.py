#!/usr/bin/env python3
"""Bedtime and wake-time planner in whole sleep cycles, across midnight.

Input (JSON on stdin):
  mode               "wake_at" (time is the wake-up time: list bedtimes)
                     | "bed_at" (time is when you go to bed: list wake-up times)
  time               "HH:MM" (24-hour)
  cycle_minutes      default 90 (30-150)
  latency_minutes    time to fall asleep, default 15 (0-120)
  min_cycles         default 3
  max_cycles         default 6
  recommended_hours  [low, high] total sleep counted as recommended, default [7, 9]
                     (adults 18-64: National Sleep Foundation, Hirshkowitz et al. 2015)
"""
import json
import re
import sys

DISCLAIMER = ("Not medical advice: the 90-minute cycle is an average and real cycles vary between people and "
              "through the night. Persistent sleep problems need a qualified clinician.")
DAY = 1440


def fail(message):
    print(json.dumps({"error": message, "disclaimer": DISCLAIMER}))
    sys.exit(2)


def integer(request, field, default, low, high):
    value = request.get(field, default)
    if isinstance(value, bool) or not isinstance(value, int) or not low <= value <= high:
        fail(f"{field} must be an integer from {low} to {high}")
    return value


def clock(minutes):
    offset, rest = divmod(minutes, DAY)
    return f"{rest // 60:02d}:{rest % 60:02d}", offset


def duration(minutes):
    return f"{minutes // 60}:{minutes % 60:02d}"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"mode", "time", "cycle_minutes", "latency_minutes", "min_cycles", "max_cycles",
                                   "recommended_hours"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    mode = request.get("mode")
    if mode not in ("wake_at", "bed_at"):
        fail("mode must be wake_at or bed_at")
    raw = request.get("time")
    match = re.fullmatch(r"([01]?\d|2[0-3]):([0-5]\d)", raw) if isinstance(raw, str) else None
    if not match:
        fail("time must be HH:MM, 24-hour")
    base = int(match.group(1)) * 60 + int(match.group(2))
    cycle = integer(request, "cycle_minutes", 90, 30, 150)
    latency = integer(request, "latency_minutes", 15, 0, 120)
    low_c = integer(request, "min_cycles", 3, 1, 12)
    high_c = integer(request, "max_cycles", 6, 1, 12)
    if low_c > high_c:
        fail("min_cycles must not exceed max_cycles")
    hours = request.get("recommended_hours", [7, 9])
    if (not isinstance(hours, list) or len(hours) != 2
            or not all(isinstance(h, (int, float)) and not isinstance(h, bool) for h in hours)
            or not 0 <= hours[0] <= hours[1] <= 24):
        fail("recommended_hours must be [low, high] with 0 <= low <= high <= 24")
    options = []
    for n in range(high_c, low_c - 1, -1) if mode == "wake_at" else range(low_c, high_c + 1):
        sleep = n * cycle
        if mode == "wake_at":
            asleep = base - sleep
            target = asleep - latency
        else:
            asleep = base + latency
            target = asleep + sleep
        time_text, offset = clock(target)
        asleep_text, asleep_offset = clock(asleep)
        options.append({
            "cycles": n, "sleep_minutes": sleep, "sleep_duration": duration(sleep),
            ("bedtime" if mode == "wake_at" else "wake_time"): time_text, "day_offset": offset,
            "day": {-1: "previous day", 0: "same day", 1: "next day"}.get(offset, f"{offset:+d} days"),
            "fall_asleep_at": asleep_text, "fall_asleep_day_offset": asleep_offset,
            "time_in_bed_minutes": sleep + latency,
            "recommended": hours[0] * 60 <= sleep <= hours[1] * 60,
        })
    result = {"mode": mode, "time": f"{base // 60:02d}:{base % 60:02d}", "cycle_minutes": cycle,
              "latency_minutes": latency, "recommended_hours": hours, "options": options,
              "notes": ["day_offset is relative to the day of the given time.",
                        "Recommended range default: 7-9 h for adults 18-64 (National Sleep Foundation, Hirshkowitz et al., "
                        "Sleep Health 2015); give recommended_hours for other ages."],
              "disclaimer": DISCLAIMER}
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
