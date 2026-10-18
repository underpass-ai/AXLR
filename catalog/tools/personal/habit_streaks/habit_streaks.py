#!/usr/bin/env python3
"""Habit streaks from a log of dates: current and longest streak, completion rates over windows
and missed days, all relative to an `as_of` date given in the input.

Input (JSON on stdin):
  as_of     "YYYY-MM-DD"  the day the report is made (never the system clock)
  habits    [{"name", "dates": ["YYYY-MM-DD", ...], "schedule"?, "start"?}]
            schedule  "daily" (default) | "weekdays" (Mon-Fri) | {"days": [1, 3, 5]} (ISO weekdays,
                      1 = Monday) | {"per_week": N} (N distinct days in each ISO week, Monday-Sunday)
            start     first day the habit counts from (default: the earliest logged date)
  windows   day counts for completion rates (default [7, 30, 90])
  today_pending  true (default): a due as_of day without a log does not break the streak yet
  missed_limit   most recent missed days listed per habit (default 31)
"""
import datetime as dt
import json
import sys
from fractions import Fraction

ONE = dt.timedelta(days=1)


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def date(value, field):
    try:
        return dt.date.fromisoformat(value)
    except (TypeError, ValueError):
        fail(f"{field} must be a date YYYY-MM-DD")


def pct(num, den):
    """num/den as a percentage with one decimal, rounded half up."""
    if den == 0:
        return None
    x = Fraction(num) * 100 / Fraction(den)
    tenths = x * 10
    whole = tenths.numerator // tenths.denominator
    if tenths - whole >= Fraction(1, 2):
        whole += 1
    return f"{whole // 10}.{whole % 10}"


def parse_schedule(value, field):
    if value is None or value == "daily":
        return "daily", set(range(1, 8)), None
    if value == "weekdays":
        return "weekdays", {1, 2, 3, 4, 5}, None
    if isinstance(value, dict):
        if set(value) == {"days"}:
            days = value["days"]
            if not isinstance(days, list) or not days or any(isinstance(d, bool) or not isinstance(d, int) or not 1 <= d <= 7 for d in days):
                fail(f"{field}.days must be a list of ISO weekdays 1 (Monday) to 7 (Sunday)")
            return "days", set(days), None
        if set(value) == {"per_week"}:
            n = value["per_week"]
            if isinstance(n, bool) or not isinstance(n, int) or not 1 <= n <= 7:
                fail(f"{field}.per_week must be an integer from 1 to 7")
            return "per_week", None, n
    fail(f"{field} must be \"daily\", \"weekdays\", {{\"days\": [...]}} or {{\"per_week\": N}}")


def daily_report(name, done, start, as_of, weekdays, windows, today_pending, missed_limit, label):
    due = []
    d = start
    while d <= as_of:
        if d.isoweekday() in weekdays:
            due.append(d)
        d += ONE
    pending = today_pending and as_of in due and as_of not in done
    longest = run = 0
    longest_end = None
    for d in due:
        if d in done:
            run += 1
            if run > longest:
                longest, longest_end = run, d
        elif d == as_of and pending:
            pass
        else:
            run = 0
    current = 0
    for d in reversed(due):
        if d == as_of and pending:
            continue
        if d in done:
            current += 1
        else:
            break
    missed = [d for d in due if d not in done and not (d == as_of and pending)]
    rates = {}
    for w in windows:
        lo = as_of - (w - 1) * ONE
        window_due = [d for d in due if d >= lo and not (d == as_of and pending)]
        hit = sum(1 for d in window_due if d in done)
        rates[str(w)] = {"done": hit, "due": len(window_due), "rate_pct": pct(hit, len(window_due))}
    all_due = [d for d in due if not (d == as_of and pending)]
    extra = sorted(d for d in done if start <= d <= as_of and d.isoweekday() not in weekdays)
    out = {"name": name, "schedule": label, "start": start.isoformat(), "unit": "day",
           "current_streak": current, "longest_streak": longest,
           "longest_streak_ended": longest_end.isoformat() if longest_end else None,
           "done_today": as_of in done, "today_pending": pending,
           "completion": {"done": sum(1 for d in all_due if d in done), "due": len(all_due),
                          "rate_pct": pct(sum(1 for d in all_due if d in done), len(all_due))},
           "windows": rates, "missed_count": len(missed),
           "missed_recent": [d.isoformat() for d in missed[-missed_limit:]] if missed_limit else []}
    if extra:
        out["logged_on_unscheduled_days"] = [d.isoformat() for d in extra]
    return out


def weekly_report(name, done, start, as_of, n, windows, today_pending, missed_limit):
    monday = lambda d: d - (d.isoweekday() - 1) * ONE
    weeks = []
    w = monday(start)
    while w <= as_of:
        days = [w + i * ONE for i in range(7)]
        count = sum(1 for d in days if d in done and start <= d <= as_of)
        complete = w + 6 * ONE <= as_of
        weeks.append({"week": w, "count": count, "met": count >= n, "complete": complete})
        w += 7 * ONE
    cur = weeks[-1]
    pending = (not cur["complete"]) and not cur["met"] and today_pending
    longest = run = 0
    longest_end = None
    for wk in weeks:
        if wk["met"]:
            run += 1
            if run > longest:
                longest, longest_end = run, wk["week"]
        elif wk is cur and pending:
            pass
        else:
            run = 0
    current = 0
    for wk in reversed(weeks):
        if wk is cur and pending:
            continue
        if wk["met"]:
            current += 1
        else:
            break
    judged = [wk for wk in weeks if not (wk is cur and pending)]
    missed = [wk for wk in judged if not wk["met"]]
    rates = {}
    for days in windows:
        lo = as_of - (days - 1) * ONE
        sel = [wk for wk in judged if wk["week"] + 6 * ONE >= lo]
        hit = sum(1 for wk in sel if wk["met"])
        lo = max(lo, start)
        sessions = sum(1 for d in done if lo <= d <= as_of)
        target = Fraction(n * ((as_of - lo).days + 1), 7)
        rates[str(days)] = {"weeks_met": hit, "weeks": len(sel), "rate_pct": pct(hit, len(sel)), "sessions": sessions,
                            "sessions_target": format(float(target), ".2f"),
                            "sessions_pct": pct(sessions, target) if target else None}
    return {"name": name, "schedule": f"{n} per week (ISO weeks, Monday to Sunday)", "start": start.isoformat(), "unit": "week",
            "current_streak": current, "longest_streak": longest,
            "longest_streak_ended": (longest_end + 6 * ONE).isoformat() if longest_end else None,
            "this_week": {"monday": cur["week"].isoformat(), "count": cur["count"], "target": n, "met": cur["met"],
                          "remaining": max(0, n - cur["count"]), "days_left": (cur["week"] + 6 * ONE - as_of).days},
            "today_pending": pending, "done_today": as_of in done,
            "completion": {"weeks_met": sum(1 for wk in judged if wk["met"]), "weeks": len(judged),
                           "rate_pct": pct(sum(1 for wk in judged if wk["met"]), len(judged))},
            "windows": rates, "missed_count": len(missed),
            "missed_weeks_recent": [{"monday": wk["week"].isoformat(), "count": wk["count"]} for wk in missed[-missed_limit:]] if missed_limit else []}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"as_of", "habits", "windows", "today_pending", "missed_limit"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    if "as_of" not in request:
        fail("as_of is required (the day of the report, YYYY-MM-DD)")
    as_of = date(request["as_of"], "as_of")
    habits = request.get("habits")
    if not isinstance(habits, list) or not habits or len(habits) > 200:
        fail("habits must be a list of 1 to 200 habits")
    windows = request.get("windows", [7, 30, 90])
    if not isinstance(windows, list) or not windows or any(isinstance(w, bool) or not isinstance(w, int) or not 1 <= w <= 3660 for w in windows):
        fail("windows must be a list of day counts from 1 to 3660")
    today_pending = request.get("today_pending", True)
    if not isinstance(today_pending, bool):
        fail("today_pending must be a boolean")
    missed_limit = request.get("missed_limit", 31)
    if isinstance(missed_limit, bool) or not isinstance(missed_limit, int) or not 0 <= missed_limit <= 3660:
        fail("missed_limit must be an integer from 0 to 3660")
    reports = []
    for i, habit in enumerate(habits):
        f = f"habits[{i}]"
        if not isinstance(habit, dict):
            fail(f"{f} must be an object")
        extra = set(habit) - {"name", "dates", "schedule", "start"}
        if extra:
            fail(f"{f}: unknown keys {sorted(extra)}")
        name = habit.get("name", f"habit {i + 1}")
        if not isinstance(name, str):
            fail(f"{f}.name must be a string")
        dates = habit.get("dates", [])
        if not isinstance(dates, list) or len(dates) > 100000:
            fail(f"{f}.dates must be a list of dates")
        logged = {date(d, f"{f}.dates[{j}]") for j, d in enumerate(dates)}
        future = sorted(d for d in logged if d > as_of)
        done = {d for d in logged if d <= as_of}
        if habit.get("start") is not None:
            start = date(habit["start"], f + ".start")
        elif done:
            start = min(done)
        else:
            fail(f"{f}: no dates on or before as_of and no start")
        if start > as_of:
            fail(f"{f}.start is after as_of")
        if (as_of - start).days > 36600:
            fail(f"{f}: more than 100 years of history")
        before = sorted(d for d in done if d < start)
        done = {d for d in done if d >= start}
        kind, weekdays, n = parse_schedule(habit.get("schedule"), f + ".schedule")
        if kind == "per_week":
            report = weekly_report(name, done, start, as_of, n, windows, today_pending, missed_limit)
        else:
            label = {"daily": "daily", "weekdays": "weekdays (Monday to Friday)"}.get(kind) or \
                "on ISO weekdays " + ", ".join(str(d) for d in sorted(weekdays))
            report = daily_report(name, done, start, as_of, weekdays, windows, today_pending, missed_limit, label)
        report["logged_days"] = len(done)
        if future:
            report["ignored_after_as_of"] = len(future)
        if before:
            report["ignored_before_start"] = len(before)
        reports.append(report)
    print(json.dumps({"as_of": as_of.isoformat(), "habits": reports,
                      "rules": ("A streak counts consecutive scheduled days (or ISO weeks for per_week) that were done; "
                                "an unscheduled day neither breaks nor extends it; with today_pending a scheduled as_of "
                                "day (or the current week) that is not done yet is left out instead of breaking the streak.")}))


if __name__ == "__main__":
    main()
