#!/usr/bin/env python3
"""Parse a date, time, date-time or duration written in ISO 8601 or in a small set of English
and Spanish natural-language forms, into ISO 8601 output and the name of the rule that matched.
Relative forms ("tomorrow", "next Monday", "in 3 days", "hace 3 días") are computed from the
reference given in the input; the clock is never read.

Input (JSON on stdin):
  text        the expression
  reference   "YYYY-MM-DD" or "YYYY-MM-DDTHH:MM[:SS]"; required for relative forms
  date_order  "DMY" | "MDY": how to read 18/10/2026 or 10/18/2026; required when ambiguous
"""
import datetime as dt
import json
import re
import sys
import unicodedata
from decimal import Decimal, InvalidOperation


class ParseError(Exception):
    pass


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def fold(text):
    """Lowercase, drop accents (mañana -> manana), collapse spaces."""
    text = unicodedata.normalize("NFD", text.lower())
    text = "".join(c for c in text if not unicodedata.combining(c))
    return re.sub(r"\s+", " ", text).strip()


MONTHS = {
    "january": 1, "jan": 1, "february": 2, "feb": 2, "march": 3, "mar": 3, "april": 4, "apr": 4, "may": 5,
    "june": 6, "jun": 6, "july": 7, "jul": 7, "august": 8, "aug": 8, "september": 9, "sept": 9, "sep": 9,
    "october": 10, "oct": 10, "november": 11, "nov": 11, "december": 12, "dec": 12,
    "enero": 1, "ene": 1, "febrero": 2, "marzo": 3, "abril": 4, "abr": 4, "mayo": 5, "junio": 6, "julio": 7,
    "agosto": 8, "ago": 8, "septiembre": 9, "setiembre": 9, "set": 9, "octubre": 10, "noviembre": 11,
    "diciembre": 12, "dic": 12,
}
WEEKDAYS = {
    "monday": 0, "mon": 0, "tuesday": 1, "tue": 1, "tues": 1, "wednesday": 2, "wed": 2, "thursday": 3, "thu": 3,
    "thurs": 3, "friday": 4, "fri": 4, "saturday": 5, "sat": 5, "sunday": 6, "sun": 6,
    "lunes": 0, "martes": 1, "miercoles": 2, "jueves": 3, "viernes": 4, "sabado": 5, "domingo": 6,
}
WEEKDAY_NAMES = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]
NUMBERS = {
    "a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
    "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
    "un": 1, "una": 1, "uno": 1, "dos": 2, "tres": 3, "cuatro": 4, "cinco": 5, "seis": 6, "siete": 7, "ocho": 8,
    "nueve": 9, "diez": 10, "once": 11, "doce": 12,
}
UNITS = {
    "second": "seconds", "seconds": "seconds", "sec": "seconds", "secs": "seconds", "segundo": "seconds", "segundos": "seconds",
    "minute": "minutes", "minutes": "minutes", "min": "minutes", "mins": "minutes", "minuto": "minutes", "minutos": "minutes",
    "hour": "hours", "hours": "hours", "hora": "hours", "horas": "hours",
    "day": "days", "days": "days", "dia": "days", "dias": "days",
    "week": "weeks", "weeks": "weeks", "semana": "weeks", "semanas": "weeks",
    "month": "months", "months": "months", "mes": "months", "meses": "months",
    "year": "years", "years": "years", "ano": "years", "anos": "years",
}
KEYWORDS = {
    "today": 0, "hoy": 0, "tomorrow": 1, "manana": 1, "yesterday": -1, "ayer": -1,
    "day after tomorrow": 2, "the day after tomorrow": 2, "pasado manana": 2,
    "day before yesterday": -2, "the day before yesterday": -2, "anteayer": -2, "antes de ayer": -2, "antier": -2,
}
NUM = r"(\d+|" + "|".join(sorted(NUMBERS, key=len, reverse=True)) + r")"
UNIT = r"(" + "|".join(sorted(UNITS, key=len, reverse=True)) + r")"
WEEKDAY = r"(" + "|".join(sorted(WEEKDAYS, key=len, reverse=True)) + r")"
MONTH = r"(" + "|".join(sorted(MONTHS, key=len, reverse=True)) + r")\.?"


def number(token):
    return int(token) if token.isdigit() else NUMBERS[token]


def make_date(y, m, d):
    try:
        return dt.date(int(y), int(m), int(d))
    except ValueError as error:
        raise ParseError(f"not a valid date: {error}")


def add_months(day, months):
    """Calendar month arithmetic, the day clamped to the month's end (31 Jan + 1 month = 28/29 Feb)."""
    total = day.year * 12 + day.month - 1 + months
    year, month = divmod(total, 12)
    month += 1
    if not 1 <= year <= 9999:
        raise ParseError("result is out of the supported year range")
    last = [31, 29 if (year % 4 == 0 and (year % 100 != 0 or year % 400 == 0)) else 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1]
    return day.replace(year=year, month=month, day=min(day.day, last))


def shift(ref, amount, unit):
    """ref is a date or datetime; months and years by calendar, the rest by exact length."""
    try:
        if unit in ("months", "years"):
            return add_months(ref, amount * (12 if unit == "years" else 1))
        if unit in ("days", "weeks"):
            return ref + dt.timedelta(days=amount * (7 if unit == "weeks" else 1))
        base = ref if isinstance(ref, dt.datetime) else dt.datetime.combine(ref, dt.time())
        return base + dt.timedelta(**{unit: amount})
    except OverflowError:
        raise ParseError("result is out of the supported date range")


def parse_offset(text):
    if text is None:
        return None
    if text == "z":
        return dt.timezone.utc
    sign = 1 if text[0] == "+" else -1
    digits = text[1:].replace(":", "")
    hours, minutes = int(digits[:2]), int(digits[2:] or 0)
    if hours > 23 or minutes > 59:
        raise ParseError("time zone offset out of range")
    return dt.timezone(sign * dt.timedelta(hours=hours, minutes=minutes))


def make_time(h, mi, s="0", frac=None, offset=None):
    micro = int((frac or "0")[:6].ljust(6, "0"))
    try:
        return dt.time(int(h), int(mi), int(s or 0), micro, tzinfo=parse_offset(offset))
    except ValueError as error:
        raise ParseError(f"not a valid time: {error}")


# ---------- ISO 8601 ----------

DUR_NUM = r"(\d+(?:[.,]\d+)?)"
ISO_DURATION = re.compile(r"^(-)?p(?:" + DUR_NUM + r"y)?(?:" + DUR_NUM + r"m)?(?:" + DUR_NUM + r"w)?(?:" + DUR_NUM + r"d)?"
                          r"(?:t(?:" + DUR_NUM + r"h)?(?:" + DUR_NUM + r"m)?(?:" + DUR_NUM + r"s)?)?$")
ISO_EXT = re.compile(r"^(\d{4})-(\d{2})-(\d{2})(?:[t ](\d{2}):(\d{2})(?::(\d{2})(?:[.,](\d+))?)?(z|[+-]\d{2}(?::?\d{2})?)?)?$")
ISO_BASIC = re.compile(r"^(\d{4})(\d{2})(\d{2})(?:t(\d{2})(\d{2})(?:(\d{2})(?:[.,](\d+))?)?(z|[+-]\d{2}(?:\d{2})?)?)?$")
ISO_WEEK = re.compile(r"^(\d{4})-?w(\d{2})(?:-?([1-7]))?$")
ISO_ORDINAL = re.compile(r"^(\d{4})-(\d{3})$")
ISO_TIME = re.compile(r"^t?(\d{2}):(\d{2})(?::(\d{2})(?:[.,](\d+))?)?(z|[+-]\d{2}(?::?\d{2})?)?$")


def duration(match, ref):
    sign, *parts = match.groups()
    names = ["years", "months", "weeks", "days", "hours", "minutes", "seconds"]
    if all(p is None for p in parts):
        raise ParseError("an ISO 8601 duration needs at least one component")
    if match.group(0).endswith("t"):
        raise ParseError("an ISO 8601 duration with T needs an hour, minute or second component")
    values = {}
    present = [i for i, p in enumerate(parts) if p is not None]
    for i in present:
        text = parts[i].replace(",", ".")
        if "." in text and i != present[-1]:
            raise ParseError("only the smallest component of an ISO 8601 duration may have a fraction")
        values[names[i]] = Decimal(text)
    negative = sign == "-"
    out = {"type": "duration", "value": match.group(0).upper(), "rule": "iso8601_duration",
           "components": {k: str(v) for k, v in values.items()}, "negative": negative}
    fixed = {"weeks": 604800, "days": 86400, "hours": 3600, "minutes": 60, "seconds": 1}
    if not values.get("years") and not values.get("months"):
        total = sum(values.get(k, Decimal(0)) * f for k, f in fixed.items())
        total = -total if negative else total
        out["total_seconds"] = format(total.normalize(), "f") if total != total.to_integral() else str(total.to_integral())
        out["total_seconds_note"] = "days counted as 24 hours (nominal)"
    else:
        out["total_seconds_note"] = "not fixed: years and months have no constant length"
    if ref is not None:
        for k in ("years", "months"):
            if k in values and values[k] != values[k].to_integral():
                raise ParseError(f"cannot add fractional {k} to a date: they have no fixed length")
        s = -1 if negative else 1
        end = ref
        end = shift(end, s * int(values.get("years", 0)), "years")
        end = shift(end, s * int(values.get("months", 0)), "months")
        rest = sum(values.get(k, Decimal(0)) * f for k, f in fixed.items())
        if rest:
            seconds = rest * s
            if seconds == seconds.to_integral() and seconds % 86400 == 0 and not isinstance(end, dt.datetime):
                end = end + dt.timedelta(days=int(seconds / 86400))
            else:
                base = end if isinstance(end, dt.datetime) else dt.datetime.combine(end, dt.time())
                end = base + dt.timedelta(seconds=float(seconds))
        out["end"] = end.isoformat()
    return out


def iso(text, ref):
    m = ISO_DURATION.match(text)
    if m and text != "p":
        return duration(m, ref)
    for pattern, rule in ((ISO_EXT, "iso8601_extended"), (ISO_BASIC, "iso8601_basic")):
        m = pattern.match(text)
        if m:
            y, mo, d, h, mi, s, frac, off = m.groups()
            day = make_date(y, mo, d)
            if h is None:
                return {"type": "date", "value": day.isoformat(), "rule": rule + "_date"}
            moment = dt.datetime.combine(day, make_time(h, mi, s, frac, off))
            return {"type": "datetime", "value": moment.isoformat(), "rule": rule + "_datetime"}
    m = ISO_WEEK.match(text)
    if m:
        y, w, d = m.groups()
        try:
            day = dt.date.fromisocalendar(int(y), int(w), int(d or 1))
        except ValueError as error:
            raise ParseError(f"not a valid ISO week date: {error}")
        return {"type": "date", "value": day.isoformat(), "rule": "iso8601_week_date"}
    m = ISO_ORDINAL.match(text)
    if m:
        y, n = int(m.group(1)), int(m.group(2))
        day = dt.date(y, 1, 1) + dt.timedelta(days=n - 1)
        if not 1 <= n or day.year != y:
            raise ParseError("ordinal day out of range for the year")
        return {"type": "date", "value": day.isoformat(), "rule": "iso8601_ordinal_date"}
    m = ISO_TIME.match(text)
    if m:
        h, mi, s, frac, off = m.groups()
        return {"type": "time", "value": make_time(h, mi, s, frac, off).isoformat(), "rule": "iso8601_time"}
    return None


# ---------- natural language ----------

TIME_SUFFIX = re.compile(r"^(.*?)(?:,)?\s+(?:at|a las|a la|@)\s+(\d{1,2})(?:[:.h](\d{2}))?\s*(am|pm|a\.m\.|p\.m\.|h)?$")
TIME_BARE = re.compile(r"^(.*?)(?:,)?\s+(\d{1,2})(?::(\d{2})\s*(am|pm|a\.m\.|p\.m\.)?|\s*(am|pm|a\.m\.|p\.m\.))$")


def clock(h, mi, ampm):
    h, mi = int(h), int(mi or 0)
    if ampm:
        if not 1 <= h <= 12:
            raise ParseError("an am/pm hour must be 1 to 12")
        h = h % 12 + (12 if ampm.startswith("p") else 0)
    if h > 23 or mi > 59:
        raise ParseError("not a valid time of day")
    return dt.time(h, mi)


def split_time(text):
    m = TIME_SUFFIX.match(text)
    if m and m.group(1):
        return m.group(1).strip(), clock(m.group(2), m.group(3), m.group(4) if m.group(4) != "h" else None)
    m = TIME_BARE.match(text)
    if m and m.group(1):
        return m.group(1).strip(), clock(m.group(2), m.group(3), m.group(4) or m.group(5))
    return text, None


def need(ref, what):
    if ref is None:
        raise ParseError(f"reference is required for a relative expression ({what})")
    return ref


def weekday_from(ref, target, mode):
    base = ref.date() if isinstance(ref, dt.datetime) else ref
    if mode == "next":
        delta = (target - base.weekday() - 1) % 7 + 1
    elif mode == "last":
        delta = -((base.weekday() - target - 1) % 7 + 1)
    else:  # this / bare: on or after the reference
        delta = (target - base.weekday()) % 7
    return base + dt.timedelta(days=delta)


def numeric(text, order, ref):
    m = re.match(r"^(\d{4})[/.-](\d{1,2})[/.-](\d{1,2})$", text)
    if m:
        return {"type": "date", "value": make_date(*m.groups()).isoformat(), "rule": "numeric_ymd"}
    m = re.match(r"^(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})$", text)
    if not m:
        if re.match(r"^\d{1,2}[/.-]\d{1,2}[/.-]\d{2}$", text):
            raise ParseError("two-digit years are ambiguous: write the year with four digits")
        return None
    a, b, y = (int(g) for g in m.groups())
    if order is None:
        if a > 12 >= b:
            order, rule = "DMY", "numeric_dmy_inferred"
        elif b > 12 >= a:
            order, rule = "MDY", "numeric_mdy_inferred"
        else:
            raise ParseError(f"'{text}' is ambiguous: give date_order DMY (day first) or MDY (month first)")
    else:
        rule = "numeric_" + order.lower()
    d, mo = (a, b) if order == "DMY" else (b, a)
    return {"type": "date", "value": make_date(y, mo, d).isoformat(), "rule": rule}


def named_month(text, ref):
    weekday = None
    m = re.match(r"^" + WEEKDAY + r",?\s+(.*)$", text)
    if m:
        weekday, text = WEEKDAYS[m.group(1)], m.group(2)
    day = month = year = None
    m = re.match(r"^(?:el\s+)?(\d{1,2})(?:st|nd|rd|th|º|o)?\s+(?:de\s+)?" + MONTH + r"(?:,?\s+(?:de\s+|del\s+)?(\d{4}))?$", text)
    if m:
        day, month, year = m.group(1), MONTHS[m.group(2)], m.group(3)
    else:
        m = re.match(r"^" + MONTH + r"\s+(\d{1,2})(?:st|nd|rd|th)?(?:,?\s+(\d{4}))?$", text)
        if m:
            month, day, year = MONTHS[m.group(1)], m.group(2), m.group(3)
    if day is None:
        return None
    rule = "named_month"
    if year is None:
        ref = need(ref, "a date without a year takes the reference's year")
        year, rule = ref.year, "named_month_reference_year"
    result = make_date(year, month, day)
    if weekday is not None and result.weekday() != weekday:
        raise ParseError(f"{result.isoformat()} is a {WEEKDAY_NAMES[result.weekday()]}, not a {WEEKDAY_NAMES[weekday]}")
    return {"type": "date", "value": result.isoformat(), "rule": rule}


def relative(text, ref):
    if text in ("now", "right now", "ahora", "ahora mismo"):
        ref = need(ref, text)
        return ref, "now", True
    if text in KEYWORDS:
        ref = need(ref, text)
        return shift(ref, KEYWORDS[text], "days"), "keyword_" + str(KEYWORDS[text]), False
    patterns = [
        (r"^(?:next|the next|this coming|el proximo|el proximo dia|proximo|el|este|this|last|the last|el pasado|pasado)?\s*" + WEEKDAY
         + r"(?:\s+(next|que viene|proximo|pasado))?$", "weekday"),
        (r"^(?:in|within|dentro de|en)\s+" + NUM + r"\s+" + UNIT + r"$", "in"),
        (r"^" + NUM + r"\s+" + UNIT + r"\s+(?:from now|later|from today|despues|mas tarde)$", "in"),
        (r"^" + NUM + r"\s+" + UNIT + r"\s+ago$", "ago"),
        (r"^hace\s+" + NUM + r"\s+" + UNIT + r"$", "ago"),
        (r"^(next|last|this|la proxima|el proximo|la semana|el mes|el ano)\s*(week|month|year|semana|mes|ano)?(?:\s+(que viene|pasada|pasado))?$", "unit"),
    ]
    for pattern, kind in patterns:
        m = re.match(pattern, text)
        if not m:
            continue
        if kind == "weekday":
            prefix = text[:m.start(1)].strip()
            suffix = m.group(2)
            if prefix in ("next", "the next", "this coming", "el proximo", "el proximo dia", "proximo") or suffix in ("next", "que viene", "proximo"):
                mode = "next"
            elif prefix in ("last", "the last", "el pasado", "pasado") or suffix == "pasado":
                mode = "last"
            else:
                mode = "this"
            ref = need(ref, text)
            return weekday_from(ref, WEEKDAYS[m.group(1)], mode), f"weekday_{mode}", False
        if kind in ("in", "ago"):
            amount = number(m.group(1))
            unit = UNITS[m.group(2)]
            ref = need(ref, text)
            sign = 1 if kind == "in" else -1
            return shift(ref, sign * amount, unit), f"relative_{kind}_{unit}", unit in ("hours", "minutes", "seconds")
        if kind == "unit":
            words = text.split()
            unit_word = next((w for w in words if w in ("week", "month", "year", "semana", "mes", "ano")), None)
            if unit_word is None:
                return None
            unit = UNITS[unit_word]
            if words[0] in ("next", "la", "el") and ("proxima" in words or "proximo" in words or "que" in words or words[0] == "next"):
                sign = 1
            elif words[0] == "last" or words[-1] in ("pasada", "pasado"):
                sign = -1
            elif words[0] == "this":
                sign = 0
            else:
                return None
            ref = need(ref, text)
            return shift(ref, sign, unit), f"relative_{'next' if sign > 0 else 'last' if sign < 0 else 'this'}_{unit[:-1]}", False
    return None


def natural(text, ref, order):
    body, at = split_time(text)
    result = numeric(body, order, ref) or named_month(body, ref)
    if result is None:
        rel = relative(body, ref)
        if rel is None:
            return None
        value, rule, keep_time = rel
        if isinstance(value, dt.datetime) and not keep_time:
            value = value.date()
        if isinstance(value, dt.datetime):
            result = {"type": "datetime", "value": value.isoformat(), "rule": rule}
        else:
            result = {"type": "date", "value": value.isoformat(), "rule": rule}
    if at is not None:
        day = dt.date.fromisoformat(result["value"][:10])
        result = {"type": "datetime", "value": dt.datetime.combine(day, at).isoformat(), "rule": result["rule"] + "+time"}
    return result


def parse_reference(value):
    if value is None:
        return None
    if not isinstance(value, str):
        fail("reference must be a string YYYY-MM-DD or YYYY-MM-DDTHH:MM[:SS]")
    m = re.match(r"^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2})(?::(\d{2}))?)?$", value.strip())
    if not m:
        fail("reference must be YYYY-MM-DD or YYYY-MM-DDTHH:MM[:SS] (local, without offset)")
    y, mo, d, h, mi, s = m.groups()
    try:
        if h is None:
            return dt.date(int(y), int(mo), int(d))
        return dt.datetime(int(y), int(mo), int(d), int(h), int(mi), int(s or 0))
    except ValueError as error:
        fail(f"reference is not a valid date: {error}")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"text", "reference", "date_order"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("text")
    if not isinstance(text, str) or not text.strip():
        fail("text must be a non-empty string")
    if len(text) > 200:
        fail("text must be at most 200 characters")
    order = request.get("date_order")
    if order is not None and order not in ("DMY", "MDY"):
        fail("date_order must be DMY or MDY")
    ref = parse_reference(request.get("reference"))
    folded = fold(text).rstrip(".")
    try:
        result = iso(folded.replace(" ", "") if folded.startswith("p") else folded, ref) or natural(folded, ref, order)
    except ParseError as error:
        fail(str(error))
    if result is None:
        fail(f"could not parse '{text}': not ISO 8601 nor one of the supported English or Spanish forms")
    out = {"text": text, **result}
    if result["type"] in ("date", "datetime"):
        out["weekday"] = WEEKDAY_NAMES[dt.date.fromisoformat(result["value"][:10]).weekday()]
    if ref is not None:
        out["reference"] = ref.isoformat()
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except OverflowError:
        fail("a date is out of the supported range (years 1 to 9999)")
