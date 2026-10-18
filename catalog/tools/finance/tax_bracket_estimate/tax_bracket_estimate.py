#!/usr/bin/env python3
"""Progressive tax by brackets: the tax on a taxable base from a bundled dated table (Spanish
IRPF state scales, US federal 2025) or from brackets given in the input, with the amount and
rate of each bracket, the marginal and effective rates, and an optional tax-free allowance,
deducted from the base or, as Spain's mínimo personal y familiar, taxed at the scale and
subtracted. Decimal arithmetic, cents rounded half up. An estimate, never a tax return.

Input (JSON on stdin):
  income     taxable base (number or decimal string)
  table      a bundled table name                                     or
  brackets   [{"up_to": 12450, "rate": 9.5}, ..., {"up_to": null, "rate": 24.5}]   rates in percent
  allowance  optional amount
  allowance_method  "deduct_from_base" (default) | "tax_on_allowance"
  list_tables  true: only list the bundled tables
"""
import json
import pathlib
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation

DISCLAIMER = "Estimate only, not tax advice: real liability depends on deductions, credits, regional scales and rules this tool does not apply."


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        number = Decimal(str(value))
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite() or number < 0:
        fail(f"{field} must be a non-negative number")
    return number


def load_tables():
    return json.loads((pathlib.Path(__file__).parent / "brackets.json").read_text())


def scale(brackets):
    out, floor = [], Decimal(0)
    for i, (up_to, rate) in enumerate(brackets):
        rate = dec(rate, f"brackets[{i}].rate")
        if rate > 100:
            fail(f"brackets[{i}].rate is a percent and must be at most 100")
        top = None if up_to is None else dec(up_to, f"brackets[{i}].up_to")
        if top is not None and top <= floor:
            fail("brackets must rise: each up_to above the previous one")
        if top is None and i != len(brackets) - 1:
            fail("only the last bracket may have up_to null")
        out.append((floor, top, rate))
        floor = top if top is not None else floor
    if out[-1][1] is not None:
        fail("the last bracket must have up_to null (no upper limit)")
    return out


def tax(amount, steps):
    total, parts = Decimal(0), []
    for floor, top, rate in steps:
        if amount <= floor:
            break
        portion = (min(amount, top) if top is not None else amount) - floor
        due = portion * rate / 100
        total += due
        parts.append({"from": str(floor), "to": str(top) if top is not None else None, "rate": str(rate), "taxed": str(portion), "tax": str(due.quantize(Decimal("0.01"), ROUND_HALF_UP))})
    return total, parts


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    bundled = load_tables()
    if request.get("list_tables"):
        print(json.dumps({"generated": bundled["generated"], "tables": {k: {"title": v["title"], "year": v["year"], "currency": v["currency"], "source": v["source"]} for k, v in bundled["tables"].items()}}, ensure_ascii=False))
        return
    income = dec(request.get("income"), "income")
    meta = {}
    if "table" in request:
        table = bundled["tables"].get(request["table"])
        if table is None:
            fail(f"unknown table; bundled: {', '.join(sorted(bundled['tables']))}")
        steps = scale(table["brackets"])
        meta = {"table": request["table"], "title": table["title"], "year": table["year"], "table_transcribed": bundled["generated"], "currency": table["currency"], "source": table["source"], "note": table["note"]}
    elif isinstance(request.get("brackets"), list) and request["brackets"]:
        rows = []
        for i, b in enumerate(request["brackets"]):
            if not isinstance(b, dict) or "rate" not in b or "up_to" not in b:
                fail(f"brackets[{i}] needs up_to (null for the last) and rate")
            rows.append((b["up_to"], b["rate"]))
        steps = scale(rows)
        meta = {"table": "given in the input"}
    else:
        fail("give table (a bundled name) or brackets")
    allowance = dec(request.get("allowance", 0), "allowance")
    method = request.get("allowance_method", "deduct_from_base")
    if method not in ("deduct_from_base", "tax_on_allowance"):
        fail("allowance_method must be deduct_from_base or tax_on_allowance")
    base = max(income - allowance, Decimal(0)) if method == "deduct_from_base" else income
    gross, parts = tax(base, steps)
    relief = Decimal(0)
    if method == "tax_on_allowance" and allowance:
        relief, _ = tax(min(allowance, income), steps)
    due = (gross - relief).quantize(Decimal("0.01"), ROUND_HALF_UP)
    marginal = next((rate for floor, top, rate in steps if base > floor and (top is None or base <= top)), steps[0][2])
    result = {
        **meta, "income": str(income), "allowance": str(allowance), "allowance_method": method, "taxable_base": str(base),
        "brackets": parts, "tax_before_allowance": str(gross.quantize(Decimal("0.01"), ROUND_HALF_UP)),
        "allowance_relief": str(relief.quantize(Decimal("0.01"), ROUND_HALF_UP)), "tax": str(due),
        "marginal_rate": str(marginal), "effective_rate": str((due / income * 100).quantize(Decimal("0.01"), ROUND_HALF_UP)) if income else "0",
        "disclaimer": DISCLAIMER,
    }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
