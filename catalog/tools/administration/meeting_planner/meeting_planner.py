#!/usr/bin/env python3
"""Common working hours across IANA time zones (zoneinfo): for each day of a date or a date
range, the UTC windows when every participant is inside their working hours, each
participant's local times for them, and ranked meeting slots of a given duration.

Input (JSON on stdin):
  participants  [{"name", "tz", "start": "09:00", "end": "18:00", "workdays": [1..7]?}, ...]
                workdays are ISO weekdays (default Monday to Friday); end <= start means
                the working period runs past midnight
  date | start_date + end_date   "YYYY-MM-DD", days of reference_tz (at most 62 days)
  reference_tz  optional zone whose calendar days are used (default: the first participant's)
  duration_minutes  default 30;  step_minutes default 15;  max_slots default 10
"""
import datetime as dt
import json
import re
import sys

try:
    import zoneinfo
except ImportError:  # pragma: no cover
    zoneinfo = None

UTC = dt.timezone.utc
WEEKDAYS = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def zone(name, field):
    if zoneinfo is None:
        fail("this Python has no zoneinfo module")
    if not isinstance(name, str) or not name.strip():
        fail(f"{field} must be an IANA time zone such as Europe/Madrid")
    try:
        return zoneinfo.ZoneInfo(name)
    except (zoneinfo.ZoneInfoNotFoundError, ValueError, TypeError):
        fail(f"{field} {name!r} is not a known IANA time zone")


def clock(value, field):
    m = re.match(r"^(\d{1,2}):(\d{2})$", value) if isinstance(value, str) else None
    if not m:
        fail(f"{field} must be HH:MM")
    h, mi = int(m.group(1)), int(m.group(2))
    if (h, mi) == (24, 0):
        return 24 * 60
    if h > 23 or mi > 59:
        fail(f"{field} must be a time from 00:00 to 24:00")
    return h * 60 + mi


def day(value, field):
    m = re.match(r"^(\d{4})-(\d{2})-(\d{2})$", value) if isinstance(value, str) else None
    if not m:
        fail(f"{field} must be YYYY-MM-DD")
    try:
        return dt.date(*(int(g) for g in m.groups()))
    except ValueError as error:
        fail(f"{field} is not a valid date: {error}")


def positive(request, key, default, low, high):
    value = request.get(key, default)
    if not isinstance(value, int) or isinstance(value, bool) or not low <= value <= high:
        fail(f"{key} must be an integer from {low} to {high}")
    return value


def local_instant(date, minutes, tz):
    moment = dt.datetime.combine(date, dt.time()) + dt.timedelta(minutes=minutes)
    return moment.replace(tzinfo=tz).astimezone(UTC)


def working_intervals(person, window_start, window_end):
    """UTC intervals of the person's working periods that touch the window."""
    tz = person["zone"]
    first = window_start.astimezone(tz).date() - dt.timedelta(days=1)
    last = window_end.astimezone(tz).date()
    out = []
    d = first
    while d <= last:
        if d.isoweekday() in person["workdays"]:
            s = local_instant(d, person["start"], tz)
            end_minutes = person["end"] if person["end"] > person["start"] else person["end"] + 1440
            e = local_instant(d, end_minutes, tz)
            s, e = max(s, window_start), min(e, window_end)
            if s < e:
                out.append((s, e, local_instant(d, person["start"], tz), local_instant(d, end_minutes, tz)))
        d += dt.timedelta(days=1)
    return out


def intersect(a, b):
    out, i, j = [], 0, 0
    while i < len(a) and j < len(b):
        s, e = max(a[i][0], b[j][0]), min(a[i][1], b[j][1])
        if s < e:
            out.append((s, e))
        if a[i][1] < b[j][1]:
            i += 1
        else:
            j += 1
    return out


def local_view(moment, tz):
    local = moment.astimezone(tz)
    offset = local.utcoffset()
    sign = "-" if offset < dt.timedelta(0) else "+"
    minutes = abs(int(offset.total_seconds())) // 60
    return {"date": local.date().isoformat(), "time": local.strftime("%H:%M"), "weekday": WEEKDAYS[local.weekday()],
            "utc_offset": f"{sign}{minutes // 60:02d}:{minutes % 60:02d}"}


def iso_utc(moment):
    return moment.astimezone(UTC).strftime("%Y-%m-%dT%H:%MZ")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"participants", "date", "start_date", "end_date", "reference_tz", "duration_minutes", "step_minutes", "max_slots"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    people_in = request.get("participants")
    if not isinstance(people_in, list) or not people_in:
        fail("participants must be a non-empty list")
    if len(people_in) > 50:
        fail("at most 50 participants")
    people, names = [], set()
    for i, p in enumerate(people_in):
        field = f"participants[{i}]"
        if not isinstance(p, dict):
            fail(f"{field} must be an object")
        bad = sorted(set(p) - {"name", "tz", "start", "end", "workdays"})
        if bad:
            fail(f"{field} has unknown field(s): {', '.join(bad)}")
        name = p.get("name", f"participant {i + 1}")
        if not isinstance(name, str) or not name.strip():
            fail(f"{field}.name must be a non-empty string")
        if name in names:
            fail(f"participant names must be unique: {name!r} repeats")
        names.add(name)
        start = clock(p.get("start", "09:00"), f"{field}.start")
        end = clock(p.get("end", "17:00"), f"{field}.end")
        if start == end or start == 1440:
            fail(f"{field}: start and end must differ and start must be before 24:00")
        workdays = p.get("workdays", [1, 2, 3, 4, 5])
        if not isinstance(workdays, list) or not all(isinstance(w, int) and not isinstance(w, bool) and 1 <= w <= 7 for w in workdays):
            fail(f"{field}.workdays must be a list of ISO weekdays 1 (Monday) to 7 (Sunday)")
        people.append({"name": name, "tz_name": p.get("tz"), "zone": zone(p.get("tz"), f"{field}.tz"),
                       "start": start, "end": end, "workdays": set(workdays)})

    if request.get("date") is not None:
        if request.get("start_date") is not None or request.get("end_date") is not None:
            fail("give date, or start_date and end_date, not both")
        first = last = day(request["date"], "date")
    else:
        if request.get("start_date") is None or request.get("end_date") is None:
            fail("give date, or start_date and end_date")
        first, last = day(request["start_date"], "start_date"), day(request["end_date"], "end_date")
        if last < first:
            fail("end_date is before start_date")
        if (last - first).days > 61:
            fail("the range may span at most 62 days")
    if not (1900 <= first.year and last.year <= 2200):
        fail("dates must fall between the years 1900 and 2200")
    ref_name = request.get("reference_tz", people[0]["tz_name"])
    ref = zone(ref_name, "reference_tz")
    duration = positive(request, "duration_minutes", 30, 1, 1440)
    step = positive(request, "step_minutes", 15, 1, 240)
    max_slots = positive(request, "max_slots", 10, 0, 500)

    windows, slots = [], []
    d = first
    while d <= last:
        w_start, w_end = local_instant(d, 0, ref), local_instant(d + dt.timedelta(days=1), 0, ref)
        per_person = [working_intervals(p, w_start, w_end) for p in people]
        common = [(s, e) for s, e, _, _ in per_person[0]]
        for intervals in per_person[1:]:
            common = intersect(common, [(s, e) for s, e, _, _ in intervals])
        for s, e in common:
            minutes = int((e - s).total_seconds() // 60)
            windows.append({"date": d.isoformat(), "start_utc": iso_utc(s), "end_utc": iso_utc(e), "minutes": minutes,
                            "fits_duration": minutes >= duration,
                            "local": {p["name"]: {"start": local_view(s, p["zone"]), "end": local_view(e, p["zone"])} for p in people}})
            # slots aligned to the step on the UTC clock
            epoch_min = int(s.timestamp() // 60)
            t_min = -(-epoch_min // step) * step
            while True:
                t = dt.datetime.fromtimestamp(t_min * 60, UTC)
                if t + dt.timedelta(minutes=duration) > e:
                    break
                margins = []
                for p, intervals in zip(people, per_person):
                    full = next((iv for iv in intervals if iv[0] <= t and t + dt.timedelta(minutes=duration) <= iv[1]), None)
                    if full is None:
                        break
                    margins.append(int(min((t - full[2]).total_seconds(), (full[3] - t - dt.timedelta(minutes=duration)).total_seconds()) // 60))
                else:
                    slots.append((min(margins), t, margins))
                t_min += step
        d += dt.timedelta(days=1)

    slots.sort(key=lambda x: (-x[0], x[1]))
    ranked = []
    for score, t, margins in slots[:max_slots]:
        end = t + dt.timedelta(minutes=duration)
        ranked.append({"start_utc": iso_utc(t), "end_utc": iso_utc(end), "score": score,
                       "local": {p["name"]: {"start": local_view(t, p["zone"]), "end": local_view(end, p["zone"]), "margin_minutes": m}
                                 for p, m in zip(people, margins)}})
    result = {
        "reference_tz": ref_name, "start_date": first.isoformat(), "end_date": last.isoformat(),
        "duration_minutes": duration, "step_minutes": step,
        "participants": [{"name": p["name"], "tz": p["tz_name"], "hours": f"{p['start'] // 60:02d}:{p['start'] % 60:02d}-{p['end'] // 60:02d}:{p['end'] % 60:02d}",
                          "workdays": sorted(p["workdays"])} for p in people],
        "windows": windows,
        "total_common_minutes": sum(w["minutes"] for w in windows),
        "slot_count": len(slots),
        "slots": ranked,
        "ranking": "score = the smallest margin, over participants, between the slot and the start or end of their working period; higher first, then earlier",
    }
    if not windows:
        result["note"] = "no common working time in the range"
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
