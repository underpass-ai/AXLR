#!/usr/bin/env python3
"""Summarise a log: group lines into templates (variable parts masked, then Drain-like token
similarity), with a count, first and last timestamp and levels per template, overall level
counts and the top templates.

Input (JSON on stdin):
  text        the log lines
  top         how many templates to list (default 20, at most 500)
  similarity  Drain similarity threshold, 0 to 1 (default 0.5)
  year        year to give syslog timestamps that carry none (optional)
  mask        list of masks to apply (default all): uuid, ip, hex, timestamp, string, number
"""
import datetime as dt
import json
import re
import sys
from fractions import Fraction

MAX_TEXT = 5_000_000
MAX_LINES = 200_000
MONTHS = {m: i + 1 for i, m in enumerate(["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"])}
MONTH_RX = "Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec"
ISO = re.compile(r"(?P<date>\d{4}-\d{2}-\d{2})[T ](?P<time>\d{2}:\d{2}(?::\d{2}(?:[.,]\d{1,9})?)?)"
                 r"(?P<tz>Z|[+-]\d{2}:?\d{2})?")
CLF = re.compile(r"\[(?P<d>\d{2})/(?P<mon>" + MONTH_RX + r")/(?P<y>\d{4}):(?P<t>\d{2}:\d{2}:\d{2}) (?P<tz>[+-]\d{4})\]")
SYSLOG = re.compile(r"^(?:<\d{1,3}>)?(?P<mon>" + MONTH_RX + r") {1,2}(?P<d>\d{1,2}) (?P<t>\d{2}:\d{2}:\d{2})\b")
LEVELS = [("FATAL", r"FATAL|EMERG(?:ENCY)?|ALERT|CRIT(?:ICAL)?|SEVERE"), ("ERROR", r"ERROR|ERR"),
          ("WARN", r"WARN(?:ING)?"), ("NOTICE", r"NOTICE"), ("INFO", r"INFO"), ("DEBUG", r"DEBUG"), ("TRACE", r"TRACE")]
LEVEL_RX = re.compile(r"(?:^|[\s\[(<|:])(" + "|".join(p for _, p in LEVELS) + r")(?=$|[\s\]):>|,])")
LEVEL_KV = re.compile(r"\b(?:level|lvl|severity)\s*[=:]\s*\"?([A-Za-z]+)", re.I)
MASKS = [
    ("uuid", re.compile(r"\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b"), "<UUID>"),
    ("string", re.compile(r"\"(?:[^\"\\]|\\.)*\"|'(?:[^'\\]|\\.)*'"), "<STR>"),
    ("timestamp", re.compile(r"\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(?::\d{2}(?:[.,]\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?|\b\d{2}:\d{2}:\d{2}(?:[.,]\d+)?\b"), "<TS>"),
    ("ip", re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}(?::\d{1,5})?\b|\b(?:[0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{1,4}\b"), "<IP>"),
    ("hex", re.compile(r"\b0x[0-9a-fA-F]+\b|\b(?=[0-9a-fA-F]*\d)(?=[0-9a-fA-F]*[a-fA-F])[0-9a-fA-F]{8,}\b"), "<HEX>"),
    ("number", re.compile(r"(?<![\w.<])[-+]?\d+(?:\.\d+)?"), "<NUM>"),
]
MASK_TOKEN = re.compile(r"^<(UUID|STR|TS|IP|HEX|NUM)>$")
WILD = "<*>"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def level_of(name):
    up = name.upper()
    for canon, pattern in LEVELS:
        if re.fullmatch(pattern, up):
            return canon
    return None


def find_timestamp(line, year):
    """Return (iso_text, sort_key, span) of the first timestamp in the line, or None."""
    m = SYSLOG.match(line)
    if m:
        mon, day, t = MONTHS[m.group("mon")], int(m.group("d")), m.group("t")
        y = year if year else 1900
        try:
            moment = dt.datetime(y, mon, day, *map(int, t.split(":")))
        except ValueError:
            return None
        text = moment.isoformat() if year else f"--{mon:02d}-{day:02d}T{t}"
        return text, moment, m.span("mon")[0], m.end()
    m = CLF.search(line)
    if m:
        try:
            moment = dt.datetime.strptime(f"{m.group('d')}/{m.group('mon')}/{m.group('y')}:{m.group('t')} {m.group('tz')}",
                                          "%d/%b/%Y:%H:%M:%S %z")
        except ValueError:
            return None
        return moment.isoformat(), moment.astimezone(dt.timezone.utc).replace(tzinfo=None), m.start(), m.end()
    m = ISO.search(line[:80])
    if m:
        t = m.group("time").replace(",", ".")
        tz = m.group("tz") or ""
        if tz and tz != "Z" and ":" not in tz:
            tz = tz[:3] + ":" + tz[3:]
        try:
            y, mo, d = map(int, m.group("date").split("-"))
            sec = int(t[6:8]) if len(t) >= 8 else 0
            micro = int((t.split(".")[1] + "000000")[:6]) if "." in t else 0
            zone = None
            if tz == "Z":
                zone = dt.timezone.utc
            elif tz:
                sign = 1 if tz[0] == "+" else -1
                zone = dt.timezone(sign * dt.timedelta(hours=int(tz[1:3]), minutes=int(tz[4:6])))
            moment = dt.datetime(y, mo, d, int(t[:2]), int(t[3:5]), sec, micro, tzinfo=zone)
        except ValueError:
            return None
        key = moment.astimezone(dt.timezone.utc).replace(tzinfo=None) if zone else moment
        return f"{m.group('date')}T{t}{tz}", key, m.start(), m.end()
    return None


def mask(message, enabled):
    for name, rx, token in MASKS:
        if name in enabled:
            message = rx.sub(token, message)
    return message


class Cluster:
    def __init__(self, tokens, number, raw):
        self.tokens = list(tokens)
        self.count = 0
        self.first_line = number
        self.example = raw
        self.first = self.last = None
        self.levels = {}


def similarity(template, tokens):
    """Drain: share of positions where the tokens are equal; wildcard positions do not count."""
    same = sum(1 for a, b in zip(template, tokens) if a == b and a != WILD)
    params = sum(1 for a in template if a == WILD)
    return Fraction(same, len(tokens)) if tokens else Fraction(1), params


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    extra = sorted(set(request) - {"text", "top", "similarity", "year", "mask"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("text")
    if not isinstance(text, str):
        fail("text must be a string with the log lines")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    top = request.get("top", 20)
    if not isinstance(top, int) or isinstance(top, bool) or not 1 <= top <= 500:
        fail("top must be an integer from 1 to 500")
    sim = request.get("similarity", 0.5)
    if isinstance(sim, bool) or not isinstance(sim, (int, float, str)):
        fail("similarity must be a number from 0 to 1")
    try:
        threshold = Fraction(str(sim))
    except (ValueError, ZeroDivisionError):
        fail("similarity must be a number from 0 to 1")
    if not 0 <= threshold <= 1:
        fail("similarity must be a number from 0 to 1")
    year = request.get("year")
    if year is not None and (not isinstance(year, int) or isinstance(year, bool) or not 1970 <= year <= 2100):
        fail("year must be an integer from 1970 to 2100")
    names = [n for n, _, _ in MASKS]
    enabled = request.get("mask", names)
    if not isinstance(enabled, list) or not all(n in names for n in enabled):
        fail(f"mask must be a list drawn from {', '.join(names)}")
    enabled = set(enabled)

    lines = text.splitlines()
    if len(lines) > MAX_LINES:
        fail(f"more than {MAX_LINES} lines")
    groups = {}
    clusters = []
    level_counts = {}
    no_ts = 0
    first_all = last_all = None
    nonempty = 0
    for number, raw in enumerate(lines, 1):
        if not raw.strip():
            continue
        nonempty += 1
        found = find_timestamp(raw, year)
        if found:
            ts_text, key, start, end = found
            message = (raw[:start] + " " + raw[end:]).strip()
        else:
            ts_text = key = None
            message = raw.strip()
            no_ts += 1
        m = LEVEL_KV.search(message)
        level = level_of(m.group(1)) if m else None
        if level is None:
            m = LEVEL_RX.search(message)
            level = level_of(m.group(1)) if m else None
        level = level or "UNKNOWN"
        level_counts[level] = level_counts.get(level, 0) + 1
        tokens = mask(message, enabled).split()
        if not tokens:
            tokens = ["<empty>"]
        first = tokens[0] if not MASK_TOKEN.match(tokens[0]) and not any(c.isdigit() for c in tokens[0]) else WILD
        bucket = groups.setdefault((len(tokens), first), [])
        best, best_score = None, None
        for c in bucket:
            score, params = similarity(c.tokens, tokens)
            rank = (score, params)
            if best_score is None or rank > best_score:
                best, best_score = c, rank
        if best is not None and best_score[0] >= threshold:
            cluster = best
            cluster.tokens = [a if a == b else WILD for a, b in zip(cluster.tokens, tokens)]
        else:
            cluster = Cluster(tokens, number, raw)
            bucket.append(cluster)
            clusters.append(cluster)
        cluster.count += 1
        cluster.levels[level] = cluster.levels.get(level, 0) + 1
        if key is not None:
            if cluster.first is None or key < cluster.first[1]:
                cluster.first = (ts_text, key)
            if cluster.last is None or key >= cluster.last[1]:
                cluster.last = (ts_text, key)
            if ts_text.startswith("--"):
                continue  # a syslog time without a year cannot be ordered against dated ones
            if first_all is None or key < first_all[1]:
                first_all = (ts_text, key)
            if last_all is None or key >= last_all[1]:
                last_all = (ts_text, key)
    clusters.sort(key=lambda c: (-c.count, c.first_line))
    templates = [{"template": " ".join(c.tokens), "count": c.count,
                  "share": f"{Fraction(c.count, nonempty) * 100:.1f}%" if nonempty else "0%",
                  "first_timestamp": c.first[0] if c.first else None, "last_timestamp": c.last[0] if c.last else None,
                  "levels": dict(sorted(c.levels.items())), "first_line": c.first_line, "example": c.example[:500]}
                 for c in clusters[:top]]
    order = [name for name, _ in LEVELS] + ["UNKNOWN"]
    result = {"lines": len(lines), "non_empty": nonempty, "template_count": len(clusters),
              "levels": {k: level_counts[k] for k in order if k in level_counts},
              "first_timestamp": first_all[0] if first_all else None, "last_timestamp": last_all[0] if last_all else None,
              "lines_without_timestamp": no_ts, "templates": templates}
    if len(clusters) > top:
        result["note"] = f"only the top {top} of {len(clusters)} templates are listed"
    if year is None and any(SYSLOG.match(l) for l in lines[:1000]):
        result["syslog_year"] = ("syslog timestamps carry no year: shown as --MM-DDTHH:MM:SS, ordered within one year inside "
                                 "their templates and left out of the overall first and last timestamps; give year")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
