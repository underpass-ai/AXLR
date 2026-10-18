#!/usr/bin/env python3
"""Heart-rate training zones: maximum heart rate estimates and zones by heart-rate reserve
(Karvonen) or by percentage of maximum heart rate.

Input (JSON on stdin):
  age          years (needed unless max_hr is given)
  resting_hr   beats per minute; enables Karvonen zones
  max_hr       measured maximum, bpm; overrides the estimate
  formula      "tanaka" | "fox" | "gellish"   estimate to use, default tanaka
  method       "karvonen" | "percent_max" | "both"   default both when resting_hr is given, else percent_max
  boundaries   ascending percentages, default [50, 60, 70, 80, 90, 100] (five zones)
  zone_names   optional names, one per zone

Estimates: Tanaka, Monahan & Seals 2001: 208 - 0.7 x age; Fox, Naughton & Haskell 1971:
220 - age; Gellish et al. 2007: 207 - 0.7 x age.
Karvonen et al. 1957: target = resting + (max - resting) x intensity.
"""
import json
import re
import sys
from decimal import ROUND_HALF_UP, Decimal
from fractions import Fraction

DISCLAIMER = ("Not medical advice: population formulas that can be off by 10 bpm or more for an individual. "
              "Check with a qualified clinician before exercising, especially with a heart condition or medication.")
FORMULAS = {
    "tanaka": (lambda age: 208 - Fraction(7, 10) * age, "Tanaka, Monahan & Seals 2001: 208 - 0.7 x age"),
    "fox": (lambda age: 220 - age, "Fox, Naughton & Haskell 1971: 220 - age"),
    "gellish": (lambda age: 207 - Fraction(7, 10) * age, "Gellish et al. 2007: 207 - 0.7 x age"),
}
DECIMAL = re.compile(r"\s*[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d{1,3})?\s*")


def fail(message):
    print(json.dumps({"error": message, "disclaimer": DISCLAIMER}))
    sys.exit(2)


def number(value, field, low, high):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number")
    if isinstance(value, str) and not DECIMAL.fullmatch(value):
        fail(f"{field} must be a number or a decimal string")
    try:
        result = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number")
    if abs(result) > 10 ** 12:
        fail(f"{field} is too large")
    if not low <= result <= high:
        fail(f"{field} must be between {low} and {high}")
    return result


def text(value):
    d = Decimal(value.numerator) / Decimal(value.denominator)
    return str(d.quantize(Decimal("0.1"), rounding=ROUND_HALF_UP))


def bpm(value):
    return int((Decimal(value.numerator) / Decimal(value.denominator)).quantize(Decimal(1), rounding=ROUND_HALF_UP))


def zones(boundaries, names, low_of):
    rows = []
    for i in range(len(boundaries) - 1):
        low, high = low_of(boundaries[i]), low_of(boundaries[i + 1])
        rows.append({"zone": i + 1, "name": names[i] if names else None, "low_pct": float(boundaries[i]),
                     "high_pct": float(boundaries[i + 1]), "low_bpm": bpm(low), "high_bpm": bpm(high),
                     "low_exact": text(low), "high_exact": text(high)})
    return rows


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"age", "resting_hr", "max_hr", "formula", "method", "boundaries", "zone_names"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    age = number(request["age"], "age", 5, 110) if request.get("age") is not None else None
    formula = request.get("formula", "tanaka")
    if not isinstance(formula, str) or formula not in FORMULAS:
        fail("formula must be tanaka, fox or gellish")
    estimates = {name: text(f(age)) for name, (f, _) in FORMULAS.items()} if age is not None else {}
    if request.get("max_hr") is not None:
        max_hr = number(request["max_hr"], "max_hr", 60, 250)
        max_source = "measured (given)"
    elif age is not None:
        max_hr = FORMULAS[formula][0](age)
        max_source = FORMULAS[formula][1]
    else:
        fail("give age or max_hr")
    resting = number(request["resting_hr"], "resting_hr", 20, 150) if request.get("resting_hr") is not None else None
    if resting is not None and resting >= max_hr:
        fail("resting_hr must be below the maximum heart rate")
    method = request.get("method", "both" if resting is not None else "percent_max")
    if method not in ("karvonen", "percent_max", "both"):
        fail("method must be karvonen, percent_max or both")
    if method in ("karvonen", "both") and resting is None:
        fail("Karvonen zones need resting_hr")
    raw = request.get("boundaries", [50, 60, 70, 80, 90, 100])
    if not isinstance(raw, list) or len(raw) < 2 or len(raw) > 21:
        fail("boundaries must be a list of 2 to 21 percentages")
    boundaries = [number(b, "boundaries[]", 0, 100) for b in raw]
    if any(b >= c for b, c in zip(boundaries, boundaries[1:])):
        fail("boundaries must be strictly ascending")
    names = request.get("zone_names")
    if names is not None and (not isinstance(names, list) or len(names) != len(boundaries) - 1
                              or not all(isinstance(n, str) for n in names)):
        fail("zone_names must be a list of strings, one per zone (len(boundaries) - 1)")
    result = {"age": float(age) if age is not None else None, "max_hr_estimates": estimates,
              "max_hr": text(max_hr), "max_hr_source": max_source}
    if resting is not None:
        result["resting_hr"] = text(resting)
        result["heart_rate_reserve"] = text(max_hr - resting)
    if method in ("karvonen", "both"):
        result["karvonen_zones"] = zones(boundaries, names, lambda p: resting + (max_hr - resting) * p / 100)
    if method in ("percent_max", "both"):
        result["percent_max_zones"] = zones(boundaries, names, lambda p: max_hr * p / 100)
    result["notes"] = ["bpm values are rounded half up; *_exact keep one decimal."]
    result["source"] = "Karvonen, Kentala & Mustala 1957 (heart-rate reserve); " + "; ".join(f[1] for f in FORMULAS.values())
    result["disclaimer"] = DISCLAIMER
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
