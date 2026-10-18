#!/usr/bin/env python3
"""Write an RFC 5545 iCalendar (VCALENDAR) with one event, optionally recurring (RRULE built
from structured fields), with alarms, organizer and attendees. Text values are escaped
(backslash, semicolon, comma, newline), lines folded at 75 octets, CRLF line ends. Times are
floating, UTC or local to an IANA TZID (with a VTIMEZONE derived from zoneinfo). The UID is
taken from the input or derived from a hash of it, and DTSTAMP is taken from the input or fixed,
so the same input always gives the same file.

Input (JSON on stdin): see manifest.json. Main fields:
  summary, start ("YYYY-MM-DD" all day, "YYYY-MM-DDTHH:MM[:SS]" with optional Z or offset),
  end | duration_minutes, tzid, utc, description, location, rrule {freq, interval, count, until,
  byday, bymonthday, bymonth, wkst}, exdates, alarms [{minutes_before, action, description}],
  organizer {email, name}, attendees [{email, name, role, partstat, rsvp, cutype}], uid, dtstamp
"""
import datetime as dt
import hashlib
import json
import re
import sys

try:
    import zoneinfo
except ImportError:  # pragma: no cover
    zoneinfo = None

FIXED_DTSTAMP = "19700101T000000Z"
WEEKDAY_CODES = ["MO", "TU", "WE", "TH", "FR", "SA", "SU"]
UTC = dt.timezone.utc


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


# ---------- RFC 5545 text rules ----------

def escape_text(value):
    """RFC 5545 3.3.11: backslash, semicolon and comma are escaped; a newline becomes \\n."""
    value = value.replace("\\", "\\\\").replace(";", "\\;").replace(",", "\\,")
    return value.replace("\r\n", "\\n").replace("\r", "\\n").replace("\n", "\\n")


def param_value(value):
    """RFC 5545 3.2: a parameter value with ; : or , is quoted; DQUOTE and controls are removed."""
    value = "".join(c for c in value if c != '"' and (c == "\t" or ord(c) >= 32) and ord(c) != 127)
    return f'"{value}"' if any(c in value for c in ";:,") else value


def fold(line):
    """RFC 5545 3.1: lines of at most 75 octets; a continuation starts with one space, which
    counts; a UTF-8 sequence is never split."""
    out, current, size, limit = [], [], 0, 75
    for char in line:
        width = len(char.encode("utf-8"))
        if size + width > limit:
            out.append("".join(current))
            current, size, limit = [" "], 1, 75
        current.append(char)
        size += width
    out.append("".join(current))
    return out


# ---------- input parsing ----------

MOMENT = re.compile(r"^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2})(?::(\d{2}))?(Z|[+-]\d{2}:?\d{2})?)?$")


def parse_moment(value, field):
    """Returns ("date", date) | ("naive", datetime) | ("aware", datetime)."""
    if not isinstance(value, str):
        fail(f"{field} must be a string YYYY-MM-DD or YYYY-MM-DDTHH:MM[:SS][Z|+HH:MM]")
    m = MOMENT.match(value.strip())
    if not m:
        fail(f"{field} must be YYYY-MM-DD or YYYY-MM-DDTHH:MM[:SS][Z|+HH:MM]")
    y, mo, d, h, mi, s, off = m.groups()
    try:
        if h is None:
            return "date", dt.date(int(y), int(mo), int(d))
        moment = dt.datetime(int(y), int(mo), int(d), int(h), int(mi), int(s or 0))
    except ValueError as error:
        fail(f"{field} is not a valid date or time: {error}")
    if off is None:
        return "naive", moment
    if off == "Z":
        return "aware", moment.replace(tzinfo=UTC)
    sign = 1 if off[0] == "+" else -1
    digits = off[1:].replace(":", "")
    if int(digits[:2]) > 23 or int(digits[2:]) > 59:
        fail(f"{field} has an invalid offset")
    return "aware", moment.replace(tzinfo=dt.timezone(sign * dt.timedelta(hours=int(digits[:2]), minutes=int(digits[2:]))))


def text_field(request, key, limit=10000, required=False):
    value = request.get(key)
    if value is None:
        if required:
            fail(f"{key} is required")
        return None
    if not isinstance(value, str) or (required and not value.strip()):
        fail(f"{key} must be a {'non-empty ' if required else ''}string")
    if len(value) > limit:
        fail(f"{key} must be at most {limit} characters")
    return value


EMAIL = re.compile(r"^[^\s@:;,<>\"]+@[^\s@:;,<>\"]+$")


def address(item, field):
    if not isinstance(item, dict):
        fail(f"{field} must be an object with email")
    email = item.get("email")
    if not isinstance(email, str) or not EMAIL.match(email):
        fail(f"{field}.email must be an e-mail address")
    name = item.get("name")
    if name is not None and not isinstance(name, str):
        fail(f"{field}.name must be a string")
    return email, name


# ---------- time zones ----------

def get_zone(tzid):
    if zoneinfo is None:
        fail("this Python has no zoneinfo module; tzid needs Python 3.9+")
    try:
        return zoneinfo.ZoneInfo(tzid)
    except (zoneinfo.ZoneInfoNotFoundError, ValueError, TypeError):
        fail(f"tzid {tzid!r} is not a known IANA time zone")


def offset_text(delta):
    seconds = int(delta.total_seconds())
    sign = "+" if seconds >= 0 else "-"
    seconds = abs(seconds)
    h, rest = divmod(seconds, 3600)
    m, s = divmod(rest, 60)
    return f"{sign}{h:02d}{m:02d}" + (f"{s:02d}" if s else "")


def transitions(zone, year):
    """UTC instants in a year where the zone's UTC offset changes, with offsets and names."""
    found = []
    t = dt.datetime(year, 1, 1, tzinfo=UTC)
    end = dt.datetime(year + 1, 1, 1, tzinfo=UTC)
    step = dt.timedelta(hours=6)
    while t < end:
        nxt = t + step
        if t.astimezone(zone).utcoffset() != nxt.astimezone(zone).utcoffset():
            lo, hi = t, nxt
            while hi - lo > dt.timedelta(seconds=1):
                mid = lo + (hi - lo) / 2
                mid = mid.replace(microsecond=0)
                if mid == lo:
                    break
                if mid.astimezone(zone).utcoffset() == lo.astimezone(zone).utcoffset():
                    lo = mid
                else:
                    hi = mid
            after = hi.astimezone(zone)
            found.append({"utc": hi, "from": lo.astimezone(zone).utcoffset(), "to": after.utcoffset(),
                          "name": after.tzname(), "dst": bool(after.dst())})
        t = nxt
    return found


def yearly_rule(local):
    last = (local.replace(day=28) + dt.timedelta(days=4)).replace(day=1) - dt.timedelta(days=1)
    n = -1 if local.day + 7 > last.day else (local.day - 1) // 7 + 1
    return f"FREQ=YEARLY;BYMONTH={local.month};BYDAY={n}{WEEKDAY_CODES[local.weekday()]}"


def vtimezone(tzid, zone, year, warnings):
    lines = ["BEGIN:VTIMEZONE", f"TZID:{tzid}"]
    prev, this = transitions(zone, year - 1), transitions(zone, year)
    if not prev and not this:
        moment = dt.datetime(year, 1, 1, tzinfo=UTC).astimezone(zone)
        off = offset_text(moment.utcoffset())
        lines += ["BEGIN:STANDARD", "DTSTART:19700101T000000", f"TZOFFSETFROM:{off}", f"TZOFFSETTO:{off}",
                  f"TZNAME:{param_safe(moment.tzname())}", "END:STANDARD"]
    else:
        rules_prev = [(yearly_rule((t["utc"] + t["from"]).replace(tzinfo=None)), t["dst"]) for t in prev]
        rules_this = [(yearly_rule((t["utc"] + t["from"]).replace(tzinfo=None)), t["dst"]) for t in this]
        recurring = len(prev) == 2 and rules_prev == rules_this
        base = prev if prev else this
        if prev and not recurring:
            base = prev + this
            warnings.append(f"{tzid} changed its rules around {year}: VTIMEZONE lists the transitions of {year - 1} and {year} without a yearly rule")
        for t in base:
            local = (t["utc"] + t["from"]).replace(tzinfo=None)
            kind = "DAYLIGHT" if t["dst"] else "STANDARD"
            lines += [f"BEGIN:{kind}", f"DTSTART:{local.strftime('%Y%m%dT%H%M%S')}"]
            if recurring:
                lines.append(f"RRULE:{yearly_rule(local)}")
            lines += [f"TZOFFSETFROM:{offset_text(t['from'])}", f"TZOFFSETTO:{offset_text(t['to'])}",
                      f"TZNAME:{param_safe(t['name'])}", f"END:{kind}"]
    lines.append("END:VTIMEZONE")
    return lines


def param_safe(text):
    return escape_text(text or "")


# ---------- main ----------

def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"summary", "start", "end", "duration_minutes", "tzid", "utc", "description", "location", "url", "status",
             "transparency", "categories", "rrule", "exdates", "alarms", "organizer", "attendees", "uid", "dtstamp",
             "sequence", "prodid", "method", "include_vtimezone"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    warnings = []
    summary = text_field(request, "summary", 2000, required=True)
    tzid = request.get("tzid")
    if tzid is not None and (not isinstance(tzid, str) or not tzid.strip()):
        fail("tzid must be an IANA time zone name such as Europe/Madrid")
    zone = get_zone(tzid) if tzid else None
    utc = request.get("utc", False)
    if not isinstance(utc, bool):
        fail("utc must be true or false")

    kind, start = parse_moment(request.get("start"), "start")
    if kind == "date" and (utc or tzid):
        fail("an all-day event (start YYYY-MM-DD) takes no tzid or utc")

    def resolve(kind_value, field):
        """Bring a parsed moment to the event's mode: date, floating, utc or tzid."""
        k, v = kind_value
        if mode == "date":
            if k != "date":
                fail(f"{field} must be a date YYYY-MM-DD like start")
            return v
        if k == "date":
            fail(f"{field} must be a date-time like start")
        if mode == "floating":
            if k == "aware":
                fail(f"{field} has an offset but start is floating (no tzid, no offset)")
            return v
        if k == "naive":
            v = v.replace(tzinfo=zone) if zone else v.replace(tzinfo=UTC)
        return v.astimezone(UTC) if mode == "utc" else v.astimezone(zone)

    if kind == "date":
        mode = "date"
    elif utc:
        if kind == "naive" and not zone:
            fail("utc needs the start's time zone: give tzid or an offset in start")
        mode = "utc"
    elif zone is not None:
        mode = "tzid"
    elif kind == "aware":
        mode = "utc"
    else:
        mode = "floating"
    start = resolve((kind, start), "start")
    if mode in ("tzid", "utc") and not 1900 <= start.year <= 2200:
        fail("a start with a time zone must fall between the years 1900 and 2200")

    def fmt(value):
        if mode == "date":
            return value.strftime("%Y%m%d")
        if mode == "utc":
            return value.astimezone(UTC).strftime("%Y%m%dT%H%M%SZ")
        return value.strftime("%Y%m%dT%H%M%S")

    def prop(name, value):
        if mode == "date":
            return f"{name};VALUE=DATE:{fmt(value)}"
        if mode == "tzid":
            return f"{name};TZID={param_value(tzid)}:{fmt(value)}"
        return f"{name}:{fmt(value)}"

    event = []
    uid = request.get("uid")
    if uid is None:
        canonical = json.dumps({k: v for k, v in request.items() if k != "uid"}, sort_keys=True, ensure_ascii=False)
        uid = hashlib.sha256(canonical.encode("utf-8")).hexdigest()[:32] + "@ics-generate.invalid"
        uid_rule = "derived: first 32 hex digits of SHA-256 of the input"
    else:
        if not isinstance(uid, str) or not uid.strip() or len(uid) > 255:
            fail("uid must be a non-empty string of at most 255 characters")
        uid_rule = "given"
    dtstamp = request.get("dtstamp")
    if dtstamp is None:
        stamp = FIXED_DTSTAMP
    else:
        k, v = parse_moment(dtstamp, "dtstamp")
        if k == "date":
            fail("dtstamp must be a date-time")
        stamp = (v if k == "aware" else v.replace(tzinfo=UTC)).astimezone(UTC).strftime("%Y%m%dT%H%M%SZ")
    event += [f"UID:{escape_text(uid)}", f"DTSTAMP:{stamp}", prop("DTSTART", start)]

    if request.get("end") is not None and request.get("duration_minutes") is not None:
        fail("give end or duration_minutes, not both")
    if request.get("end") is not None:
        end = resolve(parse_moment(request["end"], "end"), "end")
        if end <= start:
            fail("end must be after start")
        event.append(prop("DTEND", end))
    elif request.get("duration_minutes") is not None:
        minutes = request["duration_minutes"]
        if not isinstance(minutes, int) or isinstance(minutes, bool) or minutes <= 0 or minutes > 525600:
            fail("duration_minutes must be a positive integer (at most a year)")
        if mode == "date":
            fail("an all-day event takes end (a date), not duration_minutes")
        event.append(f"DURATION:{duration_text(minutes)}")
    elif mode == "date":
        event.append(prop("DTEND", start + dt.timedelta(days=1)))
    else:
        warnings.append("no end or duration_minutes: the event is an instant (RFC 5545 3.6.1)")

    event.append(f"SUMMARY:{escape_text(summary)}")
    for key, name in (("description", "DESCRIPTION"), ("location", "LOCATION")):
        value = text_field(request, key)
        if value is not None:
            event.append(f"{name}:{escape_text(value)}")
    url = text_field(request, "url", 2000)
    if url is not None:
        if any(c in url for c in " \r\n\t"):
            fail("url must not contain spaces or line breaks")
        event.append(f"URL:{url}")
    status = request.get("status")
    if status is not None:
        if status not in ("TENTATIVE", "CONFIRMED", "CANCELLED"):
            fail("status must be TENTATIVE, CONFIRMED or CANCELLED")
        event.append(f"STATUS:{status}")
    transp = request.get("transparency")
    if transp is not None:
        if transp not in ("OPAQUE", "TRANSPARENT"):
            fail("transparency must be OPAQUE or TRANSPARENT")
        event.append(f"TRANSP:{transp}")
    sequence = request.get("sequence")
    if sequence is not None:
        if not isinstance(sequence, int) or isinstance(sequence, bool) or sequence < 0:
            fail("sequence must be a non-negative integer")
        event.append(f"SEQUENCE:{sequence}")
    categories = request.get("categories")
    if categories is not None:
        if not isinstance(categories, list) or not all(isinstance(c, str) and c for c in categories):
            fail("categories must be a list of non-empty strings")
        if categories:
            event.append("CATEGORIES:" + ",".join(escape_text(c) for c in categories))

    rrule = request.get("rrule")
    if rrule is not None:
        event.append("RRULE:" + build_rrule(rrule, mode, zone, start))
    exdates = request.get("exdates")
    if exdates is not None:
        if not isinstance(exdates, list) or not exdates:
            fail("exdates must be a non-empty list")
        if rrule is None:
            fail("exdates need an rrule")
        values = [fmt(resolve(parse_moment(x, f"exdates[{i}]"), f"exdates[{i}]")) for i, x in enumerate(exdates)]
        head = "EXDATE;VALUE=DATE" if mode == "date" else f"EXDATE;TZID={param_value(tzid)}" if mode == "tzid" else "EXDATE"
        event.append(f"{head}:{','.join(values)}")

    organizer = request.get("organizer")
    if organizer is not None:
        email, name = address(organizer, "organizer")
        event.append("ORGANIZER" + (f";CN={param_value(name)}" if name else "") + f":mailto:{email}")
    attendees = request.get("attendees")
    if attendees is not None:
        if not isinstance(attendees, list):
            fail("attendees must be a list")
        for i, item in enumerate(attendees):
            event.append(attendee(item, f"attendees[{i}]"))

    alarms = request.get("alarms")
    if alarms is not None:
        if not isinstance(alarms, list):
            fail("alarms must be a list")
        for i, alarm in enumerate(alarms):
            event += build_alarm(alarm, f"alarms[{i}]", summary)

    prodid = text_field(request, "prodid", 200) or "-//AXLR catalog//ics_generate 1.0//EN"
    calendar = ["BEGIN:VCALENDAR", "VERSION:2.0", f"PRODID:{escape_text(prodid)}", "CALSCALE:GREGORIAN"]
    method = request.get("method")
    if method is not None:
        if method not in ("PUBLISH", "REQUEST", "CANCEL"):
            fail("method must be PUBLISH, REQUEST or CANCEL")
        calendar.append(f"METHOD:{method}")
    include_tz = request.get("include_vtimezone", True)
    if not isinstance(include_tz, bool):
        fail("include_vtimezone must be true or false")
    if mode == "tzid":
        if include_tz:
            calendar += vtimezone(tzid, zone, start.year, warnings)
        else:
            warnings.append("include_vtimezone is false: RFC 5545 requires a VTIMEZONE for each TZID; most clients accept an IANA TZID without it")
    calendar += ["BEGIN:VEVENT", *event, "END:VEVENT", "END:VCALENDAR"]
    physical = [part for line in calendar for part in fold(line)]
    ics = "\r\n".join(physical) + "\r\n"
    result = {"ics": ics, "uid": uid, "uid_rule": uid_rule, "dtstamp": stamp,
              "dtstamp_rule": "given" if dtstamp is not None else "fixed placeholder (no clock is read)",
              "time_mode": mode, "content_lines": len(calendar), "physical_lines": len(physical),
              "octets": len(ics.encode("utf-8")), "warnings": warnings}
    print(json.dumps(result, ensure_ascii=False))


def duration_text(minutes):
    days, rest = divmod(minutes, 1440)
    hours, mins = divmod(rest, 60)
    out = "P" + (f"{days}D" if days else "")
    if hours or mins:
        out += "T" + (f"{hours}H" if hours else "") + (f"{mins}M" if mins else "")
    return out


def attendee(item, field):
    email, name = address(item, field)
    allowed = {"email", "name", "role", "partstat", "rsvp", "cutype"}
    extra = sorted(set(item) - allowed)
    if extra:
        fail(f"{field} has unknown field(s): {', '.join(extra)}")
    role = item.get("role", "REQ-PARTICIPANT")
    if role not in ("CHAIR", "REQ-PARTICIPANT", "OPT-PARTICIPANT", "NON-PARTICIPANT"):
        fail(f"{field}.role must be CHAIR, REQ-PARTICIPANT, OPT-PARTICIPANT or NON-PARTICIPANT")
    partstat = item.get("partstat", "NEEDS-ACTION")
    if partstat not in ("NEEDS-ACTION", "ACCEPTED", "DECLINED", "TENTATIVE", "DELEGATED"):
        fail(f"{field}.partstat must be NEEDS-ACTION, ACCEPTED, DECLINED, TENTATIVE or DELEGATED")
    rsvp = item.get("rsvp", True)
    if not isinstance(rsvp, bool):
        fail(f"{field}.rsvp must be true or false")
    cutype = item.get("cutype")
    if cutype is not None and cutype not in ("INDIVIDUAL", "GROUP", "RESOURCE", "ROOM", "UNKNOWN"):
        fail(f"{field}.cutype must be INDIVIDUAL, GROUP, RESOURCE, ROOM or UNKNOWN")
    params = (f";CN={param_value(name)}" if name else "") + (f";CUTYPE={cutype}" if cutype else "")
    params += f";ROLE={role};PARTSTAT={partstat};RSVP={'TRUE' if rsvp else 'FALSE'}"
    return f"ATTENDEE{params}:mailto:{email}"


def build_alarm(alarm, field, summary):
    if not isinstance(alarm, dict):
        fail(f"{field} must be an object")
    extra = sorted(set(alarm) - {"minutes_before", "action", "description"})
    if extra:
        fail(f"{field} has unknown field(s): {', '.join(extra)}")
    minutes = alarm.get("minutes_before")
    if not isinstance(minutes, int) or isinstance(minutes, bool) or not -525600 <= minutes <= 525600:
        fail(f"{field}.minutes_before must be an integer (negative means after the start)")
    action = alarm.get("action", "DISPLAY")
    if action not in ("DISPLAY", "AUDIO"):
        fail(f"{field}.action must be DISPLAY or AUDIO")
    trigger = ("-" if minutes > 0 else "") + (duration_text(abs(minutes)) if minutes else "PT0M")
    lines = ["BEGIN:VALARM", f"ACTION:{action}", f"TRIGGER:{trigger}"]
    if action == "DISPLAY":
        text = alarm.get("description", summary)
        if not isinstance(text, str) or not text:
            fail(f"{field}.description must be a non-empty string")
        lines.append(f"DESCRIPTION:{escape_text(text)}")
    lines.append("END:VALARM")
    return lines


BYDAY = re.compile(r"^([+-]?\d{1,2})?(MO|TU|WE|TH|FR|SA|SU)$")


def build_rrule(rule, mode, zone, start):
    if not isinstance(rule, dict):
        fail("rrule must be an object")
    extra = sorted(set(rule) - {"freq", "interval", "count", "until", "byday", "bymonthday", "bymonth", "wkst"})
    if extra:
        fail(f"rrule has unknown field(s): {', '.join(extra)}")
    freq = rule.get("freq")
    if freq not in ("DAILY", "WEEKLY", "MONTHLY", "YEARLY"):
        fail("rrule.freq must be DAILY, WEEKLY, MONTHLY or YEARLY")
    parts = [f"FREQ={freq}"]
    interval = rule.get("interval")
    if interval is not None:
        if not isinstance(interval, int) or isinstance(interval, bool) or not 1 <= interval <= 1000:
            fail("rrule.interval must be an integer from 1 to 1000")
        if interval != 1:
            parts.append(f"INTERVAL={interval}")
    if rule.get("count") is not None and rule.get("until") is not None:
        fail("rrule takes count or until, not both (RFC 5545 3.3.10)")
    count = rule.get("count")
    if count is not None:
        if not isinstance(count, int) or isinstance(count, bool) or not 1 <= count <= 100000:
            fail("rrule.count must be a positive integer")
        parts.append(f"COUNT={count}")
    until = rule.get("until")
    if until is not None:
        k, v = parse_moment(until, "rrule.until")
        if mode == "date":
            if k != "date":
                fail("rrule.until must be a date YYYY-MM-DD for an all-day event")
            value = v.strftime("%Y%m%d")
        else:
            if k == "date":
                v, k = dt.datetime.combine(v, dt.time(23, 59, 59)), "naive"
            if mode == "floating":
                if k == "aware":
                    fail("rrule.until has an offset but start is floating")
                value = v.strftime("%Y%m%dT%H%M%S")
            else:
                if k == "naive":
                    v = v.replace(tzinfo=zone or UTC)
                value = v.astimezone(UTC).strftime("%Y%m%dT%H%M%SZ")
        parts.append(f"UNTIL={value}")
    wkst = rule.get("wkst")
    if wkst is not None:
        if wkst not in WEEKDAY_CODES:
            fail("rrule.wkst must be MO, TU, WE, TH, FR, SA or SU")
        parts.append(f"WKST={wkst}")
    byday = rule.get("byday")
    if byday is not None:
        if not isinstance(byday, list) or not byday:
            fail("rrule.byday must be a non-empty list like [\"MO\", \"WE\"] or [\"1FR\", \"-1SU\"]")
        for item in byday:
            m = BYDAY.match(item) if isinstance(item, str) else None
            if not m:
                fail(f"rrule.byday item {item!r} must be a weekday code, optionally with an ordinal: MO, 1FR, -1SU")
            if m.group(1):
                n = int(m.group(1))
                if freq not in ("MONTHLY", "YEARLY"):
                    fail("an ordinal in rrule.byday (like 1FR) needs freq MONTHLY or YEARLY")
                if n == 0 or abs(n) > (5 if freq == "MONTHLY" else 53):
                    fail(f"rrule.byday ordinal {n} out of range")
        parts.append("BYDAY=" + ",".join(byday))
    for key, name, low, high in (("bymonthday", "BYMONTHDAY", 1, 31), ("bymonth", "BYMONTH", 1, 12)):
        values = rule.get(key)
        if values is None:
            continue
        if not isinstance(values, list) or not values:
            fail(f"rrule.{key} must be a non-empty list of integers")
        for v in values:
            ok = isinstance(v, int) and not isinstance(v, bool) and low <= abs(v) <= high and (v > 0 or key == "bymonthday")
            if not ok:
                fail(f"rrule.{key} value {v!r} out of range")
        parts.append(f"{name}=" + ",".join(str(v) for v in values))
    return ";".join(parts)


if __name__ == "__main__":
    try:
        main()
    except OverflowError:
        fail("a date is out of the supported range (years 1 to 9999)")
