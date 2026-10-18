#!/usr/bin/env python3
"""Legislative deadlines from a bill's key dates, a session calendar and the chamber's
counting rules, all given in the input. No parliament's rules are built in.

Input (JSON on stdin):
  calendar  either {"sitting_days": ["YYYY-MM-DD", ...]}
            or {"start": date, "end": date, "sitting_weekdays": [2, 3, 4] (ISO, Monday = 1),
                "recesses": [{"start": date, "end": date, "name": "..."}],
                "non_sitting_days": [dates], "extra_sitting_days": [dates]}
            (the explicit list may also take recesses, non_sitting_days and extra days)
  events    {"publication": "2026-10-01", ...}  the bill's known dates
  rules     [{"name": "amendments", "from": "publication", "count": 6,
              "unit": "sitting_days" | "calendar_days" | "calendar_days_excluding_recess",
              "direction": "after" | "before" (default after),
              "include_start": false (sitting_days: the from date counts as day 1 if it sits),
              "roll": "none" | "next_sitting_day" | "previous_sitting_day"
                      (calendar units: move a deadline that falls on a non-sitting day)}]
            "from" names an event or another rule, so deadlines can chain.
"""
import datetime as dt
import json
import sys

DISCLAIMER = ("Computed only from the calendar and rules given in the input; it does not know any "
              "chamber's standing orders. Check the official calendar and rules before relying on a deadline.")
WEEKDAYS = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def parse(value, field):
    try:
        return dt.date.fromisoformat(value)
    except (TypeError, ValueError):
        fail(f"{field} must be a date YYYY-MM-DD")


def date_list(value, field):
    if value is None:
        return []
    if not isinstance(value, list):
        fail(f"{field} must be a list of dates")
    return [parse(v, f"{field}[]") for v in value]


def read_calendar(cal):
    if not isinstance(cal, dict):
        fail("calendar must be an object")
    unknown = set(cal) - {"sitting_days", "start", "end", "sitting_weekdays", "recesses", "non_sitting_days",
                          "extra_sitting_days"}
    if unknown:
        fail(f"calendar has unknown fields {sorted(unknown)}")
    recesses = []
    raw = cal.get("recesses", [])
    if not isinstance(raw, list):
        fail("calendar.recesses must be a list")
    for i, r in enumerate(raw):
        if not isinstance(r, dict) or set(r) - {"start", "end", "name"}:
            fail(f"calendar.recesses[{i}] must be {{start, end, name}}")
        start, end = parse(r.get("start"), f"recesses[{i}].start"), parse(r.get("end"), f"recesses[{i}].end")
        if end < start:
            fail(f"recesses[{i}] ends before it starts")
        name = r.get("name", f"recess {i + 1}")
        if not isinstance(name, str):
            fail(f"recesses[{i}].name must be a string")
        recesses.append((start, end, name))
    non_sitting = set(date_list(cal.get("non_sitting_days"), "non_sitting_days"))
    extra = set(date_list(cal.get("extra_sitting_days"), "extra_sitting_days"))
    if "sitting_days" in cal:
        listed = date_list(cal["sitting_days"], "sitting_days")
        if not listed:
            fail("calendar.sitting_days is empty")
        days = set(listed) | extra
        first = parse(cal["start"], "calendar.start") if "start" in cal else min(days)
        last = parse(cal["end"], "calendar.end") if "end" in cal else max(days)
    else:
        if "start" not in cal or "end" not in cal or "sitting_weekdays" not in cal:
            fail("calendar needs sitting_days, or start, end and sitting_weekdays")
        first, last = parse(cal["start"], "calendar.start"), parse(cal["end"], "calendar.end")
        weekdays = cal["sitting_weekdays"]
        if (not isinstance(weekdays, list) or not weekdays
                or not all(isinstance(w, int) and not isinstance(w, bool) and 1 <= w <= 7 for w in weekdays)):
            fail("calendar.sitting_weekdays must list ISO weekdays 1 (Monday) to 7 (Sunday)")
        days = {first + dt.timedelta(i) for i in range((last - first).days + 1)
                if (first + dt.timedelta(i)).isoweekday() in weekdays} | extra
    if last < first:
        fail("calendar ends before it starts")
    if (last - first).days > 3700:
        fail("calendar spans more than ten years")

    def in_recess(day):
        return next((name for start, end, name in recesses if start <= day <= end), None)

    sitting = sorted(d for d in days if first <= d <= last and d not in non_sitting and (d in extra or not in_recess(d)))
    return first, last, sitting, in_recess


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    unknown = set(request) - {"calendar", "events", "rules"}
    if unknown:
        fail(f"unknown fields: {sorted(unknown)}")
    first, last, sitting, in_recess = read_calendar(request.get("calendar"))
    sitting_set = set(sitting)
    events = request.get("events", {})
    if not isinstance(events, dict):
        fail("events must be an object of name: date")
    known = {name: parse(value, f"events.{name}") for name, value in events.items()}
    rules = request.get("rules")
    if not isinstance(rules, list) or not rules:
        fail("rules must be a non-empty list")
    by_name = {}
    for i, rule in enumerate(rules):
        if not isinstance(rule, dict):
            fail(f"rules[{i}] must be an object")
        bad = set(rule) - {"name", "from", "count", "unit", "direction", "include_start", "roll"}
        if bad:
            fail(f"rules[{i}] has unknown fields {sorted(bad)}")
        name = rule.get("name")
        if not isinstance(name, str) or not name or name in by_name or name in known:
            fail(f"rules[{i}].name must be a new, unique name")
        count = rule.get("count")
        if not isinstance(count, int) or isinstance(count, bool) or not 0 <= count <= 36500:
            fail(f"rules[{i}].count must be an integer from 0 to 36500")
        if rule.get("unit", "sitting_days") not in ("sitting_days", "calendar_days", "calendar_days_excluding_recess"):
            fail(f"rules[{i}].unit must be sitting_days, calendar_days or calendar_days_excluding_recess")
        if rule.get("direction", "after") not in ("after", "before"):
            fail(f"rules[{i}].direction must be after or before")
        if rule.get("roll", "none") not in ("none", "next_sitting_day", "previous_sitting_day"):
            fail(f"rules[{i}].roll must be none, next_sitting_day or previous_sitting_day")
        if not isinstance(rule.get("include_start", False), bool):
            fail(f"rules[{i}].include_start must be true or false")
        if not isinstance(rule.get("from"), str):
            fail(f"rules[{i}].from must name an event or a rule")
        by_name[name] = rule
    for name, rule in by_name.items():
        if rule["from"] not in known and rule["from"] not in by_name:
            fail(f"rule {name} counts from {rule['from']!r}, which is neither an event nor a rule")

    results, resolving = {}, set()

    def sitting_step(start, count, step, include_start):
        if include_start and start in sitting_set and count > 0:
            count -= 1
            if count == 0:
                return start
        day = start
        while count:
            day += dt.timedelta(step)
            if day > last or day < first:
                return None
            if day in sitting_set:
                count -= 1
        return day

    def roll(day, how):
        step = 1 if how == "next_sitting_day" else -1
        while day not in sitting_set:
            day += dt.timedelta(step)
            if day > last or day < first:
                return None
        return day

    def resolve(name):
        if name in known:
            return known[name]
        if name in results:
            return dt.date.fromisoformat(results[name]["deadline"]) if results[name]["deadline"] else None
        if name in resolving:
            fail(f"rules depend on each other in a cycle through {name}")
        resolving.add(name)
        rule = by_name[name]
        start = resolve(rule["from"])
        unit = rule.get("unit", "sitting_days")
        direction = rule.get("direction", "after")
        step = 1 if direction == "after" else -1
        count = rule["count"]
        out = {"name": name, "from": rule["from"], "from_date": start.isoformat() if start else None,
               "count": count, "unit": unit, "direction": direction}
        deadline, note = None, None
        if start is None:
            note = f"{rule['from']} has no date"
        elif unit == "sitting_days":
            if not first <= start <= last:
                note = "the start date is outside the calendar given"
            else:
                deadline = sitting_step(start, count, step, rule.get("include_start", False))
                if deadline is None:
                    note = "the count runs past the end of the calendar given"
        else:
            day, left, skipped = start, count, 0
            while left:
                day += dt.timedelta(step)
                if unit == "calendar_days_excluding_recess" and in_recess(day):
                    skipped += 1
                    continue
                left -= 1
            if unit == "calendar_days_excluding_recess":
                out["recess_days_skipped"] = skipped
            deadline = day
            how = rule.get("roll", "none")
            if how != "none" and deadline not in sitting_set:
                if not first <= deadline <= last:
                    deadline, note = None, "the deadline falls outside the calendar given, so it cannot be rolled"
                else:
                    out["unrolled"] = day.isoformat()
                    deadline = roll(deadline, how)
                    if deadline is None:
                        note = "no sitting day to roll to inside the calendar given"
        out["deadline"] = deadline.isoformat() if deadline else None
        if deadline:
            out["weekday"] = WEEKDAYS[deadline.weekday()]
            out["is_sitting_day"] = deadline in sitting_set
            recess = in_recess(deadline)
            if recess:
                out["in_recess"] = recess
        if note:
            out["note"] = note
        resolving.discard(name)
        results[name] = out
        return deadline

    for name in by_name:
        resolve(name)
    print(json.dumps({
        "calendar": {"start": first.isoformat(), "end": last.isoformat(), "sitting_days": len(sitting)},
        "deadlines": [results[name] for name in by_name],
        "disclaimer": DISCLAIMER,
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
