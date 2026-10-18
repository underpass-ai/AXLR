#!/usr/bin/env python3
"""Apply a records retention schedule GIVEN IN THE INPUT to a list of records: for each record,
every rule of its type with the date its retention ends, the governing (longest) rule, the
action date, and whether it is still retained, past retention, or on legal hold. Periods are
never assumed: a record type without a rule is reported as unscheduled.

Input (JSON on stdin):
  as_of     "YYYY-MM-DD"   the date to judge against (required, so results are reproducible)
  schedule  [{"record_type": "invoice", "period": {"years": 6} | "P6Y" | "permanent",
              "trigger": "last_book_entry",     name of the record date the period runs from (default "date")
              "count_from": "trigger_date" | "next_day",
              "rounding": "none" | "end_of_month" | "end_of_year",   move the trigger to the end of its month/year first
              "action": "destroy" | "review" | "archive",
              "legal_basis": "Código de Comercio, art. 30", "note": "..."}, ...]
  records   [{"id": "F-1", "record_type": "invoice", "dates": {"last_book_entry": "2020-12-31"},
              "legal_hold": false, "hold_reason": "..."}, ...]      "date": "YYYY-MM-DD" is short for dates.date
"""
import calendar
import datetime as dt
import json
import re
import sys

DISCLAIMER = ("Computed from the retention schedule given in the input, not legal advice: the periods, trigger events and "
              "legal bases must be checked against the law that applies to each record, and legal holds against pending matters.")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def date(value, field):
    try:
        return dt.date.fromisoformat(value)
    except (TypeError, ValueError):
        fail(f"{field} must be a date YYYY-MM-DD")


def add_period(start, years, months, days, month_end=False):
    month_index = start.month - 1 + months + 12 * years
    year, month = start.year + month_index // 12, month_index % 12 + 1
    if year > 9999:
        return None
    last = calendar.monthrange(year, month)[1]
    target = dt.date(year, month, last if month_end else min(start.day, last))
    try:
        return target + dt.timedelta(days)
    except OverflowError:
        return None


def parse_period(value, field):
    if value == "permanent":
        return None, "permanent"
    if isinstance(value, str):
        m = re.fullmatch(r"P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)D)?", value.strip().upper())
        if not m or not any(m.groups()):
            fail(f"{field} must be an ISO 8601 period like P6Y or P1Y6M, \"permanent\", or an object")
        years, months, days = (int(g or 0) for g in m.groups())
    elif isinstance(value, dict):
        unknown = set(value) - {"years", "months", "days"}
        if unknown or not value:
            fail(f"{field} takes years, months and days")
        for k, v in value.items():
            if not isinstance(v, int) or isinstance(v, bool) or v < 0:
                fail(f"{field}.{k} must be a non-negative integer")
        years, months, days = value.get("years", 0), value.get("months", 0), value.get("days", 0)
    else:
        fail(f"{field} is required: the retention period is never assumed")
    if years > 1000 or months > 12000 or days > 400000:
        fail(f"{field} is too long")
    label = " ".join(f"{n} {u}{'s' if n != 1 else ''}" for n, u in ((years, "year"), (months, "month"), (days, "day")) if n) or "0 days"
    return (years, months, days), label


def load_schedule(schedule):
    if not isinstance(schedule, list) or not schedule:
        fail("schedule must be a non-empty list of rules: periods are never assumed")
    rules = {}
    for i, rule in enumerate(schedule):
        field = f"schedule[{i}]"
        if not isinstance(rule, dict):
            fail(f"{field} must be an object")
        unknown = set(rule) - {"record_type", "period", "trigger", "count_from", "rounding", "action", "legal_basis", "note", "jurisdiction"}
        if unknown:
            fail(f"{field} has unknown keys: {', '.join(sorted(unknown))}")
        rtype = rule.get("record_type")
        if not isinstance(rtype, str) or not rtype.strip():
            fail(f"{field}.record_type is required")
        period, label = parse_period(rule.get("period"), f"{field}.period")
        trigger = rule.get("trigger", "date")
        if not isinstance(trigger, str) or not trigger:
            fail(f"{field}.trigger must name a record date")
        count_from = rule.get("count_from", "trigger_date")
        if count_from not in ("trigger_date", "next_day"):
            fail(f"{field}.count_from must be trigger_date or next_day")
        rounding = rule.get("rounding", "none")
        if rounding not in ("none", "end_of_month", "end_of_year"):
            fail(f"{field}.rounding must be none, end_of_month or end_of_year")
        action = rule.get("action", "destroy")
        if action not in ("destroy", "review", "archive"):
            fail(f"{field}.action must be destroy, review or archive")
        for key in ("legal_basis", "note", "jurisdiction"):
            if key in rule and not isinstance(rule[key], str):
                fail(f"{field}.{key} must be a string")
        rules.setdefault(rtype.strip(), []).append({"index": i, "period": period, "period_label": label, "trigger": trigger,
                                                    "count_from": count_from, "rounding": rounding, "action": action,
                                                    "legal_basis": rule.get("legal_basis"), "note": rule.get("note"),
                                                    "jurisdiction": rule.get("jurisdiction")})
    return rules


def apply_rule(rule, dates):
    if rule["trigger"] not in dates:
        return {"status": "missing_trigger_date"}
    start = dates[rule["trigger"]]
    if rule["rounding"] == "end_of_month":
        start = start.replace(day=calendar.monthrange(start.year, start.month)[1])
    elif rule["rounding"] == "end_of_year":
        start = start.replace(month=12, day=31)
    if rule["count_from"] == "next_day":
        start += dt.timedelta(1)
    if rule["period"] is None:
        return {"status": "ok", "counted_from": start.isoformat(), "retain_until": None, "permanent": True}
    month_end = rule["rounding"] != "none" and rule["count_from"] == "trigger_date"
    end = add_period(start, *rule["period"], month_end=month_end)
    if end is None:
        return {"status": "ok", "counted_from": start.isoformat(), "retain_until": None, "permanent": True}
    return {"status": "ok", "counted_from": start.isoformat(), "retain_until": end, "permanent": False}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    as_of = date(request.get("as_of"), "as_of")
    rules = load_schedule(request.get("schedule"))
    records = request.get("records")
    if not isinstance(records, list):
        fail("records must be a list")
    results, seen = [], set()
    summary = {"retain": [], "past_retention": [], "on_hold": [], "unscheduled": [], "missing_dates": [], "permanent": []}
    for i, rec in enumerate(records):
        field = f"records[{i}]"
        if not isinstance(rec, dict):
            fail(f"{field} must be an object")
        unknown = set(rec) - {"id", "record_type", "dates", "date", "legal_hold", "hold_reason", "description"}
        if unknown:
            fail(f"{field} has unknown keys: {', '.join(sorted(unknown))}")
        rid = rec.get("id", str(i))
        if not isinstance(rid, (str, int)) or isinstance(rid, bool):
            fail(f"{field}.id must be a string")
        rid = str(rid)
        if rid in seen:
            fail(f"duplicate record id {rid!r}")
        seen.add(rid)
        rtype = rec.get("record_type")
        if not isinstance(rtype, str) or not rtype.strip():
            fail(f"{field}.record_type is required")
        if not isinstance(rec.get("dates", {}), dict):
            fail(f"{field}.dates must be an object of dates")
        raw_dates = dict(rec.get("dates") or {})
        if "date" in rec:
            raw_dates.setdefault("date", rec["date"])
        dates = {k: date(v, f"{field}.dates.{k}") for k, v in raw_dates.items()}
        hold = rec.get("legal_hold", False)
        if not isinstance(hold, bool):
            fail(f"{field}.legal_hold must be true or false")
        row = {"id": rid, "record_type": rtype.strip()}
        applicable = rules.get(rtype.strip())
        if not applicable:
            row["status"] = "unscheduled"
            row["note"] = "no rule for this record type in the schedule; no period is assumed"
            summary["unscheduled"].append(rid)
            results.append(row)
            continue
        applied, missing = [], []
        for rule in applicable:
            outcome = apply_rule(rule, dates)
            entry = {"rule": rule["index"], "period": rule["period_label"], "trigger": rule["trigger"], "legal_basis": rule["legal_basis"],
                     "action": rule["action"]}
            if outcome["status"] == "missing_trigger_date":
                entry["status"] = "missing_trigger_date"
                missing.append(rule["trigger"])
            else:
                entry["counted_from"] = outcome["counted_from"]
                entry["retain_until"] = "permanent" if outcome["permanent"] else outcome["retain_until"].isoformat()
                entry["_end"] = outcome
            applied.append(entry)
        row["rules"] = applied
        complete = [e for e in applied if "_end" in e]
        if missing:
            row["missing_dates"] = sorted(set(missing))
        if not complete:
            row["status"] = "missing_dates"
        else:
            permanent = [e for e in complete if e["_end"]["permanent"]]
            governing = permanent[0] if permanent else max(complete, key=lambda e: e["_end"]["retain_until"])
            row["governing_rule"] = governing["rule"]
            row["governing_legal_basis"] = governing["legal_basis"]
            if permanent:
                row["retain_until"] = "permanent"
                row["status"] = "on_hold" if hold else "permanent"
            else:
                end = governing["_end"]["retain_until"]
                row["retain_until"] = end.isoformat()
                row["action"] = governing["action"]
                row["action_date"] = (end + dt.timedelta(1)).isoformat()
                if hold:
                    row["status"] = "on_hold"
                elif as_of > end:
                    row["status"] = "past_retention"
                    row["days_past_retention"] = (as_of - end).days
                else:
                    row["status"] = "retain"
                    row["days_remaining"] = (end - as_of).days
        if missing and complete and row["status"] != "on_hold":
            row["retain_until_at_least"] = row.pop("retain_until")
            for key in ("action", "action_date", "days_past_retention", "days_remaining"):
                row.pop(key, None)
            row["status"] = "missing_dates"
            row["note"] = "a rule could not be applied for lack of its trigger date: the record cannot be cleared until that date is known"
        if hold:
            row["legal_hold"] = True
            row["hold_reason"] = rec.get("hold_reason")
            row["hold_note"] = "legal hold suspends destruction or review until it is released, whatever the period"
        for e in applied:
            e.pop("_end", None)
        summary[row["status"]].append(rid)
        results.append(row)
    counts = {k: len(v) for k, v in summary.items()}
    print(json.dumps({"as_of": as_of.isoformat(), "records": results, "summary": summary, "counts": counts,
                      "method": "retain_until = trigger date (moved to the end of its month or year if the rule says so, plus one day "
                                "with count_from next_day) + period, months clamped to the month's last day (kept at month end after "
                                "rounding); action date = the day after; "
                                "with several rules for a type the latest end governs",
                      "disclaimer": DISCLAIMER}, ensure_ascii=False))


if __name__ == "__main__":
    main()
