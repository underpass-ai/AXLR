#!/usr/bin/env python3
"""Depreciation schedules in Decimal: straight-line, declining balance at any factor or rate
(optionally switching to straight-line when that gives more), double-declining, the fixed-rate
declining balance of Excel's DB, sum-of-the-years'-digits and units-of-production, with a
salvage value and a partial first year counted in months. Each row gives the year's charge,
the accumulated depreciation and the book value.

Input (JSON on stdin):
  method      "straight_line" | "declining_balance" | "double_declining" |
              "fixed_declining_balance" | "sum_of_years_digits" | "units_of_production"
  cost        cost of the asset
  salvage     residual value at the end of its life (default 0)
  life        useful life in years (periods), an integer
  first_year_months   months of use in the first year, 1-12 (default 12); with fewer than 12
              the schedule has life + 1 rows
  factor      declining_balance: rate = factor / life (default 2)
  rate        declining_balance: a rate per year instead of the factor (0.3 or "30%")
  switch_to_straight_line   declining_balance, double_declining: take the straight-line charge
              on the remaining book value when it is larger (default false)
  total_units, units   units_of_production: lifetime units and the units of each year
  start_year  optional label of the first year
"""
import json
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation, getcontext

getcontext().prec = 40
METHODS = ("straight_line", "declining_balance", "double_declining", "fixed_declining_balance", "sum_of_years_digits", "units_of_production")
CENT = Decimal("0.01")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(value, field, percent=False):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    text = str(value).strip()
    scale = Decimal(1)
    if percent and text.endswith("%"):
        text, scale = text[:-1], Decimal("0.01")
    try:
        number = Decimal(text) * scale
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite():
        fail(f"{field} must be finite")
    return number


def money(x):
    return format(x.quantize(CENT, ROUND_HALF_UP), "f")


def fractions_of(life, months):
    """Share of a full year of use in each fiscal year."""
    if months == 12:
        return [Decimal(1)] * life
    first = Decimal(months) / 12
    return [first] + [Decimal(1)] * (life - 1) + [1 - first]


def straight_line(cost, salvage, life, months):
    annual = (cost - salvage) / life
    return [annual * f for f in fractions_of(life, months)]


def sum_of_years(cost, salvage, life, months):
    total = Decimal(life * (life + 1) // 2)
    full = [(cost - salvage) * (life - k) / total for k in range(life)]  # depreciation year k + 1
    if months == 12:
        return full
    first = Decimal(months) / 12
    out = [full[0] * first]
    for j in range(1, life):
        out.append(full[j - 1] * (1 - first) + full[j] * first)
    out.append(full[-1] * (1 - first))
    return out


def declining(cost, salvage, life, months, rate, switch):
    out, book, used = [], cost, Decimal(0)
    for fraction in fractions_of(life, months):
        charge = book * rate * fraction
        remaining = life - used
        if switch and remaining > 0:
            charge = max(charge, (book - salvage) * fraction / remaining)
        charge = max(Decimal(0), min(charge, book - salvage))
        out.append(charge)
        book -= charge
        used += fraction
    return out


def fixed_declining(cost, salvage, life, months):
    rate = (1 - (salvage / cost) ** (Decimal(1) / life)).quantize(Decimal("0.001"), ROUND_HALF_UP)
    out, book = [], cost
    for fraction in fractions_of(life, months):
        charge = book * rate * fraction
        out.append(charge)
        book -= charge
    return out, rate


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"method", "cost", "salvage", "life", "first_year_months", "factor", "rate", "switch_to_straight_line",
             "total_units", "units", "start_year"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field: {extra[0]}")
    method = request.get("method")
    if method not in METHODS:
        fail("method must be one of " + ", ".join(METHODS))
    if "cost" not in request:
        fail("cost is required")
    cost = dec(request["cost"], "cost")
    salvage = dec(request.get("salvage", 0), "salvage")
    if cost <= 0:
        fail("cost must be positive")
    if salvage < 0 or salvage >= cost:
        fail("salvage must be from 0 to below cost")
    months = request.get("first_year_months", 12)
    if isinstance(months, bool) or not isinstance(months, int) or not 1 <= months <= 12:
        fail("first_year_months must be an integer from 1 to 12")
    start_year = request.get("start_year", 1)
    if isinstance(start_year, bool) or not isinstance(start_year, int):
        fail("start_year must be an integer")
    switch = request.get("switch_to_straight_line", False)
    if not isinstance(switch, bool):
        fail("switch_to_straight_line must be true or false")
    result = {"method": method, "cost": money(cost), "salvage": money(salvage)}

    if method == "units_of_production":
        if "total_units" not in request or "units" not in request:
            fail("units_of_production needs total_units and units (a list, one per year)")
        total_units = dec(request["total_units"], "total_units")
        if total_units <= 0:
            fail("total_units must be positive")
        raw = request["units"]
        if not isinstance(raw, list) or not 1 <= len(raw) <= 200:
            fail("units must be a list of 1 to 200 yearly amounts")
        per_unit = (cost - salvage) / total_units
        charges, book = [], cost
        for i, u in enumerate(raw):
            used = dec(u, f"units[{i}]")
            if used < 0:
                fail(f"units[{i}] must not be negative")
            charge = min(used * per_unit, book - salvage)
            charges.append(charge)
            book -= charge
        result["rate_per_unit"] = format(per_unit.quantize(Decimal("0.000001"), ROUND_HALF_UP), "f")
        if sum(dec(u, "units") for u in raw) > total_units:
            result["note"] = "the units exceed total_units: depreciation stops at the salvage value"
    else:
        life = request.get("life")
        if isinstance(life, bool) or not isinstance(life, int) or not 1 <= life <= 200:
            fail("life must be an integer number of years from 1 to 200")
        result["life"] = life
        result["first_year_months"] = months
        if method == "straight_line":
            charges = straight_line(cost, salvage, life, months)
        elif method == "sum_of_years_digits":
            charges = sum_of_years(cost, salvage, life, months)
        elif method == "fixed_declining_balance":
            if switch:
                fail("switch_to_straight_line applies to declining_balance and double_declining")
            charges, rate = fixed_declining(cost, salvage, life, months)
            result["rate"] = format(rate, "f")
        else:
            if method == "double_declining":
                if "rate" in request or "factor" in request:
                    fail("double_declining has factor 2; use declining_balance for another factor or rate")
                rate = Decimal(2) / life
            elif "rate" in request:
                if "factor" in request:
                    fail("give factor or rate, not both")
                rate = dec(request["rate"], "rate", percent=True)
            else:
                factor = dec(request.get("factor", 2), "factor")
                if factor <= 0:
                    fail("factor must be positive")
                rate = factor / life
            if not 0 < rate <= 1:
                fail("the declining rate must be above 0 and at most 100%")
            charges = declining(cost, salvage, life, months, rate, switch)
            result["rate"] = format(rate.quantize(Decimal("0.000001"), ROUND_HALF_UP).normalize(), "f")
            result["switch_to_straight_line"] = switch
    rows, accumulated = [], Decimal(0)
    for k, charge in enumerate(charges):
        start = cost - accumulated
        accumulated += charge
        rows.append({"year": start_year + k, "book_value_start": money(start), "depreciation": money(charge),
                     "accumulated": money(accumulated), "book_value_end": money(cost - accumulated)})
    result["total_depreciation"] = money(accumulated)
    result["final_book_value"] = money(cost - accumulated)
    left = cost - accumulated - salvage
    if left.quantize(CENT, ROUND_HALF_UP) != 0:
        result["undepreciated_above_salvage"] = money(left)
    result["schedule"] = rows
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except ArithmeticError:
        fail("the numbers are out of the range this tool can compute")
