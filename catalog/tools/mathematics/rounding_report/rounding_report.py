#!/usr/bin/env python3
"""Rounding and uncertainty reporting in Decimal: round a value to significant figures or
decimal places in any of seven modes (half up, half even, half down, up, down, ceiling, floor),
with scientific and engineering notation; and propagate independent standard uncertainties
through sums, products, quotients and powers to first order, reporting the result rounded to
the uncertainty's significant figures (9.81 +- 0.0234 is reported as 9.81 +- 0.02).

Input (JSON on stdin):
  operation   "round" | "report" | "propagate"
  round:      value, sig_figs or decimals, mode (default half_even)
  report:     value and uncertainty: both rounded for reporting (uncertainty_sig_figs, mode)
  propagate:  kind "sum" | "product" | "quotient" | "power"
              terms [{"value", "uncertainty", "coefficient" (sum) | "power" (product)}]
              constant (product, exact, default 1)
              uncertainty_sig_figs 1 (default) or 2, mode for the reported rounding
Values are numbers or decimal strings; floats are read through their shortest repr.
"""
import json
import sys
from decimal import (ROUND_CEILING, ROUND_DOWN, ROUND_FLOOR, ROUND_HALF_DOWN, ROUND_HALF_EVEN, ROUND_HALF_UP, ROUND_UP,
                     Decimal, InvalidOperation, localcontext)

MODES = {"half_up": ROUND_HALF_UP, "half_even": ROUND_HALF_EVEN, "half_down": ROUND_HALF_DOWN, "up": ROUND_UP,
         "down": ROUND_DOWN, "ceiling": ROUND_CEILING, "floor": ROUND_FLOOR}
PREC = 60


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        out = Decimal(str(value).strip())
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not out.is_finite():
        fail(f"{field} must be finite")
    if abs(out.adjusted()) > 10000:
        fail(f"{field} is out of range")
    return out


def mode_of(request):
    mode = request.get("mode", "half_even")
    if not isinstance(mode, str) or mode not in MODES:
        fail("mode must be one of " + ", ".join(MODES))
    return mode


def plain(d):
    """Positional notation without an exponent."""
    return format(d, "f")


def round_sig(x, sig, mode):
    if x == 0:
        return Decimal(0).quantize(Decimal(1).scaleb(-(sig - 1)))
    with localcontext() as ctx:
        ctx.prec = PREC + 10
        exp = x.adjusted() - (sig - 1)
        r = x.quantize(Decimal(1).scaleb(exp), rounding=MODES[mode])
        if r != 0 and r.adjusted() > x.adjusted():  # 9.96 -> 10.0: one digit too many
            r = x.quantize(Decimal(1).scaleb(exp + 1), rounding=MODES[mode])
        return r


def round_dp(x, places, mode):
    with localcontext() as ctx:
        ctx.prec = PREC + 10
        return x.quantize(Decimal(1).scaleb(-places), rounding=MODES[mode])


def scientific(r, sig):
    """d.ddd e N with the given number of significant digits."""
    if r == 0:
        return "0" + ("." + "0" * (sig - 1) if sig > 1 else "") + "e0"
    exp = r.adjusted()
    mant = r.scaleb(-exp)
    mant = mant.quantize(Decimal(1).scaleb(-(sig - 1)))
    return f"{plain(mant)}e{exp}"


def engineering(r, sig):
    if r == 0:
        return "0e0"
    exp = r.adjusted()
    eng = exp - exp % 3
    mant = r.scaleb(-eng)
    places = max(sig - 1 - (exp - eng), 0)
    mant = mant.quantize(Decimal(1).scaleb(-places))
    return f"{plain(mant)}e{eng}"


def sig_count(r, sig=None):
    return sig if sig is not None else len(r.as_tuple().digits)


def do_round(request):
    x = dec(request.get("value"), "value")
    mode = mode_of(request)
    has_sig, has_dp = "sig_figs" in request, "decimals" in request
    if has_sig == has_dp:
        fail("give sig_figs or decimals (one of them)")
    if has_sig:
        sig = request["sig_figs"]
        if isinstance(sig, bool) or not isinstance(sig, int) or not 1 <= sig <= 50:
            fail("sig_figs must be an integer from 1 to 50")
        fn = lambda m: round_sig(x, sig, m)
    else:
        places = request["decimals"]
        if isinstance(places, bool) or not isinstance(places, int) or not -50 <= places <= 50:
            fail("decimals must be an integer from -50 to 50")
        fn = lambda m: round_dp(x, places, m)
    r = fn(mode)
    shown_sig = sig if has_sig else max(len(r.as_tuple().digits) if r != 0 else 1, 1)
    out = {"operation": "round", "value": str(x), "mode": mode, "result": plain(r),
           "scientific": scientific(r, shown_sig), "engineering": engineering(r, shown_sig),
           "by_mode": {m: plain(fn(m)) for m in MODES}}
    if has_sig:
        out["sig_figs"] = sig
    else:
        out["decimals"] = places
    return out


def propagate(request):
    kind = request.get("kind")
    if kind not in ("sum", "product", "quotient", "power"):
        fail("kind must be sum, product, quotient or power")
    terms = request.get("terms")
    if not isinstance(terms, list) or not terms or len(terms) > 1000:
        fail("terms must be a list of 1 to 1000 {value, uncertainty} objects")
    if kind == "quotient" and len(terms) != 2:
        fail("quotient takes exactly two terms: numerator and denominator")
    if kind == "power" and len(terms) != 1:
        fail("power takes exactly one term with its power")
    parsed = []
    for i, t in enumerate(terms):
        if not isinstance(t, dict) or set(t) - {"value", "uncertainty", "coefficient", "power", "name"}:
            fail(f"terms[{i}] must be an object with value, uncertainty and coefficient or power")
        value = dec(t.get("value"), f"terms[{i}].value")
        u = dec(t.get("uncertainty", 0), f"terms[{i}].uncertainty")
        if u < 0:
            fail(f"terms[{i}].uncertainty must not be negative")
        if kind == "sum":
            if "power" in t:
                fail("a sum takes coefficient, not power")
            weight = dec(t.get("coefficient", 1), f"terms[{i}].coefficient")
        else:
            if "coefficient" in t:
                fail("a product takes power, not coefficient (use constant for a factor)")
            weight = dec(t.get("power", 1), f"terms[{i}].power")
            if kind == "quotient":
                weight = Decimal(1) if i == 0 else Decimal(-1)
            if kind == "power" and "power" not in t:
                fail("power needs the term's power")
            if value == 0:
                fail(f"terms[{i}].value is zero: relative uncertainty is undefined in a product")
            if value < 0 and weight != weight.to_integral_value():
                fail(f"terms[{i}]: a negative value cannot take a fractional power")
        parsed.append((value, u, weight))
    sig_u = request.get("uncertainty_sig_figs", 1)
    if sig_u not in (1, 2) or isinstance(sig_u, bool):
        fail("uncertainty_sig_figs must be 1 or 2")
    mode = mode_of(request)
    with localcontext() as ctx:
        ctx.prec = PREC
        if kind == "sum":
            q = sum((c * v for v, _, c in parsed), Decimal(0))
            var = sum(((c * u) ** 2 for _, u, c in parsed), Decimal(0))
            u_q = var.sqrt()
            formula = "q = sum c_i x_i; u(q) = sqrt(sum (c_i u_i)^2)"
        else:
            constant = dec(request.get("constant", 1), "constant")
            q = constant
            rel2 = Decimal(0)
            for v, u, p in parsed:
                q *= v ** p if p != p.to_integral_value() else v ** int(p)
                rel2 += (p * u / v) ** 2
            u_q = abs(q) * rel2.sqrt()
            formula = "q = c prod x_i^p_i; u(q)/|q| = sqrt(sum (p_i u_i / x_i)^2)"
        relative = (u_q / abs(q)) if q != 0 else None
    with localcontext() as ctx:
        ctx.prec = 15
        q_show, u_show = (+q).normalize(), (+u_q).normalize()
    out = {"operation": "propagate", "kind": kind, "value": plain(q_show), "uncertainty": plain(u_show), "relative_uncertainty": str(relative.quantize(Decimal("1e-12"))) if relative is not None else None,
           "formula": formula, "assumption": "independent inputs, first-order (linear) propagation, standard uncertainties"}
    out.update(report(q, u_q, sig_u, mode))
    return out


def report(q, u_q, sig_u, mode):
    if u_q == 0:
        return {"reported": f"{plain(q)} (no uncertainty)"}
    u_r = round_sig(u_q, sig_u, mode)
    with localcontext() as ctx:
        ctx.prec = PREC + 10
        q_r = q.quantize(Decimal(1).scaleb(u_r.as_tuple().exponent), rounding=MODES[mode])
    return {"reported_value": plain(q_r), "reported_uncertainty": plain(u_r), "reported": f"{plain(q_r)} ± {plain(u_r)}",
            "rounding": f"uncertainty to {sig_u} significant figure{'s' if sig_u == 2 else ''}, value to the same decimal place, mode {mode}"}


def do_report(request):
    q = dec(request.get("value"), "value")
    u = dec(request.get("uncertainty"), "uncertainty")
    if u < 0:
        fail("uncertainty must not be negative")
    sig_u = request.get("uncertainty_sig_figs", 1)
    if sig_u not in (1, 2) or isinstance(sig_u, bool):
        fail("uncertainty_sig_figs must be 1 or 2")
    out = {"operation": "report", "value": str(q), "uncertainty": str(u)}
    out.update(report(q, u, sig_u, mode_of(request)))
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    op = request.get("operation")
    if op == "round":
        out = do_round(request)
    elif op == "propagate":
        out = propagate(request)
    elif op == "report":
        out = do_report(request)
    else:
        fail("operation must be round, report or propagate")
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
