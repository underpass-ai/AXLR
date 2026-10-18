#!/usr/bin/env python3
"""Group-by and pivot tables over CSV text: group rows by key columns, aggregate value
columns (sum, count, mean, min, max, count distinct) in exact Decimal arithmetic, optionally
spread one column's values across the output columns (rows x columns, with a fill value for
empty cells), with row, column and grand totals computed from the underlying rows.

Input (JSON on stdin):
  csv         the CSV text (or path: a relative file)
  delimiter   override the sniffed delimiter
  group_by    the row key columns (may be empty for a single total row)
  columns     a column whose values become output columns (pivot); optional
  values      [{"column": "amount", "agg": "sum"}, ...]; agg: sum, count, mean, min, max, count_distinct
              (count without column counts rows)
  fill        the value for empty pivot cells, default null
  totals      true: add row totals (pivot), a total row and the grand total, default false
  decimals    round mean (and every other result) to this many places, half-up; default: exact
  sort        "key" (default) | "none" (first-seen order)
"""
import csv
import io
import json
import pathlib
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation, getcontext

AGGS = ("sum", "count", "mean", "min", "max", "count_distinct")
NUM = re.compile(r"^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$")
MAX_BYTES = 20_000_000
getcontext().prec = 34


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_text(request):
    if ("csv" in request) == ("path" in request):
        fail("give exactly one of csv (the CSV text) or path (relative)")
    if "csv" in request:
        if not isinstance(request["csv"], str):
            fail("csv must be a string")
        text = request["csv"]
    else:
        path = request["path"]
        if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path) or ".." in re.split(r"[\\/]", path):
            fail("path must be a relative path inside the working directory, without '..'")
        target = pathlib.Path(path)
        if not target.is_file():
            fail(f"no such file: {path}")
        if target.stat().st_size > MAX_BYTES:
            fail("the file is larger than 20 MB")
        text = target.read_bytes().decode("utf-8-sig", errors="replace")
    if len(text) > MAX_BYTES:
        fail("the CSV is larger than 20 MB")
    return text[1:] if text.startswith("﻿") else text


def parse(text, delimiter):
    if delimiter is None:
        try:
            delimiter = csv.Sniffer().sniff(text[:65536], delimiters=",;\t|").delimiter
        except csv.Error:
            delimiter = ","
    else:
        if not isinstance(delimiter, str) or len(delimiter.replace("\\t", "\t")) != 1:
            fail("delimiter must be a single character")
        delimiter = delimiter.replace("\\t", "\t")
    try:
        rows = [r for r in csv.reader(io.StringIO(text), delimiter=delimiter) if r and any(c.strip() for c in r)]
    except csv.Error as error:
        fail(f"the CSV could not be parsed: {error}")
    if not rows:
        fail("the CSV has no header row")
    header = [h.strip() for h in rows[0]]
    if len(set(header)) != len(header):
        fail("duplicate column names in the header")
    return header, [r + [""] * (len(header) - len(r)) for r in rows[1:]], delimiter


class Accumulator:
    def __init__(self, agg, column):
        self.agg, self.column = agg, column
        self.values = []

    def add(self, value, row_number):
        if self.agg == "count" and self.column is None:
            self.values.append(1)
            return
        if value.strip() == "":
            return
        if self.agg in ("sum", "mean"):
            text = value.strip()
            if not NUM.match(text):
                fail(f"row {row_number}: {self.column} = {value!r} is not a number ({self.agg} needs numbers)")
            self.values.append(Decimal(text))
        else:
            self.values.append(value.strip())

    def merge(self, others):
        merged = Accumulator(self.agg, self.column)
        for other in others:
            merged.values += other.values
        return merged

    def result(self, decimals):
        values = self.values
        if self.agg == "count":
            return len(values)
        if self.agg == "count_distinct":
            return len(set(values))
        if not values:
            return None
        if self.agg == "sum":
            out = sum(values, Decimal(0))
        elif self.agg == "mean":
            out = sum(values, Decimal(0)) / len(values)
        else:
            if all(NUM.match(v) for v in values):
                numbers = [Decimal(v) for v in values]
                out = min(numbers) if self.agg == "min" else max(numbers)
            else:
                return min(values) if self.agg == "min" else max(values)
        if decimals is not None:
            out = out.quantize(Decimal(1).scaleb(-decimals), rounding=ROUND_HALF_UP)
        elif self.agg == "mean":
            out = out.normalize() if out == out.to_integral_value() else out
        text = format(out, "f")
        if decimals is None and "." in text and self.agg == "mean":
            text = text.rstrip("0").rstrip(".")
        return text


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    known = {"csv", "path", "delimiter", "group_by", "columns", "values", "fill", "totals", "decimals", "sort"}
    unknown = sorted(set(request) - known)
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    header, data, delimiter = parse(read_text(request), request.get("delimiter"))
    group_by = request.get("group_by", [])
    if isinstance(group_by, str):
        group_by = [group_by]
    if not isinstance(group_by, list) or not all(isinstance(c, str) for c in group_by):
        fail("group_by must be a list of column names")
    pivot = request.get("columns")
    if pivot is not None and not isinstance(pivot, str):
        fail("columns must be one column name")
    for name in group_by + ([pivot] if pivot else []):
        if name not in header:
            fail(f"no column {name!r}; columns: {', '.join(header)}")
    if pivot in group_by:
        fail("the pivot column cannot also be a group_by column")
    specs = request.get("values", [{"agg": "count"}])
    if not isinstance(specs, list) or not specs:
        fail("values must be a non-empty list of {column, agg}")
    aggregates = []
    for spec in specs:
        if not isinstance(spec, dict) or set(spec) - {"column", "agg", "name"}:
            fail("each values item is {column, agg} (and an optional name)")
        agg = spec.get("agg", "sum")
        column = spec.get("column")
        if not isinstance(agg, str) or agg not in AGGS:
            fail(f"agg must be one of {', '.join(AGGS)}")
        if column is None and agg != "count":
            fail(f"{agg} needs a column")
        if column is not None and column not in header:
            fail(f"no column {column!r}; columns: {', '.join(header)}")
        if column is not None and not isinstance(column, str):
            fail("column must be a column name")
        if "name" in spec and not isinstance(spec["name"], str):
            fail("name must be a string")
        label = spec.get("name") or (f"{agg}({column})" if column else "count")
        aggregates.append((agg, column, label))
    fill = request.get("fill")
    totals = request.get("totals", False)
    if not isinstance(totals, bool):
        fail("totals must be true or false")
    decimals = request.get("decimals")
    if decimals is not None and (isinstance(decimals, bool) or not isinstance(decimals, int) or not 0 <= decimals <= 28):
        fail("decimals must be an integer from 0 to 28")
    sort = request.get("sort", "key")
    if sort not in ("key", "none"):
        fail("sort must be key or none")

    index = {name: i for i, name in enumerate(header)}
    cells = {}  # (group key, pivot value) -> [Accumulator per aggregate]
    group_order, pivot_order = [], []
    for number, row in enumerate(data, 2):
        key = tuple(row[index[c]].strip() for c in group_by)
        pv = row[index[pivot]].strip() if pivot else None
        if key not in cells:
            cells[key] = {}
            group_order.append(key)
        if pv not in cells[key]:
            cells[key][pv] = [Accumulator(agg, column) for agg, column, _ in aggregates]
        if pv not in pivot_order:
            pivot_order.append(pv)
        for acc, (agg, column, _) in zip(cells[key][pv], aggregates):
            acc.add(row[index[column]] if column else "", number)
    if not data:
        group_order = [tuple("" for _ in group_by)] if not group_by else []
        cells = {group_order[0]: {None: [Accumulator(a, c) for a, c, _ in aggregates]}} if group_order else {}
        pivot_order = [None] if not pivot else []

    def sort_key(value):
        return [(0, Decimal(v), "") if NUM.match(v) else (1, Decimal(0), v) for v in (value if isinstance(value, tuple) else (value,))]

    if sort == "key":
        group_order.sort(key=sort_key)
        pivot_order.sort(key=lambda v: sort_key(v if v is not None else ""))

    def cell_name(pv, label):
        if pivot is None:
            return label
        name = pv if pv != "" else "(empty)"
        return name if len(aggregates) == 1 else f"{name}|{label}"

    out_columns = list(group_by)
    for pv in pivot_order:
        for _, _, label in aggregates:
            out_columns.append(cell_name(pv, label))
    if totals and pivot:
        out_columns += [f"Total|{label}" if len(aggregates) > 1 else "Total" for _, _, label in aggregates]
    rows = []
    for key in group_order:
        record = dict(zip(group_by, key))
        for pv in pivot_order:
            accs = cells[key].get(pv)
            for position, (_, _, label) in enumerate(aggregates):
                record[cell_name(pv, label)] = accs[position].result(decimals) if accs else fill
        if totals and pivot:
            for position, (_, _, label) in enumerate(aggregates):
                merged = cells[key][next(iter(cells[key]))][position].merge([cells[key][pv][position] for pv in cells[key]])
                record[f"Total|{label}" if len(aggregates) > 1 else "Total"] = merged.result(decimals)
        rows.append(record)
    total_row = None
    if totals:
        total_row = {c: ("Total" if i == 0 else "") for i, c in enumerate(group_by)}
        all_accs = [accs for key in group_order for accs in cells[key].values()]
        for pv in pivot_order:
            column_accs = [cells[key][pv] for key in group_order if pv in cells[key]]
            for position, (_, _, label) in enumerate(aggregates):
                merged = Accumulator(*aggregates[position][:2]).merge([accs[position] for accs in column_accs])
                total_row[cell_name(pv, label)] = merged.result(decimals)
        if pivot:
            for position, (_, _, label) in enumerate(aggregates):
                merged = Accumulator(*aggregates[position][:2]).merge([accs[position] for accs in all_accs])
                total_row[f"Total|{label}" if len(aggregates) > 1 else "Total"] = merged.result(decimals)
    out = io.StringIO()
    writer = csv.writer(out, lineterminator="\n")
    writer.writerow(out_columns)
    for record in rows + ([total_row] if total_row else []):
        writer.writerow(["" if record.get(c) is None else record.get(c) for c in out_columns])
    print(json.dumps({"columns": out_columns, "rows": rows, "totals": total_row, "csv": out.getvalue(),
                      "groups": len(rows), "pivot_values": [pv for pv in pivot_order] if pivot else None,
                      "input_rows": len(data), "delimiter": delimiter,
                      "note": "sum, mean, min and max are exact decimals (as strings); empty cells are skipped; totals are computed from the rows, not from the cells"},
                     ensure_ascii=False))


if __name__ == "__main__":
    main()
