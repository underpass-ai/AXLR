#!/usr/bin/env python3
"""Events to a sorted Markdown list, a Markdown table and an ASCII timeline scaled to a width,
with duplicate, same-date and overlap detection. Years BCE are supported.

Input (JSON on stdin):
  events  [{"label": "...", "date": "1066-10-14"} | {"label": "...", "start": "1337", "end": "1453"}]
          a date is an integer year (negative = BCE) or a string: "YYYY", "YYYY-MM", "YYYY-MM-DD",
          "-YYYY", "44 BC", "500 BCE", "AD 79", "79 CE"; there is no year 0 (1 BCE is followed by 1 CE)
  width   ASCII chart width in columns, default 60 (20-200)
  era     "BCE" | "BC" labels, default BCE
  title   optional heading
"""
import json
import math
import re
import sys
from decimal import ROUND_HALF_UP, Decimal
from fractions import Fraction


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


class When:
    """A date of year, month or day precision; year is historical (no year 0), astro = 1 - BCE year."""

    def __init__(self, year, bce, month=None, day=None):
        self.year, self.bce, self.month, self.day = year, bce, month, day
        self.astro = 1 - year if bce else year

    def key_start(self):
        return (self.astro, self.month or 1, self.day or 1)

    def key_end(self):
        month = self.month or 12
        return (self.astro, month, self.day or 31)

    def pos_start(self):
        p = Fraction(self.astro)
        if self.month:
            p += Fraction(self.month - 1, 12)
        if self.day:
            p += Fraction(self.day - 1, 12 * 31)
        return p

    def pos_end(self):
        """Exclusive end of the period: the start of the next year, month or day."""
        if self.day:
            return self.pos_start() + Fraction(1, 12 * 31)
        if self.month:
            return self.pos_start() + Fraction(1, 12)
        return Fraction(self.astro + 1)

    def label(self, era, any_bce):
        if self.bce:
            if self.month:
                return f"{self.year}-{self.month:02d}" + (f"-{self.day:02d}" if self.day else "") + f" {era}"
            return f"{self.year} {era}"
        text = f"{self.year:04d}" if self.year >= 1000 else str(self.year)
        if self.month:
            text = f"{self.year:04d}-{self.month:02d}" + (f"-{self.day:02d}" if self.day else "")
        if any_bce and not self.month:
            text += " CE" if era == "BCE" else ""
            if era == "BC":
                text = "AD " + text
        return text


def parse(value, field):
    if isinstance(value, bool):
        fail(f"{field} must be a year or a date string")
    if isinstance(value, int):
        if value == 0:
            fail(f"{field}: there is no year 0 (1 BCE is followed by 1 CE)")
        return When(abs(value), value < 0)
    if not isinstance(value, str):
        fail(f"{field} must be a year or a date string")
    s = value.strip()
    m = re.fullmatch(r"(?i)(\d{1,6})\s*(BCE|BC|B\.C\.E?\.?)", s)
    if m:
        year = int(m.group(1))
        if year == 0:
            fail(f"{field}: there is no year 0")
        return When(year, True)
    m = re.fullmatch(r"(?i)(?:AD|A\.D\.)\s*(\d{1,6})|(\d{1,6})\s*(?:CE|AD|C\.E\.)", s)
    if m:
        year = int(m.group(1) or m.group(2))
        if year == 0:
            fail(f"{field}: there is no year 0")
        return When(year, False)
    m = re.fullmatch(r"(-?)(\d{1,6})(?:-(\d{2})(?:-(\d{2}))?)?", s)
    if not m:
        fail(f"{field} {value!r} is not a year or YYYY[-MM[-DD]] date")
    year, month, day = int(m.group(2)), m.group(3), m.group(4)
    if year == 0:
        fail(f"{field}: there is no year 0 (1 BCE is followed by 1 CE)")
    bce = m.group(1) == "-"
    month = int(month) if month else None
    day = int(day) if day else None
    if month is not None and not 1 <= month <= 12:
        fail(f"{field} has month {month}")
    if day is not None:
        astro = 1 - year if bce else year
        leap = astro % 4 == 0 and (astro % 100 != 0 or astro % 400 == 0)  # proleptic Gregorian
        days = [31, 29 if leap else 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1]
        if not 1 <= day <= days:
            fail(f"{field} has day {day} outside the month")
    return When(year, bce, month, day)


def col(pos, low, span, width):
    """Column of a position, scaled exactly and rounded half up."""
    return math.floor((pos - low) / span * (width - 1) + Fraction(1, 2))


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"events", "width", "era", "title"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    raw = request.get("events")
    if not isinstance(raw, list) or not 1 <= len(raw) <= 500:
        fail("events must be a list of 1 to 500 events")
    width = request.get("width", 60)
    if isinstance(width, bool) or not isinstance(width, int) or not 20 <= width <= 200:
        fail("width must be an integer from 20 to 200")
    era = request.get("era", "BCE")
    if era not in ("BCE", "BC"):
        fail("era must be BCE or BC")
    title = request.get("title")
    if title is not None and (not isinstance(title, str) or len(title) > 200):
        fail("title must be a string of at most 200 characters")
    events = []
    for i, e in enumerate(raw):
        if not isinstance(e, dict) or set(e) - {"label", "date", "year", "start", "end"}:
            fail(f"events[{i}] must have label and date (or year), or start and end")
        label = e.get("label")
        if not isinstance(label, str) or not label.strip() or len(label) > 200 or "\n" in label:
            fail(f"events[{i}].label must be a one-line string of at most 200 characters")
        point = e.get("date", e.get("year"))
        if point is not None and ("start" in e or "end" in e):
            fail(f"events[{i}]: give date or start/end, not both")
        if point is not None:
            start = end = parse(point, f"events[{i}].date")
            is_range = False
        else:
            if "start" not in e or "end" not in e:
                fail(f"events[{i}] needs date (or year), or both start and end")
            start, end = parse(e["start"], f"events[{i}].start"), parse(e["end"], f"events[{i}].end")
            if end.key_end() < start.key_start():
                fail(f"events[{i}] ends before it starts")
            is_range = True
        events.append({"index": i, "label": label.strip(), "start": start, "end": end, "range": is_range})
    events.sort(key=lambda ev: (ev["start"].key_start(), ev["end"].key_end(), ev["label"].casefold()))
    any_bce = any(ev["start"].bce or ev["end"].bce for ev in events)

    def when_text(ev):
        s = ev["start"].label(era, any_bce)
        return f"{s} – {ev['end'].label(era, any_bce)}" if ev["range"] else s

    duplicates, same_date, overlaps, during = [], [], [], []
    for a in range(len(events)):
        for b in range(a + 1, len(events)):
            x, y = events[a], events[b]
            same = x["start"].key_start() == y["start"].key_start() and x["end"].key_end() == y["end"].key_end()
            if same and x["label"].casefold() == y["label"].casefold():
                duplicates.append({"events": [x["index"] + 1, y["index"] + 1], "label": x["label"], "when": when_text(x)})
                continue
            if same and not x["range"] and not y["range"]:
                same_date.append({"events": [x["index"] + 1, y["index"] + 1], "when": when_text(x),
                                  "labels": [x["label"], y["label"]]})
            xs, xe, ys, ye = x["start"].pos_start(), x["end"].pos_end(), y["start"].pos_start(), y["end"].pos_end()
            if x["range"] and y["range"] and xs < ye and ys < xe:
                kind = "contains" if (xs <= ys and ye <= xe) or (ys <= xs and xe <= ye) else "overlap"
                overlaps.append({"events": [x["index"] + 1, y["index"] + 1], "kind": kind,
                                 "labels": [x["label"], y["label"]]})
            for r, p in ((x, y), (y, x)):
                if r["range"] and not p["range"] and r["start"].pos_start() <= p["start"].pos_start() < r["end"].pos_end():
                    during.append({"event": p["index"] + 1, "label": p["label"], "during": r["label"]})

    lines_md = ([f"## {title}", ""] if title else []) + [f"- **{when_text(ev)}** — {ev['label']}" for ev in events]
    table = ["| Start | End | Event | Years |", "|:--|:--|:--|--:|"]
    for ev in events:
        years = ""
        if ev["range"]:
            length = ev["end"].pos_start() - ev["start"].pos_start()
            years = str(length) if length.denominator == 1 else str(
                (Decimal(length.numerator) / Decimal(length.denominator)).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP))
        end = ev["end"].label(era, any_bce) if ev["range"] else ""
        table.append(f"| {ev['start'].label(era, any_bce)} | {end} | {ev['label'].replace('|', '/')} | {years} |")

    low = min(ev["start"].pos_start() for ev in events)
    high = max(ev["end"].pos_end() if ev["range"] else ev["start"].pos_start() for ev in events)
    span = high - low if high > low else Fraction(1)
    first, last = events[0]["start"].label(era, any_bce), max(
        events, key=lambda ev: (ev["end"] if ev["range"] else ev["start"]).pos_end())
    last_label = (last["end"] if last["range"] else last["start"]).label(era, any_bce)
    gap = width - len(first) - len(last_label)
    ascii_lines = [first + " " * max(gap, 1) + last_label, "+" + "-" * (width - 2) + "+"]
    for ev in events:
        row = [" "] * width
        c1 = col(ev["start"].pos_start(), low, span, width)
        if ev["range"]:
            c2 = max(c1, col(ev["end"].pos_end(), low, span, width))
            if c2 == c1:
                row[c1] = "#"
            else:
                for c in range(c1 + 1, c2):
                    row[c] = "="
                row[c1], row[c2] = "[", "]"
        else:
            row[c1] = "*"
        ascii_lines.append("".join(row) + "  " + when_text(ev) + "  " + ev["label"])

    print(json.dumps({
        "events": [{"n": k + 1, "input_index": ev["index"] + 1, "label": ev["label"], "when": when_text(ev),
                    "start": ev["start"].label(era, any_bce), "end": ev["end"].label(era, any_bce) if ev["range"] else None,
                    "astronomical_year": ev["start"].astro} for k, ev in enumerate(events)],
        "markdown_list": "\n".join(lines_md) + "\n", "markdown_table": "\n".join(table) + "\n",
        "ascii": "\n".join(ascii_lines) + "\n",
        "duplicates": duplicates, "same_date": same_date, "overlaps": overlaps, "during": during,
        "scale": {"from": first, "to": last_label, "width": width,
                  "years_per_column": str((Decimal(span.numerator) / Decimal(span.denominator * (width - 1))).quantize(
                      Decimal("0.001"), rounding=ROUND_HALF_UP))},
    }, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
