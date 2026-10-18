#!/usr/bin/env python3
"""Invoice arithmetic in Decimal: line amounts with discounts, a global discount, tax bases
and VAT per rate, an optional equivalence surcharge per line, withholding on the base, and
the total due. Amounts are rounded half up to the currency's minor unit per line and per
rate, as Spanish and most EU invoicing rules require.

Input (JSON on stdin):
  lines   [{"description", "quantity", "unit_price", "discount_percent"?, "vat_percent",
            "surcharge_percent"?}, ...]  numbers as JSON numbers or decimal strings
  global_discount_percent   optional, applied to every line after its own discount
  withholding_percent       optional, withheld on the total tax base (e.g. IRPF 15)
  currency                  optional label, default "EUR"
  decimals                  optional minor-unit digits, default 2
"""
import json
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(value, field, default=None):
    if value is None:
        if default is None:
            fail(f"{field} is required")
        return Decimal(default)
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        number = Decimal(str(value))
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite():
        fail(f"{field} must be finite")
    return number


def percent(value, field):
    number = dec(value, field, "0")
    if number < 0 or number > 100:
        fail(f"{field} must be between 0 and 100")
    return number


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    lines = request.get("lines")
    if not isinstance(lines, list) or not lines:
        fail("lines must be a non-empty list")
    decimals = request.get("decimals", 2)
    if not isinstance(decimals, int) or isinstance(decimals, bool) or not 0 <= decimals <= 6:
        fail("decimals must be an integer from 0 to 6")
    unit = Decimal(1).scaleb(-decimals)
    money = lambda d: d.quantize(unit, rounding=ROUND_HALF_UP)
    show = lambda d: str(money(d))
    global_discount = percent(request.get("global_discount_percent"), "global_discount_percent")
    withholding = percent(request.get("withholding_percent"), "withholding_percent")
    hundred = Decimal(100)

    out_lines, by_rate = [], {}
    gross_total = discount_total = Decimal(0)
    for i, line in enumerate(lines):
        if not isinstance(line, dict):
            fail(f"lines[{i}] must be an object")
        quantity = dec(line.get("quantity"), f"lines[{i}].quantity")
        price = dec(line.get("unit_price"), f"lines[{i}].unit_price")
        discount = percent(line.get("discount_percent"), f"lines[{i}].discount_percent")
        vat = dec(line.get("vat_percent"), f"lines[{i}].vat_percent")
        surcharge = percent(line.get("surcharge_percent"), f"lines[{i}].surcharge_percent")
        if vat < 0 or vat > 100:
            fail(f"lines[{i}].vat_percent must be between 0 and 100")
        gross = money(quantity * price)
        after_line = money(gross * (hundred - discount) / hundred)
        base = money(after_line * (hundred - global_discount) / hundred)
        gross_total += gross
        discount_total += gross - base
        key = (vat, surcharge)
        by_rate[key] = by_rate.get(key, Decimal(0)) + base
        out_lines.append({"description": line.get("description", ""), "quantity": str(quantity), "unit_price": str(price),
                          "gross": show(gross), "discount": show(gross - base), "base": show(base), "vat_percent": str(vat)})

    taxes, base_total, vat_total, surcharge_total = [], Decimal(0), Decimal(0), Decimal(0)
    for (vat, surcharge), base in sorted(by_rate.items(), key=lambda kv: (-kv[0][0], -kv[0][1])):
        vat_amount = money(base * vat / hundred)
        surcharge_amount = money(base * surcharge / hundred)
        entry = {"vat_percent": str(vat), "base": show(base), "vat": show(vat_amount)}
        if surcharge:
            entry.update({"surcharge_percent": str(surcharge), "surcharge": show(surcharge_amount)})
        taxes.append(entry)
        base_total += base
        vat_total += vat_amount
        surcharge_total += surcharge_amount
    withheld = money(base_total * withholding / hundred)
    total = base_total + vat_total + surcharge_total - withheld
    result = {
        "currency": request.get("currency", "EUR"),
        "lines": out_lines,
        "gross": show(gross_total),
        "discounts": show(discount_total),
        "taxes": taxes,
        "base": show(base_total),
        "vat": show(vat_total),
        "surcharge": show(surcharge_total),
        "withholding_percent": str(withholding),
        "withholding": show(withheld),
        "total": show(total),
    }
    print(json.dumps(result))


if __name__ == "__main__":
    main()
