#!/usr/bin/env python3
"""Investment appraisal of a cash-flow series: NPV, IRR (Newton's method with a bisection
fallback, every sign change reported), MIRR, payback and discounted payback, profitability
index. NPV and the paybacks are computed in Decimal; IRR to 1e-12.

Input (JSON on stdin):
  cashflows       [-1000, 300, 400, 500]   period 0 first; numbers or decimal strings
  rate            discount rate per period, as a fraction (0.08) or a percent string ("8%")
  finance_rate    MIRR: rate paid on the negative flows (default: rate)
  reinvest_rate   MIRR: rate earned on the positive flows (default: rate)
"""
import json
import sys
from decimal import Decimal, InvalidOperation, getcontext

getcontext().prec = 40


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    text = str(value).strip()
    scale = Decimal(1)
    if text.endswith("%"):
        text, scale = text[:-1], Decimal("0.01")
    try:
        number = Decimal(text) * scale
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not number.is_finite():
        fail(f"{field} must be finite")
    return number


def npv(rate, flows):
    one = 1 + rate
    return sum(cf / one ** t for t, cf in enumerate(flows))


def irr_candidates(flows):
    f = [float(cf) for cf in flows]

    def value(r):
        return sum(cf / (1 + r) ** t for t, cf in enumerate(f))

    def slope(r):
        return sum(-t * cf / (1 + r) ** (t + 1) for t, cf in enumerate(f))

    scale = max(abs(x) for x in f)
    grid = [round(-0.99 + i * 0.01, 2) for i in range(199)] + [round(1 + i * 0.05, 2) for i in range(181)] + [20, 50, 100]
    roots = []
    for lo, hi in zip(grid, grid[1:]):
        try:
            vlo, vhi = value(lo), value(hi)
        except (OverflowError, ZeroDivisionError):
            continue
        if abs(vlo) < 1e-12 * scale:
            if not roots or abs(roots[-1] - lo) > 1e-9:
                roots.append(lo)
            continue
        if vlo * vhi > 0 or abs(vhi) < 1e-12 * scale:
            continue
        r = (lo + hi) / 2
        for _ in range(50):
            d = slope(r)
            nxt = r - value(r) / d if d else None
            if nxt is None or not lo <= nxt <= hi:
                break
            if abs(nxt - r) < 1e-14:
                r = nxt
                break
            r = nxt
        if abs(value(r)) > 1e-9 * max(1.0, scale):
            a, b = lo, hi
            for _ in range(200):
                m = (a + b) / 2
                if value(a) * value(m) <= 0:
                    b = m
                else:
                    a = m
            r = (a + b) / 2
        if not roots or abs(roots[-1] - r) > 1e-9:
            roots.append(r)
    return roots


def paybacks(flows, rate):
    out = {}
    for name, discounted in (("payback_periods", False), ("discounted_payback_periods", True)):
        cumulative, result = Decimal(0), None
        for t, cf in enumerate(flows):
            value = cf / (1 + rate) ** t if discounted else cf
            previous = cumulative
            cumulative += value
            if t > 0 and previous < 0 <= cumulative and value > 0:
                result = Decimal(t - 1) + (-previous / value)
                break
        out[name] = float(round(result, 6)) if result is not None else None
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    raw = request.get("cashflows")
    if not isinstance(raw, list) or len(raw) < 2:
        fail("cashflows must be a list of at least two amounts, period 0 first")
    flows = [dec(v, f"cashflows[{i}]") for i, v in enumerate(raw)]
    if not any(cf < 0 for cf in flows) or not any(cf > 0 for cf in flows):
        fail("cashflows need at least one negative and one positive amount")
    result = {"periods": len(flows) - 1}
    roots = irr_candidates(flows)
    result["irr"] = round(roots[0], 12) if len(roots) == 1 else None
    result["irr_candidates"] = [round(r, 12) for r in roots]
    changes = sum(1 for a, b in zip([c for c in flows if c], [c for c in flows if c][1:]) if (a < 0) != (b < 0))
    result["sign_changes"] = changes
    if len(roots) > 1:
        result["note"] = "more than one IRR: the cash flows change sign more than once; use NPV or MIRR"
    if "rate" in request:
        rate = dec(request["rate"], "rate")
        if rate <= -1:
            fail("rate must be greater than -100%")
        value = npv(rate, flows)
        result["rate"] = str(rate)
        result["npv"] = str(round(value, 6))
        invested = -sum(cf / (1 + rate) ** t for t, cf in enumerate(flows) if cf < 0)
        result["profitability_index"] = float(round((value + invested) / invested, 6)) if invested else None
        result.update(paybacks(flows, rate))
        finance = dec(request.get("finance_rate", request["rate"]), "finance_rate")
        reinvest = dec(request.get("reinvest_rate", request["rate"]), "reinvest_rate")
        n = len(flows) - 1
        future = sum(cf * (1 + reinvest) ** (n - t) for t, cf in enumerate(flows) if cf > 0)
        present = -sum(cf / (1 + finance) ** t for t, cf in enumerate(flows) if cf < 0)
        result["mirr"] = round(float((future / present) ** (Decimal(1) / n)) - 1, 12)
    else:
        result.update(paybacks(flows, Decimal(0)))
        result.pop("discounted_payback_periods")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
