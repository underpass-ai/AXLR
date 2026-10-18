#!/usr/bin/env python3
"""Break a public budget down: totals, per-capita amounts, each programme's share, year-over-year
changes and the largest items, from CSV text given in the input. Decimal arithmetic.

Input (JSON on stdin):
  csv            the budget as CSV text with a header row
  columns        {"programme": "...", "amount": "...", "year": "...", "population": "..."}
                 header names to use (defaults: programme, amount, year, population); year and
                 population columns are optional
  population     a number, or {"2025": 52000, ...} per year; overrides the population column
  delimiter      "," | ";" | "\\t" | "|"  (default: guessed from the header)
  decimal_comma  true when amounts are written 1.234,56 (default false: 1,234.56 or 1234.56)
  top            how many largest programmes to list per year (default 5)
"""
import csv
import io
import json
import re
import sys
from decimal import Decimal, InvalidOperation, ROUND_HALF_UP, localcontext

CENT = Decimal("0.01")
PLAIN = re.compile(r"^[+-]?(\d+|\d{1,3}(,\d{3})+)(\.\d+)?$")
COMMA = re.compile(r"^[+-]?(\d+|\d{1,3}(\.\d{3})+)(,\d+)?$")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def money(value):
    return str(value.quantize(CENT, rounding=ROUND_HALF_UP))


def percent(part, whole):
    if whole == 0:
        return None
    return str((part * 100 / whole).quantize(CENT, rounding=ROUND_HALF_UP))


def amount_of(text, decimal_comma, where):
    raw = text.strip().replace(" ", "").replace(" ", "")
    pattern = COMMA if decimal_comma else PLAIN
    if not pattern.match(raw):
        fail(f"{where}: {text!r} is not an amount" + (" (decimal_comma is on)" if decimal_comma else ""))
    if decimal_comma:
        raw = raw.replace(".", "").replace(",", ".")
    else:
        raw = raw.replace(",", "")
    return Decimal(raw)


def population_of(value, where):
    if isinstance(value, bool):
        fail(f"{where} must be a positive number")
    try:
        number = Decimal(str(value).strip().replace(",", "")) if isinstance(value, (int, float, str)) else None
    except InvalidOperation:
        number = None
    if number is None or not number.is_finite() or number <= 0:
        fail(f"{where} must be a positive number")
    return number


def year_key(year):
    return (0, int(year), year) if year.isdigit() else (1, 0, year)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    unknown = set(request) - {"csv", "columns", "population", "delimiter", "decimal_comma", "top"}
    if unknown:
        fail(f"unknown fields: {sorted(unknown)}")
    text = request.get("csv")
    if not isinstance(text, str) or not text.strip():
        fail("csv must be the budget as CSV text with a header row")
    if len(text) > 5_000_000:
        fail("csv is larger than 5 MB")
    columns = {"programme": "programme", "amount": "amount", "year": "year", "population": "population"}
    given = request.get("columns", {})
    if not isinstance(given, dict) or set(given) - set(columns) or not all(isinstance(v, str) for v in given.values()):
        fail("columns maps programme, amount, year and population to header names")
    columns.update(given)
    decimal_comma = request.get("decimal_comma", False)
    if not isinstance(decimal_comma, bool):
        fail("decimal_comma must be true or false")
    top = request.get("top", 5)
    if not isinstance(top, int) or isinstance(top, bool) or top < 0:
        fail("top must be a non-negative integer")
    header_line = text.lstrip("﻿").splitlines()[0]
    delimiter = request.get("delimiter")
    if delimiter is None:
        delimiter = max([",", ";", "\t", "|"], key=header_line.count)
    if delimiter not in (",", ";", "\t", "|"):
        fail("delimiter must be one of , ; tab |")
    reader = csv.DictReader(io.StringIO(text.lstrip("﻿")), delimiter=delimiter)
    header = [h.strip() for h in (reader.fieldnames or [])]
    reader.fieldnames = header
    for role in ("programme", "amount"):
        if columns[role] not in header:
            fail(f"the {role} column {columns[role]!r} is not in the header {header}")
    has_year = columns["year"] in header
    has_pop = columns["population"] in header
    if "year" in given and not has_year:
        fail(f"the year column {columns['year']!r} is not in the header {header}")
    if "population" in given and not has_pop:
        fail(f"the population column {columns['population']!r} is not in the header {header}")
    amounts, populations, rows = {}, {}, 0
    for number, row in enumerate(reader, start=2):
        if None in row:
            fail(f"line {number} has more fields than the header (check the delimiter and quoting)")
        if not any((v or "").strip() for v in row.values()):
            continue
        programme = (row.get(columns["programme"]) or "").strip()
        if not programme:
            fail(f"line {number}: empty programme")
        year = (row.get(columns["year"]) or "").strip() if has_year else "all"
        if not year:
            fail(f"line {number}: empty year")
        value = amount_of(row.get(columns["amount"]) or "", decimal_comma, f"line {number}")
        amounts.setdefault(year, {})
        amounts[year][programme] = amounts[year].get(programme, Decimal(0)) + value
        if has_pop and (row.get(columns["population"]) or "").strip():
            pop = population_of(row[columns["population"]], f"line {number} population")
            if year in populations and populations[year] != pop:
                fail(f"line {number}: population {pop} differs from {populations[year]} given earlier for {year}")
            populations[year] = pop
        rows += 1
    if not rows:
        fail("csv has no data rows")
    override = request.get("population")
    if override is not None:
        if isinstance(override, dict):
            populations = {str(y): population_of(v, f"population[{y}]") for y, v in override.items()}
        else:
            populations = {y: population_of(override, "population") for y in amounts}
    years = sorted(amounts, key=year_key)
    by_year = []
    with localcontext() as context:
        context.prec = 40
        for year in years:
            items = amounts[year]
            total = sum(items.values(), Decimal(0))
            pop = populations.get(year)
            ranked = sorted(items, key=lambda p: (-items[p], p))
            by_year.append({
                "year": year,
                "total": money(total),
                "population": (str(int(pop)) if pop == pop.to_integral_value() else str(pop)) if pop is not None else None,
                "per_capita": money(total / pop) if pop else None,
                "programmes": [{"programme": p, "amount": money(items[p]), "share_percent": percent(items[p], total),
                                "per_capita": money(items[p] / pop) if pop else None} for p in ranked],
                "top": [{"programme": p, "amount": money(items[p]), "share_percent": percent(items[p], total)}
                        for p in ranked[:top]],
            })
        changes = []
        for before, after in zip(years, years[1:]):
            a, b = amounts[before], amounts[after]
            ta, tb = sum(a.values(), Decimal(0)), sum(b.values(), Decimal(0))
            change = {"from": before, "to": after, "total_change": money(tb - ta),
                      "total_change_percent": percent(tb - ta, ta)}
            pa, pb = populations.get(before), populations.get(after)
            if pa and pb:
                ca, cb = ta / pa, tb / pb
                change["per_capita_change"] = money(cb - ca)
                change["per_capita_change_percent"] = percent(cb - ca, ca)
            names = sorted(set(a) | set(b), key=lambda p: (-(b.get(p, Decimal(0)) - a.get(p, Decimal(0))), p))
            change["programmes"] = []
            for p in names:
                old, new = a.get(p, Decimal(0)), b.get(p, Decimal(0))
                entry = {"programme": p, "from": money(old), "to": money(new), "change": money(new - old),
                         "change_percent": percent(new - old, old) if p in a else None}
                if p not in a:
                    entry["status"] = "new"
                elif p not in b:
                    entry["status"] = "dropped"
                change["programmes"].append(entry)
            changes.append(change)
    print(json.dumps({
        "rows": rows,
        "years": years,
        "by_year": by_year,
        "year_over_year": changes,
        "rounding": "amounts and per-capita figures to 0.01, percentages to 0.01, half up; sums use the unrounded values",
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
