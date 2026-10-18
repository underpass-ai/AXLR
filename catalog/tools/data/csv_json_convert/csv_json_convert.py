#!/usr/bin/env python3
"""Convert CSV to JSON and JSON to CSV. CSV to JSON nests dotted column names (a.b.c) into
objects and numeric parts (tags.0) into arrays, with optional type inference; JSON to CSV
flattens nested objects and arrays back into dotted columns, so the two directions round-trip.

Input (JSON on stdin):
  csv / json / json_text / path   the input: CSV text, a JSON value, JSON or JSON Lines text,
                                  or a relative file (.csv, .json, .jsonl)
  to            "json" or "csv" (default: the other format)
  delimiter     CSV delimiter: sniffed when reading, "," when writing
  nest          CSV to JSON: nest dotted keys, default true
  separator     key separator for nesting and flattening, default "."
  infer_types   CSV to JSON: numbers, true/false and null from text, default true
  empty         CSV to JSON: an empty cell becomes "null" (default), "omit" or "string" ("")
  format        CSV to JSON: "array" (default), "jsonl" or "keyed" (an object keyed by key_column)
  key_column    the column for format keyed
  arrays        JSON to CSV: "index" (default: a.0, a.1) or "json" (the array as JSON text in one cell)
"""
import csv
import io
import json
import pathlib
import re
import sys

INT = re.compile(r"^-?(0|[1-9]\d*)$")
NUM = re.compile(r"^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$")
MAX_BYTES = 20_000_000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_path(path):
    if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path) or ".." in re.split(r"[\\/]", path):
        fail("path must be a relative path inside the working directory, without '..'")
    target = pathlib.Path(path)
    if not target.is_file():
        fail(f"no such file: {path}")
    if target.stat().st_size > MAX_BYTES:
        fail("the file is larger than 20 MB")
    return target.read_bytes().decode("utf-8-sig", errors="replace")


def infer(text):
    """JSON-ish scalars from CSV text; numbers with leading zeros (zip codes, ids) stay text."""
    if INT.match(text):
        return int(text)
    if NUM.match(text):
        value = float(text)
        if value == value and value not in (float("inf"), float("-inf")):
            return value
    low = text.lower()
    if low == "true":
        return True
    if low == "false":
        return False
    if low == "null":
        return None
    return text


def set_path(target, parts, value, column):
    """Put value at the nested path; numeric parts make arrays."""
    node = target
    for depth, part in enumerate(parts):
        last = depth == len(parts) - 1
        following = None if last else parts[depth + 1]
        make = (lambda: []) if (following is not None and following.isdigit()) else (lambda: {})
        if isinstance(node, list):
            if not part.isdigit():
                fail(f"column {column!r}: {part!r} is not an array index")
            index = int(part)
            if index > 100000:
                fail(f"column {column!r}: array index too large")
            while len(node) <= index:
                node.append(None)
            if last:
                node[index] = value
            else:
                if node[index] is None:
                    node[index] = make()
                elif not isinstance(node[index], (dict, list)):
                    fail(f"column {column!r} conflicts with another column holding a value at the same path")
                node = node[index]
        else:
            if last:
                if part in node and isinstance(node[part], (dict, list)):
                    fail(f"column {column!r} conflicts with columns nested under it")
                node[part] = value
            else:
                if part not in node:
                    node[part] = make()
                elif not isinstance(node[part], (dict, list)):
                    fail(f"column {column!r} conflicts with column {'.'.join(parts[:depth + 1])!r}")
                node = node[part]


def csv_to_json(text, request):
    delimiter = request.get("delimiter")
    if delimiter is None:
        try:
            delimiter = csv.Sniffer().sniff(text[:65536], delimiters=",;\t|").delimiter
        except csv.Error:
            delimiter = ","
    else:
        delimiter = delimiter.replace("\\t", "\t")
        if len(delimiter) != 1:
            fail("delimiter must be a single character")
    try:
        rows = [r for r in csv.reader(io.StringIO(text), delimiter=delimiter) if r and any(c.strip() for c in r)]
    except csv.Error as error:
        fail(f"the CSV could not be parsed: {error}")
    if not rows:
        fail("the CSV has no header row")
    header = [h.strip() for h in rows[0]]
    if len(set(header)) != len(header):
        fail("duplicate column names in the header")
    nest = request.get("nest", True)
    infer_types = request.get("infer_types", True)
    empty = request.get("empty", "null")
    separator = request.get("separator", ".")
    form = request.get("format", "array")
    for field, value in (("nest", nest), ("infer_types", infer_types)):
        if not isinstance(value, bool):
            fail(f"{field} must be true or false")
    if empty not in ("null", "omit", "string"):
        fail("empty must be null, omit or string")
    if not isinstance(separator, str) or not separator:
        fail("separator must be a non-empty string")
    if form not in ("array", "jsonl", "keyed"):
        fail("format must be array, jsonl or keyed")
    records = []
    for number, row in enumerate(rows[1:], 2):
        if len(row) > len(header):
            fail(f"row {number} has {len(row)} cells, the header {len(header)}")
        record = {}
        for index, column in enumerate(header):
            cell = row[index] if index < len(row) else ""
            if cell == "":
                if empty == "omit":
                    continue
                value = None if empty == "null" else ""
            else:
                value = infer(cell) if infer_types else cell
            parts = column.split(separator) if nest and separator in column else [column]
            if any(p == "" for p in parts):
                fail(f"column {column!r} has an empty key part")
            set_path(record, parts, value, column)
        records.append(record)
    result = {"to": "json", "rows": len(records), "columns": header, "delimiter": delimiter}
    if form == "array":
        result["json"] = records
    elif form == "jsonl":
        result["jsonl"] = "".join(json.dumps(r, ensure_ascii=False) + "\n" for r in records)
    else:
        key = request.get("key_column")
        if key not in header:
            fail("format keyed needs key_column, one of the columns")
        keyed = {}
        for record, row in zip(records, rows[1:]):
            name = row[header.index(key)] if header.index(key) < len(row) else ""
            if name in keyed:
                fail(f"key_column value {name!r} appears twice")
            keyed[name] = record
        result["json"] = keyed
    return result


def flatten(value, prefix, separator, arrays, out):
    if isinstance(value, dict):
        if not value and prefix:
            out[prefix] = "{}"
        for key, child in value.items():
            if separator in str(key):
                fail(f"key {key!r} contains the separator {separator!r}; choose another separator")
            flatten(child, f"{prefix}{separator}{key}" if prefix else str(key), separator, arrays, out)
    elif isinstance(value, list):
        if arrays == "json" or not value:
            out[prefix] = json.dumps(value, ensure_ascii=False)
        else:
            for index, child in enumerate(value):
                flatten(child, f"{prefix}{separator}{index}" if prefix else str(index), separator, arrays, out)
    else:
        out[prefix or "value"] = value


def cell(value):
    if value is None:
        return ""
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, float) and value.is_integer() and abs(value) < 1e16:
        return repr(value)
    return str(value)


def json_to_csv(data, request):
    if isinstance(data, dict):
        data = [data]
    if not isinstance(data, list) or not data:
        fail("JSON to CSV needs an array of objects (or one object)")
    separator = request.get("separator", ".")
    if not isinstance(separator, str) or not separator:
        fail("separator must be a non-empty string")
    arrays = request.get("arrays", "index")
    if arrays not in ("index", "json"):
        fail("arrays must be index or json")
    delimiter = request.get("delimiter", ",").replace("\\t", "\t")
    if len(delimiter) != 1:
        fail("delimiter must be a single character")
    flat_rows, columns, seen = [], [], set()
    for index, item in enumerate(data):
        if not isinstance(item, dict):
            item = {"value": item}
        flat = {}
        flatten(item, "", separator, arrays, flat)
        for key in flat:
            if key not in seen:
                seen.add(key)
                columns.append(key)
        flat_rows.append(flat)
    out = io.StringIO()
    writer = csv.writer(out, delimiter=delimiter, lineterminator="\n")
    writer.writerow(columns)
    for flat in flat_rows:
        writer.writerow([cell(flat.get(c)) for c in columns])
    notes = ["null and missing values are both written as empty cells", "types are not stored in CSV: read back with infer_types to recover numbers and booleans"]
    if arrays == "index":
        notes.append("arrays become numbered columns (a.0, a.1); empty arrays and objects are written as [] and {}")
    return {"to": "csv", "rows": len(flat_rows), "columns": columns, "csv": out.getvalue(), "round_trip_notes": notes}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    known = {"csv", "json", "json_text", "path", "to", "delimiter", "nest", "separator", "infer_types", "empty", "format", "key_column", "arrays"}
    unknown = sorted(set(request) - known)
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    given = [k for k in ("csv", "json", "json_text", "path") if k in request]
    if len(given) != 1:
        fail("give exactly one input: csv, json, json_text or path")
    source = given[0]
    if "delimiter" in request and not isinstance(request["delimiter"], str):
        fail("delimiter must be a string")
    if source == "path":
        text = read_path(request["path"])
        source = "csv" if request["path"].lower().endswith((".csv", ".tsv", ".txt")) else "json_text"
        request = dict(request, **{source: text})
    target = request.get("to", "json" if source == "csv" else "csv")
    if target not in ("json", "csv"):
        fail("to must be json or csv")
    if source == "csv":
        if not isinstance(request["csv"], str):
            fail("csv must be a string")
        if target != "json":
            fail("the input is CSV: to must be json")
        text = request["csv"]
        if len(text) > MAX_BYTES:
            fail("the CSV is larger than 20 MB")
        print(json.dumps(csv_to_json(text[1:] if text.startswith("﻿") else text, request), ensure_ascii=False))
        return
    if target != "csv":
        fail("the input is JSON: to must be csv")
    if source == "json":
        data = request["json"]
    else:
        text = request["json_text"]
        if not isinstance(text, str):
            fail("json_text must be a string")
        try:
            data = json.loads(text)
        except ValueError:
            try:
                data = [json.loads(line) for line in text.splitlines() if line.strip()]
            except ValueError as error:
                fail(f"json_text is neither JSON nor JSON Lines: {error}")
    print(json.dumps(json_to_csv(data, request), ensure_ascii=False))


if __name__ == "__main__":
    main()
