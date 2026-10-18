#!/usr/bin/env python3
"""Per-column statistics of a CSV, as csvkit's csvstat gives them: inferred type, nulls,
minimum, maximum, sum, mean, median, standard deviation, distinct values, top values and
text lengths. The dialect (delimiter, quoting) is sniffed unless given. Numbers are summed
exactly (Fraction) and printed as decimal strings.

Input (JSON on stdin):
  csv          the CSV text (or path: a relative file in the working directory)
  delimiter    override the sniffed delimiter (",", ";", "\t", "|", ...)
  header       true (default): the first row holds column names
  null_values  strings counted as null, default ["", "NA", "N/A", "null", "NULL", "None", "nan", "NaN", "-"]
  top          how many top values per column, default 5
  columns      restrict to these column names
  precision    significant digits for mean, median and stdev, default 28
"""
import csv
import datetime as dt
import io
import json
import pathlib
import re
import sys
from decimal import Decimal, getcontext
from fractions import Fraction

NULLS = ["", "NA", "N/A", "null", "NULL", "None", "nan", "NaN", "-"]
INT = re.compile(r"^[+-]?\d+$")
NUM = re.compile(r"^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$")
DATE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
DATETIME = re.compile(r"^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?$")
TRUE = {"true", "yes", "t", "y"}
FALSE = {"false", "no", "f", "n"}
MAX_BYTES = 20_000_000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_text(request, field="csv"):
    if field in request and "path" in request:
        fail(f"give {field} or path, not both")
    if field in request:
        if not isinstance(request[field], str):
            fail(f"{field} must be a string")
        return request[field]
    if "path" in request:
        path = request["path"]
        if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path) or ".." in re.split(r"[\\/]", path):
            fail("path must be a relative path inside the working directory, without '..'")
        target = pathlib.Path(path)
        if not target.is_file():
            fail(f"no such file: {path}")
        if target.stat().st_size > MAX_BYTES:
            fail("the file is larger than 20 MB")
        data = target.read_bytes()
        if data.startswith(b"\xef\xbb\xbf"):
            data = data[3:]
        return data.decode("utf-8", errors="replace")
    fail(f"{field} is required (the CSV text), or path")


def read_rows(text, delimiter=None):
    if text.startswith("﻿"):
        text = text[1:]
    if len(text) > MAX_BYTES:
        fail("the CSV is larger than 20 MB")
    if delimiter is not None:
        if not isinstance(delimiter, str) or len(delimiter.replace("\\t", "\t")) != 1:
            fail("delimiter must be a single character")
        dialect_delimiter = delimiter.replace("\\t", "\t")
        sniffed = False
    else:
        sample = text[:65536]
        try:
            dialect_delimiter = csv.Sniffer().sniff(sample, delimiters=",;\t|").delimiter
            sniffed = True
        except csv.Error:
            dialect_delimiter, sniffed = ",", False
    try:
        rows = list(csv.reader(io.StringIO(text), delimiter=dialect_delimiter))
    except csv.Error as error:
        fail(f"the CSV could not be parsed: {error}")
    rows = [row for row in rows if row and any(cell.strip() for cell in row)]
    return rows, dialect_delimiter, sniffed


def to_decimal_string(value, precision):
    if value.denominator == 1:
        return str(value.numerator)
    getcontext().prec = precision
    result = Decimal(value.numerator) / Decimal(value.denominator)
    text = format(result.normalize(), "f")
    return text


def number(text):
    if NUM.match(text):
        return Fraction(Decimal(text))
    return None


def parse_datetime(text):
    try:
        value = text.replace("Z", "+00:00").replace(" ", "T", 1)
        if re.search(r"[+-]\d{4}$", value):
            value = value[:-2] + ":" + value[-2:]
        return dt.datetime.fromisoformat(value)
    except ValueError:
        return None


def infer(values):
    """The narrowest type every non-null value fits: integer, decimal, boolean, date, datetime or text."""
    if not values:
        return "empty"
    lower = [v.lower() for v in values]
    if all(INT.match(v) for v in values):
        return "integer"
    if all(NUM.match(v) for v in values):
        return "decimal"
    if all(v in TRUE or v in FALSE for v in lower):
        return "boolean"
    if all(DATE.match(v) for v in values):
        try:
            for v in values:
                dt.date.fromisoformat(v)
            return "date"
        except ValueError:
            return "text"
    if all(DATETIME.match(v) or DATE.match(v) for v in values) and all(parse_datetime(v) for v in values):
        return "datetime"
    return "text"


def column_stats(name, cells, nulls, top, precision):
    present = [c.strip() for c in cells if c.strip() not in nulls and c not in nulls]
    kind = infer(present)
    counts = {}
    for value in present:
        counts[value] = counts.get(value, 0) + 1
    ranked = sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))
    stats = {"column": name, "type": kind, "count": len(cells), "nulls": len(cells) - len(present), "non_null": len(present),
             "distinct": len(counts), "unique": len(counts) == len(present) and bool(present),
             "top": [{"value": v, "count": c} for v, c in ranked[:top]]}
    if kind in ("integer", "decimal"):
        nums = sorted(number(v) for v in present)
        total = sum(nums, Fraction(0))
        n = len(nums)
        mean = total / n
        middle = n // 2
        median = nums[middle] if n % 2 else (nums[middle - 1] + nums[middle]) / 2
        stats.update({"min": to_decimal_string(nums[0], precision), "max": to_decimal_string(nums[-1], precision),
                      "sum": to_decimal_string(total, precision), "mean": to_decimal_string(mean, precision),
                      "median": to_decimal_string(median, precision)})
        if n > 1:
            variance = sum(((x - mean) ** 2 for x in nums), Fraction(0)) / (n - 1)
            getcontext().prec = precision
            root = (Decimal(variance.numerator) / Decimal(variance.denominator)).sqrt()
            stats["stdev"] = format(root.normalize(), "f")
            stats["variance"] = to_decimal_string(variance, precision)
        decimals = [len(v.split(".")[1].split("e")[0].split("E")[0]) if "." in v else 0 for v in present]
        stats["max_decimal_places"] = max(decimals)
        stats["zeros"] = sum(1 for x in nums if x == 0)
    elif kind == "boolean":
        stats.update({"true": sum(1 for v in present if v.lower() in TRUE), "false": sum(1 for v in present if v.lower() in FALSE)})
    elif kind in ("date", "datetime"):
        if kind == "date":
            parsed = sorted(dt.date.fromisoformat(v) for v in present)
            stats.update({"min": parsed[0].isoformat(), "max": parsed[-1].isoformat(), "span_days": (parsed[-1] - parsed[0]).days})
            middle = len(parsed) // 2
            stats["median"] = parsed[middle].isoformat() if len(parsed) % 2 else (parsed[middle - 1] + (parsed[middle] - parsed[middle - 1]) / 2).isoformat()
        else:
            items = [(parse_datetime(v), v) for v in present]
            aware = [d.tzinfo is not None for d, _ in items]
            if all(aware) or not any(aware):
                items.sort(key=lambda item: item[0])
                stats.update({"min": items[0][1], "max": items[-1][1]})
            else:
                stats["note"] = "mixes values with and without a time zone; min and max not computed"
    if kind not in ("integer", "decimal"):
        if present:
            lengths = [len(v) for v in present]
            stats["min_length"] = min(lengths)
            stats["max_length"] = max(lengths)
            if kind == "text":
                stats["min"] = min(present)
                stats["max"] = max(present)
    return stats


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"csv", "path", "delimiter", "header", "null_values", "top", "columns", "precision"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    text = read_text(request)
    header = request.get("header", True)
    if not isinstance(header, bool):
        fail("header must be true or false")
    nulls = request.get("null_values", NULLS)
    if not isinstance(nulls, list) or not all(isinstance(v, str) for v in nulls):
        fail("null_values must be a list of strings")
    top = request.get("top", 5)
    precision = request.get("precision", 28)
    for field, value, low, high in (("top", top, 0, 1000), ("precision", precision, 1, 100)):
        if isinstance(value, bool) or not isinstance(value, int) or not low <= value <= high:
            fail(f"{field} must be an integer from {low} to {high}")
    rows, delimiter, sniffed = read_rows(text, request.get("delimiter"))
    if not rows:
        fail("the CSV has no rows")
    width = max(len(row) for row in rows)
    if header:
        names = [n.strip() or f"column_{i + 1}" for i, n in enumerate(rows[0])] + [f"column_{i + 1}" for i in range(len(rows[0]), width)]
        data = rows[1:]
    else:
        names = [f"column_{i + 1}" for i in range(width)]
        data = rows
    ragged = sum(1 for row in data if len(row) != len(names))
    wanted = request.get("columns")
    if wanted is not None:
        if not isinstance(wanted, list) or not all(isinstance(c, str) for c in wanted):
            fail("columns must be a list of column names")
        missing = [c for c in wanted if c not in names]
        if missing:
            fail(f"no such column: {missing[0]} (columns: {', '.join(names)})")
    indexes = [i for i, n in enumerate(names) if wanted is None or n in wanted]
    columns = [column_stats(names[i], [row[i] if i < len(row) else "" for row in data], set(nulls), top, precision) for i in indexes]
    print(json.dumps({"rows": len(data), "columns": len(names), "delimiter": delimiter, "delimiter_sniffed": sniffed,
                      "header": header, "ragged_rows": ragged, "stats": columns}))


if __name__ == "__main__":
    main()
