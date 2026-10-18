#!/usr/bin/env python3
"""Weight- or surface-based dose arithmetic with a hard maximum taken from the input.

The tool never suggests a dose: the dose per kg (or per m2), the maximum single dose and every
other number come from the caller (a prescription, a formulary); the tool multiplies, divides,
rounds and caps, exactly, and says what it did.

Input (JSON on stdin):
  dose               amount per basis unit (number or decimal string), required
  basis              "kg" | "m2"                      default kg
  weight_kg          required with basis kg
  bsa_m2             required with basis m2
  dose_unit          label, default "mg" (mg, mcg, g, units, mmol, ...)
  dose_period        "per_dose" | "per_day"           default per_dose; per_day is divided by frequency
  frequency_per_day  integer 1-24, default 1
  max_dose           hard maximum single dose, in dose_unit (REQUIRED)
  max_daily_dose     optional hard maximum per day, in dose_unit
  rounding_increment optional, in dose_unit (e.g. 0.5); rounding "nearest" (default) or "down"
  concentration      optional: number (dose_unit per mL) or {"amount": a, "volume_ml": v}
  volume_increment   optional, mL (e.g. 0.1), nearest
"""
import json
import re
import sys
from decimal import ROUND_FLOOR, ROUND_HALF_UP, Decimal, localcontext
from fractions import Fraction

DISCLAIMER = ("Not medical advice and not a dosing recommendation: arithmetic on the numbers given only. "
              "Every dose must be checked against the prescription and a current formulary by a qualified clinician "
              "or pharmacist.")
FIELDS = {"dose", "basis", "weight_kg", "bsa_m2", "dose_unit", "dose_period", "frequency_per_day", "max_dose",
          "max_daily_dose", "rounding_increment", "rounding", "concentration", "volume_increment"}
DECIMAL = re.compile(r"\s*[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d{1,3})?\s*")


def fail(message):
    print(json.dumps({"error": message, "disclaimer": DISCLAIMER}))
    sys.exit(2)


def number(value, field, high=None):
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
    if high is not None and result > high:
        fail(f"{field} must be at most {high}")
    return result


def text(value):
    with localcontext() as ctx:
        ctx.prec = 50
        d = (Decimal(value.numerator) / Decimal(value.denominator)).quantize(Decimal("0.000001"), rounding=ROUND_HALF_UP)
    s = format(d.normalize(), "f")
    return s


def to_increment(value, increment, mode):
    steps = value / increment
    with localcontext() as ctx:
        ctx.prec = 50
        d = Decimal(steps.numerator) / Decimal(steps.denominator)
        n = int(d.quantize(Decimal(1), rounding=ROUND_FLOOR if mode == "down" else ROUND_HALF_UP))
    return n * increment


def floor_to(value, increment):
    return to_increment(value, increment, "down") if increment else value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - FIELDS)
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    if request.get("max_dose") is None:
        fail("max_dose is required: the hard maximum single dose must come from the prescription or formulary")
    if request.get("dose") is None:
        fail("dose (amount per kg or per m2) is required")
    dose = number(request["dose"], "dose")
    max_dose = number(request["max_dose"], "max_dose")
    unit = request.get("dose_unit", "mg")
    if not isinstance(unit, str) or not 0 < len(unit) <= 20:
        fail("dose_unit must be a short label such as mg")
    basis = request.get("basis", "kg")
    if basis == "kg":
        if request.get("weight_kg") is None:
            fail("weight_kg is required with basis kg")
        amount_of_basis = number(request["weight_kg"], "weight_kg", 500)
    elif basis == "m2":
        if request.get("bsa_m2") is None:
            fail("bsa_m2 is required with basis m2")
        amount_of_basis = number(request["bsa_m2"], "bsa_m2", 5)
    else:
        fail("basis must be kg or m2")
    period = request.get("dose_period", "per_dose")
    if period not in ("per_dose", "per_day"):
        fail("dose_period must be per_dose or per_day")
    frequency = request.get("frequency_per_day", 1)
    if isinstance(frequency, bool) or not isinstance(frequency, int) or not 1 <= frequency <= 24:
        fail("frequency_per_day must be an integer from 1 to 24")
    max_daily = number(request["max_daily_dose"], "max_daily_dose") if request.get("max_daily_dose") is not None else None
    increment = number(request["rounding_increment"], "rounding_increment") if request.get("rounding_increment") is not None else None
    mode = request.get("rounding", "nearest")
    if mode not in ("nearest", "down"):
        fail("rounding must be nearest or down")
    concentration = None
    if request.get("concentration") is not None:
        c = request["concentration"]
        if isinstance(c, dict):
            if set(c) - {"amount", "volume_ml"} or "amount" not in c or "volume_ml" not in c:
                fail("concentration object needs exactly amount and volume_ml")
            concentration = number(c["amount"], "concentration.amount") / number(c["volume_ml"], "concentration.volume_ml")
        else:
            concentration = number(c, "concentration")
    volume_increment = number(request["volume_increment"], "volume_increment") if request.get("volume_increment") is not None else None
    if volume_increment is not None and concentration is None:
        fail("volume_increment needs concentration")

    flags = []
    total = dose * amount_of_basis
    calculated = total if period == "per_dose" else total / frequency
    working = calculated
    if increment:
        working = to_increment(calculated, increment, mode)
        if working != calculated:
            flags.append(f"rounded {mode} to a multiple of {text(increment)} {unit}")
        if working == 0:
            fail("the calculated dose rounds to zero at this increment")
    final = working
    if final > max_dose:
        final = floor_to(max_dose, increment)
        flags.append(f"CAPPED at max_dose {text(max_dose)} {unit}: the calculated dose {text(calculated)} {unit} exceeds it")
    if max_daily is not None and final * frequency > max_daily:
        final = floor_to(max_daily / frequency, increment)
        flags.append(f"CAPPED at max_daily_dose {text(max_daily)} {unit} per day ({frequency} doses)")
    if final <= 0:
        fail("the maximum is smaller than one rounding increment")
    result = {
        "basis": basis, "basis_amount": text(amount_of_basis), "dose_per_basis": text(dose), "dose_unit": unit,
        "dose_period": period, "frequency_per_day": frequency,
        "calculated_dose": text(calculated), "dose_per_administration": text(final),
        "daily_total": text(final * frequency), "capped": any(f.startswith("CAPPED") for f in flags),
        "max_dose": text(max_dose), "max_daily_dose": text(max_daily) if max_daily is not None else None,
        "effective_dose_per_basis": text(final / amount_of_basis),
    }
    if concentration is not None:
        volume = final / concentration
        result["concentration_per_ml"] = text(concentration)
        result["volume_ml"] = text(volume)
        if volume_increment:
            measured = to_increment(volume, volume_increment, "nearest")
            if measured * concentration > final:
                measured = to_increment(volume, volume_increment, "down")
            if measured == 0:
                fail("the volume rounds to zero at this volume_increment")
            result["volume_ml_measured"] = text(measured)
            result["dose_in_measured_volume"] = text(measured * concentration)
            if measured != volume:
                flags.append(f"volume rounded to a multiple of {text(volume_increment)} mL without exceeding the dose")
    result["flags"] = flags
    result["disclaimer"] = DISCLAIMER
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
