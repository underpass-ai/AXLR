#!/usr/bin/env python3
"""Body-mass index with the WHO adult categories and body-surface area by four formulas.

Input (JSON on stdin):
  weight   number or decimal string (kg, or lb with units "imperial")
  height   number or decimal string (cm, or in with units "imperial")
  units    "metric" | "imperial"            optional, default metric
  age      years                            optional; under 18 the WHO adult cut-offs do not apply

BMI = kg / m^2 (exact fraction). BSA in m^2 (H in cm, W in kg):
  Mosteller (1987)      sqrt(H * W / 3600)
  DuBois & DuBois (1916) 0.007184 * W^0.425 * H^0.725
  Haycock (1978)        0.024265 * W^0.5378 * H^0.3964
  Gehan & George (1970) 0.0235 * W^0.51456 * H^0.42246
"""
import json
import math
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, localcontext
from fractions import Fraction

DISCLAIMER = ("Not medical advice: arithmetic from published formulas only. Verify every result with a "
              "qualified clinician before any clinical use.")
LB, INCH = Fraction("0.45359237"), Fraction("2.54")
WHO = [  # (lower bound inclusive, category, sub-category), WHO adult BMI classification
    (Fraction(0), "Underweight", "Severe thinness"),
    (Fraction(16), "Underweight", "Moderate thinness"),
    (Fraction(17), "Underweight", "Mild thinness"),
    (Fraction("18.5"), "Normal range", "Normal range"),
    (Fraction(25), "Pre-obese", "Pre-obese"),
    (Fraction(30), "Obese", "Obese class I"),
    (Fraction(35), "Obese", "Obese class II"),
    (Fraction(40), "Obese", "Obese class III"),
]
DECIMAL = re.compile(r"\s*[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d{1,3})?\s*")


def fail(message):
    print(json.dumps({"error": message, "disclaimer": DISCLAIMER}))
    sys.exit(2)


def number(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    if isinstance(value, str) and not DECIMAL.fullmatch(value):
        fail(f"{field} must be a number or a decimal string")
    try:
        result = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number or a decimal string")
    if abs(result) > 10 ** 12:
        fail(f"{field} is too large")
    if result <= 0:
        fail(f"{field} must be positive")
    return result


def fixed(value, places):
    with localcontext() as ctx:
        ctx.prec = 60
        if isinstance(value, Fraction):
            value = Decimal(value.numerator) / Decimal(value.denominator)
        return str(value.quantize(Decimal(1).scaleb(-places), rounding=ROUND_HALF_UP))


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"weight", "height", "units", "age"}
    extra = sorted(set(request) - allowed)
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    units = request.get("units", "metric")
    if units not in ("metric", "imperial"):
        fail("units must be metric or imperial")
    weight = number(request.get("weight"), "weight")
    height = number(request.get("height"), "height")
    age = request.get("age")
    if age is not None and (isinstance(age, bool) or not isinstance(age, (int, float)) or age < 0 or age > 130):
        fail("age must be a number of years between 0 and 130")
    kg, cm = (weight, height) if units == "metric" else (weight * LB, height * INCH)
    if not (Fraction(20) <= cm <= Fraction(300)):
        fail("height must be between 20 and 300 cm (8 and 118 in)")
    if not (Fraction(1) <= kg <= Fraction(700)):
        fail("weight must be between 1 and 700 kg")
    bmi = kg / (cm / 100) ** 2
    band = [row for row in WHO if bmi >= row[0]][-1]
    with localcontext() as ctx:
        ctx.prec = 60
        mosteller = (Decimal((cm * kg).numerator) / Decimal((cm * kg).denominator) / 3600).sqrt()
    w, h = float(kg), float(cm)
    bsa = {
        "mosteller": fixed(mosteller, 4),
        "dubois": fixed(Decimal(repr(0.007184 * math.pow(w, 0.425) * math.pow(h, 0.725))), 4),
        "haycock": fixed(Decimal(repr(0.024265 * math.pow(w, 0.5378) * math.pow(h, 0.3964))), 4),
        "gehan_george": fixed(Decimal(repr(0.0235 * math.pow(w, 0.51456) * math.pow(h, 0.42246))), 4),
    }
    notes = []
    if age is not None and age < 18:
        notes.append("Under 18: the WHO adult cut-offs do not apply; use BMI-for-age percentiles "
                     "(WHO growth reference or national charts).")
    result = {
        "weight_kg": fixed(kg, 3),
        "height_cm": fixed(cm, 2),
        "bmi": fixed(bmi, 2),
        "bmi_rounded": fixed(bmi, 1),
        "bmi_exact": str(bmi),
        "who_category": band[1] if age is None or age >= 18 else None,
        "who_subcategory": band[2] if age is None or age >= 18 else None,
        "bsa_m2": bsa,
        "notes": notes,
        "source": "WHO adult BMI classification; Mosteller 1987, DuBois & DuBois 1916, Haycock 1978, Gehan & George 1970",
        "disclaimer": DISCLAIMER,
    }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
