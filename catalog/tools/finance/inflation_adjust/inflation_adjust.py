#!/usr/bin/env python3
"""Adjust an amount for inflation between two years or two months with a consumer price index:
amount x index(to) / index(from). Bundled series (cpi.json, fetched from BLS, Eurostat and INE
by tools/catalog/data/inflation_adjust.py; each answer names the source and the fetch date)
or any index given in the input. Decimal arithmetic, cents rounded half up.

Input (JSON on stdin):
  amount       number or decimal string
  from, to     a year (2020 or "2020") or a month ("2020-06"); both of the same kind
  series       "us_cpi_u" | "ea_hicp" | "es_ipc"                                or
  index        {"2020": 100, "2021": 103.2, ...}   your own index, keyed by year or "YYYY-MM"
  list_series  true: only list the bundled series and the periods they cover
"""
import json
import pathlib
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation, getcontext

getcontext().prec = 40
META = ("title", "agency", "series_id", "base", "currency", "source_url", "annual_method", "fetched", "dataset_updated")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(value, field, positive=False):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        number = Decimal(str(value).strip())
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite() or (positive and number <= 0):
        fail(f"{field} must be a {'positive' if positive else 'finite'} number")
    return number


def period(value, field):
    if isinstance(value, int) and not isinstance(value, bool):
        value = str(value)
    if not isinstance(value, str):
        fail(f"{field} must be a year (2020) or a month (\"2020-06\")")
    value = value.strip()
    if re.fullmatch(r"\d{4}", value):
        return value, "annual"
    match = re.fullmatch(r"(\d{4})-(\d{1,2})", value)
    if match and 1 <= int(match.group(2)) <= 12:
        return f"{match.group(1)}-{int(match.group(2)):02d}", "monthly"
    fail(f"{field} must be a year (2020) or a month (\"2020-06\")")


def years_between(a, b, kind):
    if kind == "annual":
        return Decimal(int(b) - int(a))
    months = (int(b[:4]) * 12 + int(b[5:])) - (int(a[:4]) * 12 + int(a[5:]))
    return Decimal(months) / 12


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"amount", "from", "to", "series", "index", "list_series"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field: {extra[0]}")
    data = json.loads((pathlib.Path(__file__).parent / "cpi.json").read_text())
    bundled = data["series"]
    if request.get("list_series"):
        listing = {}
        for name, s in bundled.items():
            item = {k: s[k] for k in META if k in s}
            annual, monthly = list(s["annual"]), list(s["monthly"])
            item["years"] = f"{annual[0]}-{annual[-1]}" if annual else None
            item["months"] = f"{monthly[0]} to {monthly[-1]}" if monthly else None
            listing[name] = item
        print(json.dumps({"series": listing, "not_fetched": data.get("not_fetched", {}), "generated": data["generated"]}))
        return
    if "amount" not in request:
        fail("amount is required")
    amount = dec(request["amount"], "amount")
    start, kind = period(request.get("from"), "from")
    end, kind_to = period(request.get("to"), "to")
    if kind != kind_to:
        fail("from and to must both be years or both be months")
    if ("series" in request) == ("index" in request):
        fail("give either series (a bundled one) or index (your own values)")
    if "series" in request:
        name = request["series"]
        if name not in bundled:
            missing = data.get("not_fetched", {})
            why = f" (not bundled: {missing[name]})" if name in missing else ""
            fail(f"unknown series {name!r}{why}; bundled: {', '.join(bundled)}")
        chosen = bundled[name]
        table = chosen[kind]
        meta = {k: chosen[k] for k in META if k in chosen}
        meta["generated_by"] = data["generator"]
    else:
        raw = request["index"]
        if not isinstance(raw, dict) or not raw:
            fail("index must be an object of period: value")
        table = {}
        for key, value in raw.items():
            norm, key_kind = period(key, f"index key {key!r}")
            if key_kind != kind:
                fail(f"index key {key!r} is not of the same kind as from and to")
            table[norm] = dec(value, f"index[{key!r}]", positive=True)
        name, meta = "given in the input", {"title": "index given in the input"}
    for label, key in (("from", start), ("to", end)):
        if key not in table:
            have = list(table)
            if have and min(have) <= key <= max(have):
                fail(f"{label} {key} is not in the {kind} series {name}: the agency published no value for it")
            span = f"{min(have)} to {max(have)}" if have else "none"
            fail(f"{label} {key} is not in the {kind} series {name} (available: {span})")
    first, last = Decimal(str(table[start])), Decimal(str(table[end]))
    if first <= 0 or last <= 0:
        fail("index values must be positive")
    factor = last / first
    result = {
        "amount": format(amount, "f"), "from": start, "to": end, "frequency": kind, "series": name,
        "from_index": format(first, "f"), "to_index": format(last, "f"),
        "factor": format(factor.quantize(Decimal("0.000001"), ROUND_HALF_UP), "f"),
        "adjusted_amount": format((amount * factor).quantize(Decimal("0.01"), ROUND_HALF_UP), "f"),
        "cumulative_inflation_pct": format(((factor - 1) * 100).quantize(Decimal("0.01"), ROUND_HALF_UP), "f"),
    }
    span = years_between(start, end, kind)
    if span != 0:
        rate = (factor.ln() / span).exp() - 1
        result["annualised_inflation_pct"] = format((rate * 100).quantize(Decimal("0.01"), ROUND_HALF_UP), "f")
    result["years"] = format(span.quantize(Decimal("0.01")) if kind == "monthly" else span, "f")
    result["source"] = meta
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except ArithmeticError:
        fail("the numbers are out of the range this tool can compute")
