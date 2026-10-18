#!/usr/bin/env python3
"""Mail merge: CSV rows (given inline) through a text template with {{field}} and
{{field|default}} placeholders, one rendered document per row, with the missing fields of
each row reported and an optional file name per document.

Input (JSON on stdin):
  csv                CSV text with a header row
  template           text with {{field}} or {{field|default}} (spaces inside the braces allowed)
  filename_template  optional, same syntax, e.g. "letter_{{id}}.txt"
  delimiter          optional single character, default ","
  missing            "keep" (default: leave {{field}} in the text) | "empty" | "error" (exit 2)
  rows               optional list of 1-based data row numbers to render (default all)
A field is missing in a row when its column does not exist or the cell is empty; a default
after | is used in both cases.
"""
import csv
import io
import json
import re
import sys

PLACEHOLDER = re.compile(r"\{\{\s*([^{}|\s][^{}|]*?)\s*(?:\|([^{}]*))?\}\}")
UNSAFE = re.compile(r'[/\\:*?"<>|\x00-\x1f\x7f]')
MAX_ROWS = 10000


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def render(template, row, header, mode):
    """Returns (text, missing fields, defaulted fields)."""
    missing, defaulted = [], []

    def substitute(match):
        field, default = match.group(1), match.group(2)
        value = row.get(field, "") if field in header else ""
        if value != "":
            return value
        if default is not None:
            defaulted.append(field)
            return default
        missing.append({"field": field, "reason": "no such column" if field not in header else "empty cell"})
        return match.group(0) if mode == "keep" else ""

    return PLACEHOLDER.sub(substitute, template), missing, defaulted


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"csv", "template", "filename_template", "delimiter", "missing", "rows"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    text = request.get("csv")
    if not isinstance(text, str) or not text.strip():
        fail("csv must be non-empty CSV text with a header row")
    if len(text) > 5_000_000:
        fail("csv must be at most 5,000,000 characters")
    template = request.get("template")
    if not isinstance(template, str) or not template:
        fail("template must be a non-empty string")
    name_template = request.get("filename_template")
    if name_template is not None and (not isinstance(name_template, str) or not name_template.strip()):
        fail("filename_template must be a non-empty string")
    delimiter = request.get("delimiter", ",")
    if not isinstance(delimiter, str) or len(delimiter) != 1 or delimiter in "\"\r\n":
        fail("delimiter must be one character (not a quote or newline)")
    mode = request.get("missing", "keep")
    if mode not in ("keep", "empty", "error"):
        fail("missing must be keep, empty or error")

    text = text.lstrip("﻿")
    try:
        records = list(csv.reader(io.StringIO(text, newline=""), delimiter=delimiter, strict=True))
    except csv.Error as error:
        fail(f"csv could not be read: {error}")
    records = [r for r in records if any(cell.strip() for cell in r)]
    if not records:
        fail("csv has no header row")
    header = [h.strip() for h in records[0]]
    if any(h == "" for h in header):
        fail("csv header has an empty column name")
    duplicates = sorted({h for h in header if header.count(h) > 1})
    if duplicates:
        fail(f"csv header repeats column name(s): {', '.join(duplicates)}")
    data = records[1:]
    if len(data) > MAX_ROWS:
        fail(f"csv has more than {MAX_ROWS} data rows")
    wanted = request.get("rows")
    if wanted is not None:
        if not isinstance(wanted, list) or not all(isinstance(n, int) and not isinstance(n, bool) for n in wanted):
            fail("rows must be a list of 1-based row numbers")
        bad = [n for n in wanted if not 1 <= n <= len(data)]
        if bad:
            fail(f"rows out of range 1..{len(data)}: {bad}")
    selected = wanted if wanted is not None else list(range(1, len(data) + 1))

    placeholders = []
    for source in (template, name_template or ""):
        for m in PLACEHOLDER.finditer(source):
            if m.group(1) not in placeholders:
                placeholders.append(m.group(1))
    documents, problems, names = [], [], {}
    warnings = []
    for number in selected:
        cells = data[number - 1]
        if len(cells) != len(header):
            warnings.append(f"row {number} has {len(cells)} cells for {len(header)} columns")
        row = {h: (cells[i] if i < len(cells) else "") for i, h in enumerate(header)}
        body, missing, defaulted = render(template, row, header, mode)
        doc = {"row": number, "text": body, "missing": missing}
        if defaulted:
            doc["defaults_used"] = sorted(set(defaulted))
        if name_template is not None:
            raw, name_missing, _ = render(name_template, row, header, "empty")
            name = UNSAFE.sub("_", raw).strip(" .") or f"document_{number}"
            doc["filename"] = name
            names.setdefault(name, []).append(number)
            if name_missing:
                doc["filename_missing"] = name_missing
        if missing:
            problems.append({"row": number, "fields": [m["field"] for m in missing]})
        documents.append(doc)
    if mode == "error" and problems:
        first = problems[0]
        fail(f"{len(problems)} row(s) have missing fields; first: row {first['row']} lacks {', '.join(first['fields'])}")
    clashes = {n: rows for n, rows in names.items() if len(rows) > 1}
    if clashes:
        warnings.append("duplicate file names: " + "; ".join(f"{n} (rows {', '.join(map(str, r))})" for n, r in clashes.items()))
    unknown = [p for p in placeholders if p not in header]
    unused = [h for h in header if h not in placeholders]
    result = {
        "columns": header, "placeholders": placeholders,
        "placeholders_without_column": unknown, "columns_not_used": unused,
        "rows_total": len(data), "rendered": len(documents),
        "rows_with_missing": problems,
        "documents": documents,
        "warnings": warnings,
    }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
