#!/usr/bin/env python3
"""Compound-interest arithmetic in Decimal: future and present value of a sum and of a level
annuity (ordinary or due), effective annual rate from a nominal rate with m compoundings a year
or continuous compounding, the nominal rate from an effective one, the doubling time (exact and
by the rules of 72, 70 and 69.3) and the real rate by Fisher's equation.

Input (JSON on stdin):
  operation        "future_value" | "present_value" | "effective_rate" | "nominal_rate"
                   | "doubling_time" | "real_rate"
  rate             nominal annual rate: 0.05 or "5%"
  compounding      compoundings (and payments) a year, default 1, or "continuous"
  years | periods  term (periods = years x compounding)                    value operations
  present_value    future_value: the sum invested now (default 0)
  future_value     present_value: the sum wanted at the end (default 0)
  payment          level payment each period (default 0)
  due              true: payments at the start of each period (annuity due)
  effective_rate   nominal_rate: the effective annual rate to convert
  inflation        real_rate: the inflation rate
Amounts are positive; the sign convention of spreadsheets is not used.
"""
import json
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation, getcontext

getcontext().prec = 50
CENT = Decimal("0.01")
OPERATIONS = ("future_value", "present_value", "effective_rate", "nominal_rate", "doubling_time", "real_rate")


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


def rate_out(x):
    """A rate as a fraction to 10 places and as a percent to 4."""
    return format(x.quantize(Decimal("1e-10"), ROUND_HALF_UP), "f"), format((x * 100).quantize(Decimal("0.0001"), ROUND_HALF_UP), "f")


def fixed(x, places):
    return format(x.quantize(Decimal(1).scaleb(-places), ROUND_HALF_UP), "f")


def compounding_of(request):
    m = request.get("compounding", 1)
    if m == "continuous":
        return None
    if isinstance(m, bool) or not isinstance(m, int) or not 1 <= m <= 100000:
        fail("compounding must be an integer from 1 to 100000 or \"continuous\"")
    return m


def need(request, field, percent=False):
    if field not in request:
        fail(f"{field} is required for this operation")
    return dec(request[field], field, percent)


def ear(rate, m):
    return rate.exp() - 1 if m is None else (1 + rate / m) ** m - 1


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"operation", "rate", "compounding", "years", "periods", "present_value", "future_value", "payment", "due",
             "effective_rate", "inflation"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field: {extra[0]}")
    operation = request.get("operation")
    if operation not in OPERATIONS:
        fail("operation must be one of " + ", ".join(OPERATIONS))
    m = compounding_of(request)
    result = {"operation": operation, "compounding": "continuous" if m is None else m}

    if operation in ("future_value", "present_value"):
        rate = need(request, "rate", True)
        if rate <= -1:
            fail("rate must be greater than -100%")
        if ("years" in request) == ("periods" in request):
            fail("give either years or periods")
        if "periods" in request:
            if m is None:
                fail("continuous compounding takes years, not periods")
            periods = request["periods"]
            if isinstance(periods, bool) or not isinstance(periods, int) or periods < 0:
                fail("periods must be a non-negative integer")
            n = Decimal(periods)
            years = n / m
        else:
            years = dec(request["years"], "years")
            if years < 0:
                fail("years must not be negative")
            n = years * m if m is not None else None
        sum_field = "present_value" if operation == "future_value" else "future_value"
        lump = dec(request.get(sum_field, 0), sum_field)
        payment = dec(request.get("payment", 0), "payment")
        due = request.get("due", False)
        if not isinstance(due, bool):
            fail("due must be true or false")
        if m is None:
            if payment:
                fail("payments need a whole number of periods: give compounding as an integer")
            growth = (rate * years).exp()
            annuity_factor = Decimal(0)
        else:
            i = rate / m
            if payment and n != n.to_integral_value():
                fail("with a payment, years x compounding must be a whole number of periods")
            growth = (1 + i) ** n
            if payment:
                annuity_factor = n if i == 0 else (growth - 1) / i
                if due:
                    annuity_factor *= 1 + i
            else:
                annuity_factor = Decimal(0)
            result["period_rate"] = rate_out(i)[0]
            result["periods"] = format(n.normalize(), "f")
        result["years"] = format(years.quantize(Decimal("1e-6"), ROUND_HALF_UP).normalize(), "f")
        if operation == "future_value":
            lump_part = lump * growth
            annuity_part = payment * annuity_factor
        else:
            lump_part = lump / growth
            annuity_part = payment * annuity_factor / growth
        result[operation] = money(lump_part + annuity_part)
        result["from_lump_sum"] = money(lump_part)
        result["from_payments"] = money(annuity_part)
        if payment:
            result["annuity"] = "due" if due else "ordinary"
            result["total_payments"] = money(payment * (n if m is not None else 0))
        result["growth_factor"] = fixed(growth, 10)

    elif operation == "effective_rate":
        rate = need(request, "rate", True)
        if m is not None and rate <= -m:
            fail("rate is too negative for this compounding")
        effective = ear(rate, m)
        result["nominal_rate"] = rate_out(rate)[0]
        result["effective_rate"], result["effective_rate_pct"] = rate_out(effective)

    elif operation == "nominal_rate":
        effective = need(request, "effective_rate", True)
        if effective <= -1:
            fail("effective_rate must be greater than -100%")
        if m is None:
            nominal = (1 + effective).ln()
        else:
            nominal = m * ((1 + effective) ** (Decimal(1) / m) - 1)
        result["effective_rate"] = rate_out(effective)[0]
        result["nominal_rate"], result["nominal_rate_pct"] = rate_out(nominal)

    elif operation == "doubling_time":
        rate = need(request, "rate", True)
        if rate <= 0:
            fail("rate must be positive to double")
        two = Decimal(2).ln()
        if m is None:
            exact = two / rate
        else:
            exact = two / (m * (1 + rate / m).ln())
        pct = rate * 100
        result["rate_pct"] = fixed(pct, 6).rstrip("0").rstrip(".")
        result["exact_years"] = fixed(exact, 6)
        rules = {}
        for k in ("72", "70", "69.3"):
            estimate = Decimal(k) / pct
            rules[f"rule_of_{k.replace('.', '_')}"] = {"years": fixed(estimate, 6), "error_years": fixed(estimate - exact, 6)}
        result.update(rules)
        result["effective_rate"] = rate_out(ear(rate, m))[0]

    else:  # real_rate
        rate = need(request, "rate", True)
        inflation = need(request, "inflation", True)
        if inflation <= -1:
            fail("inflation must be greater than -100%")
        real = (1 + rate) / (1 + inflation) - 1
        result["nominal_rate"] = rate_out(rate)[0]
        result["inflation"] = rate_out(inflation)[0]
        result["real_rate"], result["real_rate_pct"] = rate_out(real)
        result["approximation"], result["approximation_pct"] = rate_out(rate - inflation)
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except ArithmeticError:
        fail("the numbers are out of the range this tool can compute")
