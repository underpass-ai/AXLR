#!/usr/bin/env python3
"""Legal and administrative time limits from a rule the caller states: working or calendar
days, weeks, months or years; the day the count starts; which weekdays, dates and periods are
non-working; how a month ends; and what happens when the last day is non-working. The rule is
always given in the input; nothing is assumed about a jurisdiction.

Input (JSON on stdin):
  start   "YYYY-MM-DD"  the day of the triggering act (notification, publication)
  amount  integer
  unit    "working_days" | "calendar_days" | "weeks" | "months" | "years"
  rule    {
    "count_from": "next_day" | "same_day",
    "non_working_weekdays": [6, 7],            ISO weekdays; required for working_days
    "holidays": ["YYYY-MM-DD", ...],
    "non_working_periods": [{"from": "08-01", "to": "08-31"}],   MM-DD (every year) or full dates
    "month_end": "same_day" | "day_before",    months/years: the same date number, or the day before it;
                                                 missing dates fall to the month's last day
    "if_last_day_non_working": "next_working_day" | "keep"
  }
"""
import calendar
import datetime as dt
import json
import sys

WEEKDAYS = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def date(value, field):
    try:
        return dt.date.fromisoformat(value)
    except (TypeError, ValueError):
        fail(f"{field} must be a date YYYY-MM-DD")


def period_matcher(periods):
    checks = []
    for i, p in enumerate(periods):
        if not isinstance(p, dict) or not isinstance(p.get("from"), str) or not isinstance(p.get("to"), str):
            fail(f"rule.non_working_periods[{i}] needs from and to")
        a, b = p["from"], p["to"]
        if len(a) == 5 and len(b) == 5:
            try:
                ka, kb = (int(a[:2]), int(a[3:])), (int(b[:2]), int(b[3:]))
            except ValueError:
                fail(f"rule.non_working_periods[{i}] must be MM-DD or YYYY-MM-DD")
            checks.append(lambda d, ka=ka, kb=kb: (ka <= (d.month, d.day) <= kb) if ka <= kb else ((d.month, d.day) >= ka or (d.month, d.day) <= kb))
        else:
            da, db = date(a, f"rule.non_working_periods[{i}].from"), date(b, f"rule.non_working_periods[{i}].to")
            checks.append(lambda d, da=da, db=db: da <= d <= db)
    return lambda d: any(c(d) for c in checks)


def add_months(start, months, month_end):
    month_index = start.month - 1 + months
    year, month = start.year + month_index // 12, month_index % 12 + 1
    last = calendar.monthrange(year, month)[1]
    target = dt.date(year, month, min(start.day, last))
    if month_end == "day_before" and start.day <= last:
        target -= dt.timedelta(1)
    return target


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    start = date(request.get("start"), "start")
    amount, unit, rule = request.get("amount"), request.get("unit"), request.get("rule")
    if not isinstance(amount, int) or isinstance(amount, bool) or amount < 0 or amount > 100000:
        fail("amount must be a non-negative integer")
    if unit not in ("working_days", "calendar_days", "weeks", "months", "years"):
        fail("unit must be working_days, calendar_days, weeks, months or years")
    if not isinstance(rule, dict):
        fail("rule is required: the jurisdiction's counting rule is never assumed")
    count_from = rule.get("count_from")
    if count_from not in ("next_day", "same_day"):
        fail("rule.count_from must be next_day or same_day")
    weekdays = rule.get("non_working_weekdays")
    if weekdays is None and unit == "working_days":
        fail("rule.non_working_weekdays is required for working_days (e.g. [6, 7] for Saturday and Sunday)")
    weekdays = set(weekdays or [])
    if not all(isinstance(w, int) and 1 <= w <= 7 for w in weekdays):
        fail("rule.non_working_weekdays must be ISO weekdays 1-7")
    holidays = {date(h, "rule.holidays[]") for h in rule.get("holidays", [])}
    in_period = period_matcher(rule.get("non_working_periods", []))
    month_end = rule.get("month_end", "same_day")
    if month_end not in ("same_day", "day_before"):
        fail("rule.month_end must be same_day or day_before")
    shift = rule.get("if_last_day_non_working")
    if shift not in ("next_working_day", "keep"):
        fail("rule.if_last_day_non_working must be next_working_day or keep")

    def why_not_working(d):
        if d.isoweekday() in weekdays:
            return "weekday"
        if d in holidays:
            return "holiday"
        if in_period(d):
            return "period"
        return None

    skipped = {"weekday": 0, "holiday": 0, "period": 0}
    first = start + dt.timedelta(1) if count_from == "next_day" else start
    if unit == "working_days":
        current, counted = first - dt.timedelta(1), 0
        while counted < amount:
            current += dt.timedelta(1)
            reason = why_not_working(current)
            if reason:
                skipped[reason] += 1
            else:
                counted += 1
        end = current if amount else start
    elif unit in ("calendar_days", "weeks"):
        days = amount * (7 if unit == "weeks" else 1)
        end = first + dt.timedelta(days - 1) if days else start
    else:
        months = amount * (12 if unit == "years" else 1)
        end = add_months(start, months, month_end)
    nominal, shifted_by = end, 0
    if shift == "next_working_day":
        while why_not_working(end):
            skipped[why_not_working(end)] += 1
            end += dt.timedelta(1)
            shifted_by += 1
            if shifted_by > 3660:
                fail("no working day within ten years after the nominal end")
    result = {
        "start": start.isoformat(), "amount": amount, "unit": unit, "first_day_counted": first.isoformat(),
        "nominal_end": nominal.isoformat(), "end": end.isoformat(), "weekday": WEEKDAYS[end.weekday()],
        "calendar_days_elapsed": (end - start).days, "shifted_days": shifted_by,
        "skipped": {"non_working_weekdays": skipped["weekday"], "holidays": skipped["holiday"], "non_working_periods": skipped["period"]},
        "disclaimer": "Computed from the rule given in the input, not legal advice: check the rule against the applicable law and the official calendar of non-working days.",
    }
    print(json.dumps(result))


if __name__ == "__main__":
    main()
