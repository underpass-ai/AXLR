#!/usr/bin/env python3
"""Convert common laboratory results between mass units (mg/dL, mg/L, g/L) and molar units
(mmol/L, umol/L) through the analyte's molar mass, exactly.

Input (JSON on stdin):
  analyte        one of ANALYTES (or list_analytes: true)
  value          number or decimal string
  from           unit of value: mg/dL | mg/L | g/L | mmol/L | umol/L (µmol/L)
  to             target unit; optional, default the analyte's usual unit on the other side
  decimals       optional, default 2 (digits after the point in "result")

Molar masses are computed here from the formula with the IUPAC/CIAAW conventional atomic
weights (CIAAW, Standard Atomic Weights, https://www.ciaaw.org/atomic-weights.htm, abridged
values as in the IUPAC Periodic Table of the Elements, 2022): H 1.008, C 12.011, N 14.007,
O 15.999, Ca 40.078.
"""
import json
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, localcontext
from fractions import Fraction

DISCLAIMER = ("Not medical advice: a unit conversion only. Reference ranges and units differ between "
              "laboratories; verify with a qualified clinician before any clinical use.")
ATOMIC = {"H": Fraction("1.008"), "C": Fraction("12.011"), "N": Fraction("14.007"), "O": Fraction("15.999"),
          "Ca": Fraction("40.078")}
ATOMIC_SOURCE = "IUPAC/CIAAW conventional atomic weights (abridged, IUPAC Periodic Table 2022)"
# name: (formula used for the molar mass, mass unit usual, molar unit usual, what is counted, note)
ANALYTES = {
    "glucose": ("C6H12O6", "mg/dL", "mmol/L", "glucose", None),
    "cholesterol_total": ("C27H46O", "mg/dL", "mmol/L", "cholesterol", None),
    "cholesterol_hdl": ("C27H46O", "mg/dL", "mmol/L", "cholesterol in HDL", None),
    "cholesterol_ldl": ("C27H46O", "mg/dL", "mmol/L", "cholesterol in LDL", None),
    "triglycerides": ("C57H104O6", "mg/dL", "mmol/L", "triglyceride as triolein",
                      "Triglycerides are a mixture; laboratories convert as triolein (885.45 g/mol), the convention "
                      "behind the usual factor 0.01129."),
    "creatinine": ("C4H7N3O", "mg/dL", "umol/L", "creatinine", None),
    "urea": ("CH4N2O", "mg/dL", "mmol/L", "urea", None),
    "bun": ("N2", "mg/dL", "mmol/L", "urea (from blood urea nitrogen)",
            "BUN is the mass of nitrogen in urea: one mole of urea carries two N atoms (28.014 g), so BUN in mg/dL "
            "converts to mmol/L of urea. Urea in mg/dL is a different quantity (analyte 'urea', 60.06 g/mol)."),
    "uric_acid": ("C5H4N4O3", "mg/dL", "umol/L", "uric acid", None),
    "calcium": ("Ca", "mg/dL", "mmol/L", "calcium (total)", None),
}
# unit: (kind, factor to g/L for mass, or to mol/L for molar)
UNITS = {"mg/dL": ("mass", Fraction(1, 100)), "mg/L": ("mass", Fraction(1, 1000)), "g/L": ("mass", Fraction(1)),
         "mmol/L": ("molar", Fraction(1, 1000)), "umol/L": ("molar", Fraction(1, 1000000))}


def fail(message):
    print(json.dumps({"error": message, "disclaimer": DISCLAIMER}))
    sys.exit(2)


def molar_mass(formula):
    total = Fraction(0)
    for symbol, count in re.findall(r"([A-Z][a-z]?)(\d*)", formula):
        total += ATOMIC[symbol] * (int(count) if count else 1)
    return total


def decimal_text(value, places):
    with localcontext() as ctx:
        ctx.prec = 60
        exact = Decimal(value.numerator) / Decimal(value.denominator)
        return str(exact.quantize(Decimal(1).scaleb(-places), rounding=ROUND_HALF_UP))


def unit_name(text, field):
    if not isinstance(text, str):
        fail(f"{field} must be one of {', '.join(UNITS)}")
    text = text.strip().replace("µ", "u").replace("μ", "u")
    if text not in UNITS:
        fail(f"{field} must be one of {', '.join(UNITS)}")
    return text


def factor(analyte, source, target):
    """result = value * factor, exactly."""
    mass = molar_mass(ANALYTES[analyte][0])
    kind_s, to_base_s = UNITS[source]
    kind_t, to_base_t = UNITS[target]
    base = to_base_s
    if kind_s == "mass" and kind_t == "molar":
        base = base / mass
    elif kind_s == "molar" and kind_t == "mass":
        base = base * mass
    return base / to_base_t, mass


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"analyte", "value", "from", "to", "decimals", "list_analytes"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    if request.get("list_analytes"):
        rows = []
        for name, (formula, mass_unit, molar_unit, counted, note) in ANALYTES.items():
            f, mass = factor(name, mass_unit, molar_unit)
            rows.append({"analyte": name, "formula": formula, "molar_mass_g_mol": decimal_text(mass, 3),
                         "counted_as": counted, "usual": f"{mass_unit} -> {molar_unit}",
                         "factor": decimal_text(f, 6), "note": note})
        print(json.dumps({"analytes": rows, "atomic_weights": {k: str(v) for k, v in ATOMIC.items()},
                          "source": ATOMIC_SOURCE, "disclaimer": DISCLAIMER}, ensure_ascii=False))
        return
    analyte = request.get("analyte")
    if not isinstance(analyte, str) or analyte not in ANALYTES:
        fail(f"analyte must be one of {', '.join(ANALYTES)}")
    value = request.get("value")
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail("value must be a number or a decimal string")
    if isinstance(value, str) and not re.fullmatch(r"\s*[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d{1,3})?\s*", value):
        fail("value must be a number or a decimal string")
    try:
        amount = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail("value must be a number or a decimal string")
    if amount > 10 ** 12:
        fail("value is too large")
    if amount < 0:
        fail("value must not be negative")
    source = unit_name(request.get("from"), "from")
    formula, mass_unit, molar_unit, counted, note = ANALYTES[analyte]
    if "to" in request:
        target = unit_name(request.get("to"), "to")
    else:
        target = molar_unit if UNITS[source][0] == "mass" else mass_unit
    decimals = request.get("decimals", 2)
    if isinstance(decimals, bool) or not isinstance(decimals, int) or not 0 <= decimals <= 12:
        fail("decimals must be an integer from 0 to 12")
    f, mass = factor(analyte, source, target)
    result = amount * f
    out = {
        "analyte": analyte, "counted_as": counted, "value": str(value), "from": source, "to": target,
        "result": decimal_text(result, decimals), "result_exact": str(result),
        "factor": decimal_text(f, 8), "formula": formula, "molar_mass_g_mol": decimal_text(mass, 3),
        "notes": [note] if note else [], "source": ATOMIC_SOURCE, "disclaimer": DISCLAIMER,
    }
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
