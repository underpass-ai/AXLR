#!/usr/bin/env python3
"""Describe the columns of a CSV dataset and draft a data dictionary: per column the inferred
type, nulls, distinct values, examples, min/max (and mean for numbers), pattern hints (dates,
e-mail, URL, UUID, identifiers, categories, zero-padded codes), plus a dictionary skeleton in
JSON and Markdown with blank description fields to fill in.

Input (JSON on stdin):
  csv            the CSV text (or path: a relative file path inside the working directory)
  delimiter      "," ";" "\t" "|" or "auto" (default: sniffed from the first lines)
  header         first row holds the column names (default true)
  null_values    strings counted as missing, default ["", "NA", "N/A", "NULL", "null", "None", "NaN", "nan", "-"]
  max_examples   distinct examples per column, default 3 (0-10)
  dataset        dataset name for the skeleton, default "dataset"
"""
import csv
import datetime as dt
import io
import json
import os
import re
import sys
from decimal import Decimal, InvalidOperation

MAX_TEXT = 20_000_000
DEFAULT_NULLS = ["", "NA", "N/A", "NULL", "null", "None", "NaN", "nan", "-"]
INTEGER = re.compile(r"^[+-]?\d+$")
NUMBER = re.compile(r"^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$")
BOOLEAN = {"true", "false", "yes", "no", "t", "f", "y", "n"}
ISO_DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
SLASH_DATE = re.compile(r"^(\d{1,2})[/.](\d{1,2})[/.](\d{4})$")
DATETIME = re.compile(r"^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?$")
TIME = re.compile(r"^\d{1,2}:\d{2}(:\d{2})?$")
EMAIL = re.compile(r"^[^@\s]+@[^@\s]+\.[A-Za-z]{2,}$")
URL = re.compile(r"^https?://\S+$", re.I)
UUID = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
ID_NAME = re.compile(r"((^|_)(id|key|code|uuid)$|^id_|[a-z]Id$)")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_input(request):
    if ("csv" in request) == ("path" in request):
        fail("give csv (the text) or path (a relative file), not both")
    if "csv" in request:
        text = request["csv"]
        if not isinstance(text, str):
            fail("csv must be a string")
        return text
    path = request["path"]
    if not isinstance(path, str) or not path or os.path.isabs(path) or ".." in re.split(r"[\\/]", path):
        fail("path must be a relative path inside the working directory, without ..")
    full = os.path.realpath(path)
    if not full.startswith(os.path.realpath(os.getcwd()) + os.sep):
        fail("path must stay inside the working directory")
    try:
        with open(full, encoding="utf-8-sig", newline="") as handle:
            return handle.read(MAX_TEXT + 1)
    except OSError as error:
        fail(f"cannot read {path}: {error.strerror}")
    except UnicodeDecodeError:
        fail(f"{path} is not UTF-8 text")


def valid_date(y, m, d):
    try:
        dt.date(y, m, d)
        return True
    except ValueError:
        return False


def classify(value):
    """The narrowest type of one non-null value."""
    v = value.strip()
    if v.lower() in BOOLEAN:
        return "boolean"
    if INTEGER.match(v):
        return "integer"
    if NUMBER.match(v):
        return "number"
    if ISO_DATE.match(v) and valid_date(int(v[:4]), int(v[5:7]), int(v[8:10])):
        return "date"
    m = SLASH_DATE.match(v)
    if m:
        a, b, y = int(m.group(1)), int(m.group(2)), int(m.group(3))
        if valid_date(y, b, a) or valid_date(y, a, b):
            return "date"
    if DATETIME.match(v):
        # checked by hand rather than with fromisoformat, whose accepted forms differ by Python version
        hh, mm = int(v[11:13]), int(v[14:16])
        ss = int(v[17:19]) if len(v) > 18 and v[16] == ":" else 0
        if valid_date(int(v[:4]), int(v[5:7]), int(v[8:10])) and hh < 24 and mm < 60 and ss < 60:
            return "datetime"
    if TIME.match(v):
        h, mi = int(v.split(":")[0]), int(v.split(":")[1])
        if h < 24 and mi < 60:
            return "time"
    return "string"


def column_type(kinds):
    present = set(kinds)
    if not present:
        return "empty"
    if len(present) == 1:
        return present.pop()
    if present <= {"integer", "number"}:
        return "number"
    if present <= {"date", "datetime"}:
        return "datetime"
    return "string"


def date_format(values):
    if all(ISO_DATE.match(v) for v in values):
        return "YYYY-MM-DD"
    parts = [SLASH_DATE.match(v) for v in values]
    if all(parts):
        sep = "/" if "/" in values[0] else "."
        first_big = any(int(p.group(1)) > 12 for p in parts)
        second_big = any(int(p.group(2)) > 12 for p in parts)
        if first_big and not second_big:
            return f"DD{sep}MM{sep}YYYY"
        if second_big and not first_big:
            return f"MM{sep}DD{sep}YYYY"
        return f"DD{sep}MM{sep}YYYY or MM{sep}DD{sep}YYYY (ambiguous)"
    return "mixed"


def date_key(v, fmt):
    v = v.strip()
    if ISO_DATE.match(v) or DATETIME.match(v):
        return v.replace(" ", "T", 1)
    m = SLASH_DATE.match(v)
    if m:
        a, b, y = m.group(1), m.group(2), m.group(3)
        day, month = (b, a) if fmt.startswith("MM") else (a, b)
        return f"{y}-{int(month):02d}-{int(day):02d}"
    return v


def describe(name, raw_values, nulls, max_examples):
    total = len(raw_values)
    values = [v for v in raw_values if v.strip() not in nulls and v not in nulls]
    stripped = [v.strip() for v in values]
    kinds = [classify(v) for v in stripped]
    ctype = column_type(kinds)
    counts = {}
    for v in stripped:
        counts[v] = counts.get(v, 0) + 1
    distinct = len(counts)
    examples = list(counts)[:max_examples]
    col = {"name": name, "type": ctype, "rows": total, "non_null": len(values), "nulls": total - len(values),
           "null_percent": round(100 * (total - len(values)) / total, 2) if total else 0.0,
           "distinct": distinct, "unique": distinct == len(values) and len(values) > 0, "examples": examples, "hints": []}
    kind_counts = {}
    for k in kinds:
        kind_counts[k] = kind_counts.get(k, 0) + 1
    if len(kind_counts) > 1:
        col["type_counts"] = kind_counts
    hints = col["hints"]
    if ctype in ("integer", "number") and stripped:
        nums = [Decimal(v) for v in stripped]
        lo, hi = min(nums), max(nums)
        col["min"] = stripped[nums.index(lo)]
        col["max"] = stripped[nums.index(hi)]
        col["mean"] = float(round(sum(nums) / len(nums), 6))
        if ctype == "integer" and any(len(v.lstrip("+-")) > 1 and v.lstrip("+-").startswith("0") for v in stripped):
            col["type"] = "string"
            hints.append("zero-padded digits: a code, keep as text")
            for k in ("min", "max", "mean"):
                col.pop(k)
        elif ctype == "integer" and set(stripped) <= {"0", "1"}:
            hints.append("binary 0/1 flag")
    if ctype in ("date", "datetime"):
        fmt = date_format(stripped) if ctype == "date" else "ISO 8601 date-time"
        col["format"] = fmt
        keys = [date_key(v, fmt) for v in stripped]
        if "ambiguous" not in fmt:
            col["min"] = min(keys)
            col["max"] = max(keys)
    if ctype == "time":
        col["format"] = "HH:MM[:SS]"
    if ctype == "boolean":
        col["values"] = sorted(counts)
    if ctype == "string" and stripped:
        lengths = [len(v) for v in stripped]
        col["min_length"], col["max_length"] = min(lengths), max(lengths)
        if all(EMAIL.match(v) for v in stripped):
            hints.append("e-mail address")
        elif all(URL.match(v) for v in stripped):
            hints.append("URL")
        elif all(UUID.match(v) for v in stripped):
            hints.append("UUID")
    named_id = bool(ID_NAME.search(name if re.search(r"[a-z]Id$", name) else name.lower()))
    looks_id = col["unique"] and len(values) >= 3 and (ctype == "integer" or "UUID" in hints)
    if values and (named_id or looks_id):
        if col["unique"] and col["nulls"] == 0:
            hints.append("identifier: unique and never null")
        elif named_id:
            hints.append("named like an identifier but not unique or has nulls")
    if values and distinct == 1 and len(values) > 1:
        hints.append("constant: one value only")
    elif values and ctype in ("string", "integer", "boolean") and 1 < distinct <= min(20, len(values) // 2):
        hints.append("categorical")
        col["value_counts"] = dict(sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))[:20])
    if any(v != v.strip() for v in values):
        hints.append("leading or trailing whitespace")
    return col


def sniff(text):
    """The candidate delimiter that splits the first lines into the most consistent number of
    fields (ties: more fields, then the order , ; tab |)."""
    sample = "\n".join(text.splitlines()[:30])
    best, best_score = ",", None
    for order, candidate in enumerate((",", ";", "\t", "|")):
        try:
            counts = [len(r) for r in csv.reader(io.StringIO(sample, newline=""), delimiter=candidate) if r]
        except csv.Error:
            continue
        if not counts:
            continue
        mode = max(set(counts), key=counts.count)
        score = (mode > 1, counts.count(mode) / len(counts), mode, -order)
        if best_score is None or score > best_score:
            best, best_score = candidate, score
    return best


def markdown(dataset, rows, cols):
    esc = lambda s: str(s).replace("|", "\\|").replace("\n", " ")
    lines = [f"# Data dictionary: {esc(dataset)}", "", f"{rows} rows, {len(cols)} columns.", "",
             "| Column | Type | Nullable | Distinct | Example | Notes | Description |",
             "|:--|:--|:--|--:|:--|:--|:--|"]
    for c in cols:
        notes = "; ".join(c["hints"] + ([f"format {c['format']}"] if "format" in c else []) +
                          ([f"range {c['min']} to {c['max']}"] if "min" in c else []))
        example = c["examples"][0] if c["examples"] else ""
        lines.append(f"| {esc(c['name'])} | {c['type']} | {'yes' if c['nulls'] else 'no'} | {c['distinct']} | "
                     f"{esc(example)} | {esc(notes)} |  |")
    return "\n".join(lines) + "\n"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"csv", "path", "delimiter", "header", "null_values", "max_examples", "dataset"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    text = read_input(request)
    if len(text) > MAX_TEXT:
        fail(f"the CSV may be at most {MAX_TEXT} characters")
    text = text.lstrip("\ufeff")
    if not text.strip():
        fail("the CSV is empty")
    delimiter = request.get("delimiter", "auto")
    if delimiter not in ("auto", ",", ";", "\t", "|"):
        fail("delimiter must be auto, comma, semicolon, tab or pipe")
    if delimiter == "auto":
        delimiter = sniff(text)
    header = request.get("header", True)
    if not isinstance(header, bool):
        fail("header must be true or false")
    nulls = request.get("null_values", DEFAULT_NULLS)
    if not isinstance(nulls, list) or not all(isinstance(v, str) for v in nulls):
        fail("null_values must be a list of strings")
    nulls = set(nulls)
    max_examples = request.get("max_examples", 3)
    if isinstance(max_examples, bool) or not isinstance(max_examples, int) or not 0 <= max_examples <= 10:
        fail("max_examples must be an integer from 0 to 10")
    dataset = request.get("dataset", "dataset")
    if not isinstance(dataset, str) or len(dataset) > 200:
        fail("dataset must be a string of at most 200 characters")
    try:
        rows = [r for r in csv.reader(io.StringIO(text, newline=""), delimiter=delimiter)]
    except csv.Error as error:
        fail(f"CSV error: {error}")
    rows = [r for r in rows if any(cell.strip() for cell in r)]
    if not rows:
        fail("the CSV has no rows")
    warnings = []
    width = max(len(r) for r in rows)
    if header:
        names, body = rows[0], rows[1:]
    else:
        names, body = [], rows
    names = [n.strip() or f"column_{i + 1}" for i, n in enumerate(names)] + \
            [f"column_{i + 1}" for i in range(len(names), width)]
    seen = {}
    for i, n in enumerate(names):
        if n in seen:
            seen[n] += 1
            names[i] = f"{n}_{seen[n]}"
            warnings.append(f"repeated column name '{n}' renamed to '{names[i]}'")
        else:
            seen[n] = 1
    ragged = sum(1 for r in body if len(r) != len(names))
    if ragged:
        warnings.append(f"{ragged} row(s) have a different number of fields than the header; missing fields count as null")
    columns = []
    for i, name in enumerate(names):
        values = [r[i] if i < len(r) else "" for r in body]
        columns.append(describe(name, values, nulls, max_examples))
    skeleton = {"dataset": dataset, "rows": len(body), "columns": [
        {"name": c["name"], "type": c["type"], **({"format": c["format"]} if "format" in c else {}),
         "nullable": c["nulls"] > 0, "unique": c["unique"],
         "example": c["examples"][0] if c["examples"] else None,
         **({"allowed_values": list(c["value_counts"])} if "value_counts" in c else {}),
         **({"min": c["min"], "max": c["max"]} if "min" in c else {}),
         "description": "", "unit": "", "source": "", "notes": ""} for c in columns]}
    out = {"rows": len(body), "column_count": len(columns), "delimiter": delimiter, "header": header,
           "columns": columns, "dictionary": skeleton, "markdown": markdown(dataset, len(body), columns),
           "warnings": warnings}
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
