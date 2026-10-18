#!/usr/bin/env python3
"""Time-zone arithmetic over the IANA database (zoneinfo): convert a date-time between zones,
show a zone's offset and abbreviation on a date, or list a year's DST transitions. Local
times that do not exist (the spring-forward gap) or happen twice (the fall-back hour) are
reported, never silently shifted.

Input (JSON on stdin):
  operation  "convert" (default) | "offset" | "transitions"
  datetime   "2026-07-01T12:00" (local in from_zone) or with an offset/Z    convert, offset
  from_zone  "Europe/Madrid"                                                 convert
  to_zones   ["America/New_York", "UTC"]                                     convert
  zone       "Europe/Madrid"                                                 offset, transitions
  year       2026                                                            transitions
  fold       0 | 1   which of two repeated local times is meant (default 0, the first)
"""
import datetime as dt
import json
import pathlib
import sys
import zoneinfo


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def tzdata_version():
    for root in zoneinfo.TZPATH:
        path = pathlib.Path(root) / "tzdata.zi"
        try:
            first = path.read_text().splitlines()[0]
        except (OSError, IndexError):
            continue
        if first.startswith("# version"):
            return first.split()[-1]
    return None


def zone(name, field):
    if not isinstance(name, str) or not name:
        fail(f"{field} must be an IANA zone name such as Europe/Madrid")
    try:
        return zoneinfo.ZoneInfo(name)
    except (zoneinfo.ZoneInfoNotFoundError, ValueError):
        fail(f"{field}: unknown zone {name!r}")


def offset_text(delta):
    minutes = int(delta.total_seconds() // 60)
    sign = "+" if minutes >= 0 else "-"
    return f"{sign}{abs(minutes) // 60:02d}:{abs(minutes) % 60:02d}"


def describe(moment):
    return {"datetime": moment.isoformat(), "utc_offset": offset_text(moment.utcoffset()), "abbreviation": moment.tzname(),
            "dst": bool(moment.dst()), "weekday": moment.strftime("%A")}


def localise(text, tz, fold):
    try:
        naive = dt.datetime.fromisoformat(text.replace("Z", "+00:00"))
    except (TypeError, ValueError, AttributeError):
        fail("datetime must be ISO 8601, such as 2026-07-01T12:00")
    if naive.tzinfo is not None:
        return naive.astimezone(tz), None
    local = naive.replace(tzinfo=tz, fold=fold)
    back = local.astimezone(dt.timezone.utc).astimezone(tz)
    if back.replace(tzinfo=None) != naive:
        return local, "nonexistent: this local time falls in a daylight-saving gap"
    other = naive.replace(tzinfo=tz, fold=1 - fold)
    if other.utcoffset() != local.utcoffset():
        return local, f"ambiguous: this local time happens twice; fold={fold} chose the {'first' if fold == 0 else 'second'}"
    return local, None


def transitions(tz, year):
    start = dt.datetime(year, 1, 1, tzinfo=dt.timezone.utc)
    hour, out, previous = dt.timedelta(hours=1), [], start.astimezone(tz).utcoffset()
    t = start
    while t.year == year:
        nxt = t + hour
        offset = nxt.astimezone(tz).utcoffset()
        if offset != previous:
            lo, hi = t, nxt
            while hi - lo > dt.timedelta(seconds=1):
                mid = lo + (hi - lo) / 2
                if mid.astimezone(tz).utcoffset() == previous:
                    lo = mid
                else:
                    hi = mid
            hi = hi.replace(microsecond=0)
            before, after = (hi - dt.timedelta(seconds=1)).astimezone(tz), hi.astimezone(tz)
            out.append({"utc": hi.strftime("%Y-%m-%dT%H:%M:%SZ"), "local_before": (before + dt.timedelta(seconds=1)).replace(tzinfo=None).isoformat(),
                        "local_after": after.replace(tzinfo=None).isoformat(), "offset_before": offset_text(previous),
                        "offset_after": offset_text(offset), "abbreviation_after": after.tzname()})
            previous = offset
        t = nxt
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    operation = request.get("operation", "convert")
    fold = request.get("fold", 0)
    if fold not in (0, 1):
        fail("fold must be 0 or 1")
    result = {"operation": operation, "tzdata": tzdata_version()}
    if operation == "convert":
        source = zone(request.get("from_zone"), "from_zone")
        targets = request.get("to_zones")
        if not isinstance(targets, list) or not targets:
            fail("to_zones must be a non-empty list of zone names")
        moment, warning = localise(request.get("datetime"), source, fold)
        result["from"] = {"zone": request["from_zone"], **describe(moment)}
        result["to"] = [{"zone": name, **describe(moment.astimezone(zone(name, "to_zones[]")))} for name in targets]
        if warning:
            result["warning"] = warning
    elif operation == "offset":
        tz = zone(request.get("zone"), "zone")
        moment, warning = localise(request.get("datetime"), tz, fold)
        result.update({"zone": request["zone"], **describe(moment)})
        if warning:
            result["warning"] = warning
    elif operation == "transitions":
        tz = zone(request.get("zone"), "zone")
        year = request.get("year")
        if not isinstance(year, int) or isinstance(year, bool) or not 1900 <= year <= 2100:
            fail("year must be an integer from 1900 to 2100")
        result.update({"zone": request["zone"], "year": year, "transitions": transitions(tz, year)})
    else:
        fail("operation must be convert, offset or transitions")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
