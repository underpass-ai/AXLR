#!/usr/bin/env python3
"""Explain a cron expression in English and Spanish and list its next runs in an IANA time zone.

Input (JSON on stdin):
  expression  "30 4 1,15 * 5", "@daily", or a whole crontab line (the rest is the command)
  seconds     true: the expression has a leading seconds field (Spring/Quartz style); default false
  from        ISO 8601 date-time after which to list runs, e.g. "2026-10-18T00:00" (required for runs)
  timezone    IANA zone such as "Europe/Madrid" (default "UTC"); a naive `from` is local to it
  count       how many runs (default 5, at most 100; 0 explains only)
  dst         "skip" (default) or "shift": what to do with a local time that a spring-forward gap removes
"""
import datetime as dt
import json
import re
import sys

try:
    import zoneinfo
except ImportError:  # pragma: no cover
    zoneinfo = None

MONTHS = ["jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"]
DAYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"]
EN_MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September",
             "October", "November", "December"]
ES_MONTHS = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre",
             "octubre", "noviembre", "diciembre"]
EN_DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"]
ES_DAYS = ["domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"]
ES_DAYS_PL = ["domingos", "lunes", "martes", "miércoles", "jueves", "viernes", "sábados"]
ES_ORD = {1: "primer", 2: "segundo", 3: "tercer", 4: "cuarto", 5: "quinto"}
MACROS = {"@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *", "@monthly": "0 0 1 * *",
          "@weekly": "0 0 * * 0", "@daily": "0 0 * * *", "@midnight": "0 0 * * *", "@hourly": "0 * * * *"}
SPEC = {  # name: (low, high, names)
    "second": (0, 59, None), "minute": (0, 59, None), "hour": (0, 23, None),
    "day_of_month": (1, 31, None), "month": (1, 12, MONTHS), "day_of_week": (0, 7, DAYS)}
SEARCH_YEARS = 30


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def ordinal(n):
    if 10 <= n % 100 <= 20:
        return f"{n}th"
    return f"{n}{ {1: 'st', 2: 'nd', 3: 'rd'}.get(n % 10, 'th') }"


def join(items, word):
    items = list(items)
    if len(items) <= 1:
        return "".join(items)
    return ", ".join(items[:-1]) + f" {word} " + items[-1]


class Field:
    """One parsed field: the set of values plus a list of elements for the wording."""

    def __init__(self, name, text, nonstandard):
        self.name, self.text = name, text
        low, high, names = SPEC[name]
        self.star = text.startswith("*") or text == "?"
        self.values = set()
        self.elements = []  # (kind, a, b, step) kind: any, value, range, step
        self.special = []   # Quartz-style day specials: ("L", offset) ("W", n) ("LW",) ("lastdow", d) ("nth", d, k)
        if text == "?":
            if name not in ("day_of_month", "day_of_week"):
                fail(f"{name}: ? is only allowed in the day fields")
            nonstandard.append(f"{name}: ? (Quartz 'no specific value') is read as *")
            text = "*"
        for part in text.split(","):
            if not part:
                fail(f"{name}: empty element in {self.text!r}")
            if self.parse_special(part, nonstandard):
                continue
            m = re.fullmatch(r"(\*|[0-9A-Za-z]+)(?:-([0-9A-Za-z]+))?(?:/(\d+))?", part)
            if not m:
                fail(f"{name}: cannot read {part!r}")
            first, last, step = m.group(1), m.group(2), m.group(3)
            if step is not None:
                step = int(step)
                if step < 1:
                    fail(f"{name}: step must be at least 1")
            if first == "*":
                if last is not None:
                    fail(f"{name}: * cannot start a range")
                a, b = low, high if name != "day_of_week" else 6
                kind = "step" if step else "any"
            else:
                a = self.number(first, low, high, names)
                if last is not None:
                    b = self.number(last, low, high, names)
                    if b < a:
                        fail(f"{name}: range {part!r} runs backwards")
                    kind = "range"
                elif step:
                    b = high if name != "day_of_week" else 6
                    kind = "range"
                    nonstandard.append(f"{name}: {part!r} (start/step without a range) means {first}-{b}/{step}")
                else:
                    b = a
                    kind = "value"
            vals = range(a, b + 1, step or 1)
            for v in vals:
                self.values.add(0 if (name == "day_of_week" and v == 7) else v)
            self.elements.append((kind, a, b, step))

    def number(self, token, low, high, names):
        if token.isdigit():
            value = int(token)
        elif names and token.lower() in names:
            value = names.index(token.lower()) + (1 if self.name == "month" else 0)
        else:
            fail(f"{self.name}: {token!r} is not a number" + (" or a three-letter name" if names else ""))
        if not low <= value <= high:
            fail(f"{self.name}: {value} is outside {low}-{high}")
        return value

    def parse_special(self, part, nonstandard):
        up = part.upper()
        if self.name == "day_of_month":
            m = re.fullmatch(r"L(?:-(\d+))?", up)
            if m:
                self.special.append(("L", int(m.group(1) or 0)))
                nonstandard.append(f"day_of_month: {part} (Quartz/cronie last-day extension)")
                return True
            if up == "LW":
                self.special.append(("LW",))
                nonstandard.append("day_of_month: LW (Quartz last-weekday extension)")
                return True
            m = re.fullmatch(r"(\d+)W", up)
            if m:
                n = int(m.group(1))
                if not 1 <= n <= 31:
                    fail("day_of_month: nW needs 1-31")
                self.special.append(("W", n))
                nonstandard.append(f"day_of_month: {part} (Quartz nearest-weekday extension)")
                return True
        if self.name == "day_of_week":
            m = re.fullmatch(r"([0-7]|[A-Z]{3})L", up)
            if m:
                d = self.number(m.group(1), 0, 7, DAYS) % 7
                self.special.append(("lastdow", d))
                nonstandard.append(f"day_of_week: {part} (Quartz last-weekday-of-month extension, crontab day numbers)")
                return True
            m = re.fullmatch(r"([0-7]|[A-Z]{3})#([1-5])", up)
            if m:
                d = self.number(m.group(1), 0, 7, DAYS) % 7
                self.special.append(("nth", d, int(m.group(2))))
                nonstandard.append(f"day_of_week: {part} (Quartz/cronie nth-weekday extension, crontab day numbers)")
                return True
        if re.search(r"[LW#]", up) and not re.fullmatch(r"[A-Z]{3}(?:-[A-Z]{3})?", up):
            fail(f"{self.name}: {part!r} uses L, W or # where they are not allowed")
        return False

    def day_match(self, day):
        """day_of_month / day_of_week match of a date, including the specials."""
        if self.name == "day_of_month":
            if day.day in self.values:
                return True
            last = last_day(day.year, day.month)
            for sp in self.special:
                if sp[0] == "L" and day.day == last - sp[1]:
                    return True
                if sp[0] == "LW" and day == nearest_weekday(day.year, day.month, last):
                    return True
                if sp[0] == "W" and sp[1] <= last and day == nearest_weekday(day.year, day.month, sp[1]):
                    return True
            return False
        wd = (day.weekday() + 1) % 7  # Sunday = 0
        if wd in self.values:
            return True
        for sp in self.special:
            if sp[0] == "lastdow" and wd == sp[1] and day.day + 7 > last_day(day.year, day.month):
                return True
            if sp[0] == "nth" and wd == sp[1] and (day.day - 1) // 7 + 1 == sp[2]:
                return True
        return False


def last_day(year, month):
    nxt = dt.date(year + (month == 12), month % 12 + 1, 1)
    return (nxt - dt.timedelta(1)).day


def nearest_weekday(year, month, n):
    d = dt.date(year, month, n)
    if d.weekday() == 5:  # Saturday -> Friday, unless that leaves the month
        return d - dt.timedelta(1) if n > 1 else d + dt.timedelta(2)
    if d.weekday() == 6:  # Sunday -> Monday, unless that leaves the month
        return d + dt.timedelta(1) if n < last_day(year, month) else d - dt.timedelta(2)
    return d


# ---- wording ----------------------------------------------------------------------------

def label(field, v, lang):
    if field == "month":
        return (EN_MONTHS if lang == "en" else ES_MONTHS)[v - 1]
    if field == "day_of_week":
        return (EN_DAYS if lang == "en" else ES_DAYS)[v % 7]
    return str(v)


UNIT = {"second": ("second", "segundo", "segundos"), "minute": ("minute", "minuto", "minutos"),
        "hour": ("hour", "hora", "horas"), "day_of_month": ("day-of-month", "día", "días"),
        "month": ("month", "mes", "meses"), "day_of_week": ("day-of-week", "día de la semana", "días de la semana")}


def describe_en(f):
    unit = UNIT[f.name][0]
    if all(k == "value" for k, *_ in f.elements) and f.elements:
        vals = [label(f.name, a, "en") for _, a, _, _ in f.elements]
        prefix = "" if f.name in ("month", "day_of_week") else f"{unit} "
        return prefix + join(vals, "and")
    parts = []
    for kind, a, b, step in f.elements:
        if kind == "any":
            parts.append(f"every {unit}")
        elif kind == "value":
            parts.append(label(f.name, a, "en") if f.name in ("month", "day_of_week") else f"{unit} {a}")
        elif kind == "step":
            parts.append(f"every {ordinal(step)} {unit}")
        else:
            lead = f"every {ordinal(step)} {unit}" if step else f"every {unit}"
            parts.append(f"{lead} from {label(f.name, a, 'en')} through {label(f.name, b, 'en')}")
    return join(parts, "and")


def describe_es(f):
    one, many = UNIT[f.name][1], UNIT[f.name][2]
    fem = f.name == "hour"
    art_one, art_many = ("la", "las") if fem else ("el", "los")
    if all(k == "value" for k, *_ in f.elements) and f.elements:
        vals = [label(f.name, a, "es") for _, a, _, _ in f.elements]
        if f.name == "month":
            return "en " + join(vals, "y")
        if f.name == "day_of_week":
            return "los " + join([ES_DAYS_PL[a % 7] for _, a, _, _ in f.elements], "y")
        if len(vals) == 1:
            return f"{art_one} {one} {vals[0]}"
        return f"{art_many} {many} " + join(vals, "y")
    parts = []
    for kind, a, b, step in f.elements:
        if kind == "any":
            parts.append(f"cada {one}")
        elif kind == "value":
            if f.name == "day_of_week":
                parts.append("los " + ES_DAYS_PL[a % 7])
            else:
                parts.append(label(f.name, a, "es") if f.name == "month" else f"{art_one} {one} {a}")
        elif kind == "step":
            parts.append(f"cada {step} {many}")
        else:
            lead = f"cada {step} {many}" if step else f"cada {one}"
            if f.name in ("month", "day_of_week"):
                parts.append(f"{lead} de {label(f.name, a, 'es')} a {label(f.name, b, 'es')}")
            else:
                parts.append(f"{lead} del {a} al {b}" if f.name != "hour" else f"{lead} de {a} a {b}")
    return join(parts, "y")


def special_en(sp):
    if sp[0] == "L":
        return "the last day of the month" if not sp[1] else f"{sp[1]} days before the last day of the month"
    if sp[0] == "LW":
        return "the last weekday (Mon-Fri) of the month"
    if sp[0] == "W":
        return f"the weekday (Mon-Fri) nearest day {sp[1]} of the month"
    if sp[0] == "lastdow":
        return f"the last {EN_DAYS[sp[1]]} of the month"
    return f"the {ordinal(sp[2])} {EN_DAYS[sp[1]]} of the month"


def special_es(sp):
    if sp[0] == "L":
        return "el último día del mes" if not sp[1] else f"{sp[1]} días antes del último día del mes"
    if sp[0] == "LW":
        return "el último día laborable (lunes a viernes) del mes"
    if sp[0] == "W":
        return f"el día laborable (lunes a viernes) más cercano al día {sp[1]} del mes"
    if sp[0] == "lastdow":
        return f"el último {ES_DAYS[sp[1]]} del mes"
    return f"el {ES_ORD[sp[2]]} {ES_DAYS[sp[1]]} del mes"


def day_text(f, lang):
    base = (describe_en(f) if lang == "en" else describe_es(f)) if f.elements else ""
    sps = [(special_en if lang == "en" else special_es)(sp) for sp in f.special]
    items = ([base] if base else []) + sps
    return join(items, "and" if lang == "en" else "y")


def single(f):
    return len(f.elements) == 1 and f.elements[0][0] == "value"


def explain(fields, has_seconds):
    mi, ho, dom, mon, dow = (fields[k] for k in ("minute", "hour", "day_of_month", "month", "day_of_week"))
    se = fields.get("second")
    sec_single = se is None or single(se)
    en, es = "", ""
    if single(mi) and sec_single and all(k == "value" for k, *_ in ho.elements):
        sec = f":{se.elements[0][1]:02d}" if has_seconds else ""
        times = [f"{h:02d}:{mi.elements[0][1]:02d}{sec}" for _, h, _, _ in ho.elements]
        en = "At " + join(times, "and")
        es_times = [("a la " if t[:2] == "01" else "a las ") + t for t in times]
        es = "A" + join(es_times, "y")[1:]
    else:
        m_en, h_en = describe_en(mi), describe_en(ho)
        m_es, h_es = describe_es(mi), describe_es(ho)
        en = f"At {m_en} past {h_en}"
        es = f"{m_es[0].upper() + m_es[1:]} de {h_es}" if m_es.startswith("cada") else f"En {m_es} de {h_es}"
        if has_seconds and not (single(se) and se.elements[0][1] == 0):
            en = f"At {describe_en(se)}, " + en[0].lower() + en[1:]
            es = f"{describe_es(se)[0].upper() + describe_es(se)[1:]}, " + es[0].lower() + es[1:]
    dom_r, dow_r = not dom.star, not dow.star
    if dom_r and dow_r:
        en += f", {dom_en(dom)} or on {day_text(dow, 'en')}"
        es += f", {dom_es(dom)} o {day_text(dow, 'es')}"
        rule = "or"
    elif dom_r or dow_r:
        if dom_r:
            en += f", {dom_en(dom)}"
            es += f", {dom_es(dom)}"
        else:
            en += f", on {day_text(dow, 'en')}"
            es += f", {day_text(dow, 'es')}"
        rule = "single"
        other = dow if dom_r else dom
        if other.elements and other.elements[0][0] != "any" or other.special:
            # the unrestricted-looking field starts with * but has a step: Vixie ANDs them
            if dom_r:
                en += ", only when the day-of-week is also " + join([EN_DAYS[v] for v in sorted(dow.values)], "or")
                es += ", solo si además es " + join([ES_DAYS[v] for v in sorted(dow.values)], "o")
            else:
                en += ", only when the day-of-month is also " + join([str(v) for v in sorted(dom.values)], "or")
                es += ", solo si el día del mes es además " + join([str(v) for v in sorted(dom.values)], "o")
            rule = "and"
    else:
        rule = "and"
        extra_en, extra_es = [], []
        for f in (dom, dow):
            if f.elements and f.elements[0][0] != "any":
                extra_en.append(day_text(f, "en"))
                extra_es.append(day_text(f, "es") if f is dow else dom_es(f))
        if extra_en:
            en += ", on " + " and ".join(extra_en)
            es += ", " + " y ".join(extra_es)
    if not mon.star or (mon.elements and mon.elements[0][0] != "any"):
        en += ", in " + describe_en(mon)
        mon_es = describe_es(mon)
        es += ", " + (mon_es if mon_es.startswith("en ") else "en " + mon_es)
    return en + ".", es + ".", rule


def dom_en(f):
    parts = []
    if f.elements:
        parts.append("on " + describe_en(f))
    parts += ["on " + special_en(sp) for sp in f.special]
    return join(parts, "and")


def dom_es(f):
    txt = day_text(f, "es")
    if f.special and not f.elements:
        return txt
    if all(k == "value" for k, *_ in f.elements) and f.elements:
        vals = [str(a) for _, a, _, _ in f.elements]
        lead = ("el día " if len(vals) == 1 else "los días ") + join(vals, "y")
        sps = [special_es(sp) for sp in f.special]
        return join([lead + " del mes"] + sps, "y")
    return txt + " del mes" if not f.special else txt


# ---- schedule ---------------------------------------------------------------------------

def day_ok(fields, day):
    dom, dow, mon = fields["day_of_month"], fields["day_of_week"], fields["month"]
    if day.month not in mon.values:
        return False
    a, b = dom.day_match(day), dow.day_match(day)
    if dom.star or dow.star:  # Vixie: a field whose first character is * makes the test an AND
        return a and b
    return a or b


def get_zone(name):
    if name in ("UTC", "Etc/UTC", "Z"):
        return dt.timezone.utc
    if zoneinfo is None:
        fail("this Python has no zoneinfo; only UTC is available")
    if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9_+\-]+(?:/[A-Za-z0-9_+\-]+)*", name):
        fail("timezone must be an IANA zone name such as Europe/Madrid")
    try:
        return zoneinfo.ZoneInfo(name)
    except Exception:  # ZoneInfoNotFoundError, ValueError
        fail(f"unknown time zone {name!r} (or no time zone database on this system)")


def exists(naive, tz):
    aware = naive.replace(tzinfo=tz)
    back = aware.astimezone(dt.timezone.utc).astimezone(tz).replace(tzinfo=None)
    return back == naive


def ambiguous(naive, tz):
    return naive.replace(tzinfo=tz, fold=0).utcoffset() != naive.replace(tzinfo=tz, fold=1).utcoffset()


def gap_end(naive, tz):
    """First instant after the spring-forward gap that swallowed `naive`."""
    lo = naive.replace(tzinfo=tz, fold=1).astimezone(dt.timezone.utc)
    hi = naive.replace(tzinfo=tz, fold=0).astimezone(dt.timezone.utc)
    if hi < lo:
        lo, hi = hi, lo
    after = hi.astimezone(tz).utcoffset()
    while hi - lo > dt.timedelta(seconds=1):
        mid = lo + (hi - lo) / 2
        if mid.astimezone(tz).utcoffset() == after:
            hi = mid
        else:
            lo = mid
    return hi.replace(microsecond=0).astimezone(tz)


def next_runs(fields, start, tz, count, policy):
    runs, notes = [], []
    secs = sorted(fields["second"].values) if "second" in fields else [0]
    hours, minutes = sorted(fields["hour"].values), sorted(fields["minute"].values)
    local_start = start.astimezone(tz)
    day = local_start.date()
    limit = day + dt.timedelta(days=366 * SEARCH_YEARS)
    seen = set()
    while day <= limit and len(runs) < count:
        if day_ok(fields, day):
            for h in hours:
                for mi in minutes:
                    for s in secs:
                        naive = dt.datetime(day.year, day.month, day.day, h, mi, s)
                        if not exists(naive, tz):
                            if policy == "skip":
                                notes.append(f"{naive.isoformat()} does not exist in {tz} (DST gap): skipped")
                                continue
                            moment = gap_end(naive, tz)
                            note = f"{naive.isoformat()} does not exist in {tz} (DST gap): run at {moment.isoformat()}"
                        else:
                            moment = naive.replace(tzinfo=tz, fold=0)
                            note = None
                            if ambiguous(naive, tz):
                                note = f"{naive.isoformat()} occurs twice in {tz} (DST end): run once, at the first"
                        utc = moment.astimezone(dt.timezone.utc)
                        if utc <= start.astimezone(dt.timezone.utc) or utc in seen:
                            continue
                        seen.add(utc)
                        if note:
                            notes.append(note)
                        runs.append(moment)
                        if len(runs) >= count:
                            break
                    if len(runs) >= count:
                        break
                if len(runs) >= count:
                    break
        day += dt.timedelta(1)
    runs.sort(key=lambda m: m.astimezone(dt.timezone.utc))
    return runs, notes, day > limit


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra = sorted(set(request) - {"expression", "seconds", "from", "timezone", "count", "dst"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    expr = request.get("expression")
    if not isinstance(expr, str) or not expr.strip():
        fail("expression must be a non-empty string")
    if len(expr) > 1000:
        fail("expression is longer than 1000 characters")
    has_seconds = request.get("seconds", False)
    if not isinstance(has_seconds, bool):
        fail("seconds must be true or false")
    count = request.get("count", 5)
    if not isinstance(count, int) or isinstance(count, bool) or not 0 <= count <= 100:
        fail("count must be an integer from 0 to 100")
    policy = request.get("dst", "skip")
    if policy not in ("skip", "shift"):
        fail("dst must be skip or shift")
    tzname = request.get("timezone", "UTC")
    if not isinstance(tzname, str):
        fail("timezone must be a string")
    tz = get_zone(tzname)

    tokens = expr.split()
    macro = None
    warnings, nonstandard = [], []
    if tokens[0].startswith("@"):
        macro = tokens[0].lower()
        command = " ".join(tokens[1:]) or None
        if macro == "@reboot":
            print(json.dumps({"expression": expr, "macro": "@reboot", "command": command,
                              "explanation_en": "Once, when the cron daemon starts.",
                              "explanation_es": "Una vez, al arrancar el demonio cron.", "next_runs": []},
                             ensure_ascii=False))
            return
        if macro not in MACROS:
            fail(f"unknown macro {tokens[0]}; known: @reboot, {', '.join(MACROS)}")
        if has_seconds:
            fail("a macro has no seconds field; leave seconds false")
        tokens = MACROS[macro].split()
    else:
        command = None
    names = (["second"] if has_seconds else []) + ["minute", "hour", "day_of_month", "month", "day_of_week"]
    if len(tokens) < len(names):
        fail(f"expected {len(names)} fields ({', '.join(names)}), got {len(tokens)}")
    if macro is None:
        command = " ".join(tokens[len(names):]) or None
        if command and not has_seconds and re.fullmatch(r"[0-9*/,\-?LW#A-Za-z]+", tokens[len(names)]) \
                and re.fullmatch(r"[0-9*/,\-?LW#]+", tokens[len(names)]):
            warnings.append("a sixth field that looks like cron syntax was read as the command; "
                            "set seconds: true if the first field is seconds")
    fields = {n: Field(n, t, nonstandard) for n, t in zip(names, tokens)}
    for n in ("day_of_month", "day_of_week"):
        if fields[n].special and fields[n].star:
            fail(f"{n}: L, W or # cannot be combined with *")
    en, es, rule = explain(fields, has_seconds)
    result = {"expression": expr, "fields": {}, "explanation_en": en, "explanation_es": es,
              "day_rule": {"or": "day-of-month OR day-of-week (both restricted, Vixie rule)",
                           "and": "day-of-month AND day-of-week (a field starting with * counts as unrestricted)",
                           "single": "only the restricted day field decides"}[rule]}
    if macro:
        result["macro"] = macro
        result["equivalent"] = MACROS[macro]
    if command:
        result["command"] = command
    for n in names:
        f = fields[n]
        result["fields"][n] = {"text": f.text, "values": sorted(f.values)}
        if f.special:
            result["fields"][n]["special"] = [special_en(sp) for sp in f.special]
    if nonstandard:
        result["nonstandard"] = nonstandard
    if warnings:
        result["warnings"] = warnings
    if count:
        start_text = request.get("from")
        if not isinstance(start_text, str):
            fail("from is required to list runs: an ISO 8601 date-time such as 2026-10-18T00:00 (or count 0)")
        try:
            start = dt.datetime.fromisoformat(start_text.replace("Z", "+00:00"))
        except ValueError:
            fail("from must be an ISO 8601 date-time such as 2026-10-18T09:30 or 2026-10-18T09:30:00+02:00")
        if start.tzinfo is None:
            start = start.replace(tzinfo=tz)
        runs, notes, exhausted = next_runs(fields, start, tz, count, policy)
        result["timezone"] = tzname
        result["from"] = start.isoformat()
        result["next_runs"] = [r.isoformat() for r in runs]
        result["next_runs_utc"] = [r.astimezone(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ") for r in runs]
        result["dst_policy"] = policy
        if notes:
            result["dst_notes"] = notes[:20]
        if len(runs) < count:
            result["note"] = f"only {len(runs)} run(s) in the {SEARCH_YEARS} years searched"
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
