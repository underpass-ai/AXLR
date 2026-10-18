#!/usr/bin/env python3
"""Loan amortization: the instalment and the full schedule for the French (constant payment),
German (constant principal) and American (interest only, principal at the end) systems, with
early repayments that shorten the term or lower the instalment, and the totals. Decimal
arithmetic: each period's interest is rounded to the cent and the last payment absorbs the
remainder so the balance ends at exactly zero.

Input (JSON on stdin):
  principal          amount borrowed
  annual_rate        nominal annual rate: 0.05 or "5%"
  periods            number of payments                          or
  years              term in years (years x payments_per_year must be whole)
  payments_per_year  default 12
  system             "french" (default) | "german" | "american"
  early_repayments   [{"period": 24, "amount": 5000, "mode": "reduce_term" | "reduce_payment"}]
                     paid right after that period's regular payment
  schedule           default true; false leaves the rows out
"""
import json
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation, getcontext

getcontext().prec = 40
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


def cents(x):
    return x.quantize(CENT, ROUND_HALF_UP)


def money(x):
    return format(cents(x), "f")


def annuity(balance, rate, periods):
    if periods <= 0:
        return balance
    if rate == 0:
        return cents(balance / periods)
    return cents(balance * rate / (1 - (1 + rate) ** -periods))


def amortize(principal, rate, periods, system, extras):
    balance, end = principal, periods
    payment = annuity(principal, rate, periods) if system == "french" else None
    step = cents(principal / periods) if system == "german" else None
    rows, total_interest, total_extra = [], Decimal(0), Decimal(0)
    k = 0
    while balance > 0:
        k += 1
        interest = cents(balance * rate)
        if system == "french":
            principal_part = payment - interest
        elif system == "german":
            principal_part = step
        else:
            principal_part = Decimal(0)
        if k >= end or principal_part >= balance:
            principal_part = balance
        balance -= principal_part
        extra = Decimal(0)
        for item in extras.get(k, []):
            if balance <= 0:
                break
            paid = min(item["amount"], balance)
            balance -= paid
            extra += paid
            remaining = end - k
            if item["mode"] == "reduce_payment" and balance > 0 and remaining > 0:
                if system == "french":
                    payment = annuity(balance, rate, remaining)
                elif system == "german":
                    step = cents(balance / remaining)
        total_interest += interest
        total_extra += extra
        rows.append({"period": k, "payment": money(interest + principal_part), "interest": money(interest),
                     "principal": money(principal_part), "extra": money(extra), "balance": money(balance)})
    return rows, total_interest, total_extra


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    known = {"principal", "annual_rate", "periods", "years", "payments_per_year", "system", "early_repayments", "schedule"}
    extra = sorted(set(request) - known)
    if extra:
        fail(f"unknown field: {extra[0]}")
    if "principal" not in request or "annual_rate" not in request:
        fail("principal and annual_rate are required")
    principal = dec(request["principal"], "principal")
    if principal <= 0:
        fail("principal must be positive")
    principal = cents(principal)
    annual_rate = dec(request["annual_rate"], "annual_rate", percent=True)
    if annual_rate < 0:
        fail("annual_rate must not be negative")
    ppy = request.get("payments_per_year", 12)
    if isinstance(ppy, bool) or not isinstance(ppy, int) or not 1 <= ppy <= 365:
        fail("payments_per_year must be an integer from 1 to 365")
    if ("periods" in request) == ("years" in request):
        fail("give either periods or years")
    if "periods" in request:
        periods = request["periods"]
        if isinstance(periods, bool) or not isinstance(periods, int):
            fail("periods must be an integer")
    else:
        total = dec(request["years"], "years") * ppy
        if total != total.to_integral_value():
            fail("years x payments_per_year must be a whole number of payments")
        periods = int(total)
    if not 1 <= periods <= 12000:
        fail("the number of payments must be from 1 to 12000")
    system = request.get("system", "french")
    if system not in ("french", "german", "american"):
        fail("system must be french, german or american")
    raw = request.get("early_repayments", [])
    if not isinstance(raw, list):
        fail("early_repayments must be a list")
    extras = {}
    for i, item in enumerate(raw):
        if not isinstance(item, dict) or set(item) - {"period", "amount", "mode"}:
            fail(f"early_repayments[{i}] must be an object with period, amount and mode")
        period = item.get("period")
        if isinstance(period, bool) or not isinstance(period, int) or not 1 <= period <= periods:
            fail(f"early_repayments[{i}].period must be an integer from 1 to {periods}")
        amount = cents(dec(item.get("amount"), f"early_repayments[{i}].amount"))
        if amount <= 0:
            fail(f"early_repayments[{i}].amount must be positive")
        mode = item.get("mode", "reduce_term")
        if mode not in ("reduce_term", "reduce_payment"):
            fail(f"early_repayments[{i}].mode must be reduce_term or reduce_payment")
        extras.setdefault(period, []).append({"amount": amount, "mode": mode})
    show = request.get("schedule", True)
    if not isinstance(show, bool):
        fail("schedule must be true or false")
    rate = annual_rate / ppy
    rows, interest, extra_paid = amortize(principal, rate, periods, system, extras)
    regular = sum(Decimal(r["payment"]) for r in rows)
    result = {
        "system": system, "principal": money(principal), "annual_rate": format(annual_rate, "f"),
        "period_rate": format(rate.quantize(Decimal("1e-12"), ROUND_HALF_UP).normalize(), "f"),
        "payments_per_year": ppy, "periods_contracted": periods, "periods_paid": len(rows),
        "payment": rows[0]["payment"], "last_payment": rows[-1]["payment"],
        "totals": {"paid": money(regular + extra_paid), "interest": money(interest), "principal": money(principal),
                   "early_repayments": money(extra_paid)},
    }
    if system == "american":
        result["note"] = "American system: early repayments lower the balance and the interest; the rest of the principal is still due at the end"
    if extras:
        base_rows, base_interest, _ = amortize(principal, rate, periods, system, {})
        result["without_early_repayments"] = {"interest": money(base_interest), "periods": len(base_rows),
                                              "interest_saved": money(base_interest - interest)}
    if show:
        result["schedule"] = rows
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except ArithmeticError:
        fail("the numbers are out of the range this tool can compute")
