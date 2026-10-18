#!/usr/bin/env python3
"""Estimated GFR by the race-free CKD-EPI 2021 equations (Inker et al., N Engl J Med 2021)
and the KDIGO 2012 G stage.

Input (JSON on stdin):
  age               years, 18 or more (the equations are for adults)
  sex               "female" | "male"
  creatinine        serum creatinine (IDMS-standardised), number or decimal string
  creatinine_unit   "mg/dL" | "umol/L" (also "µmol/L")   optional, default mg/dL; 88.4 umol/L = 1 mg/dL
  cystatin_c        serum cystatin C in mg/L              optional: adds the creatinine-cystatin 2021 equation

eGFRcr     = 142 x min(Scr/k, 1)^a x max(Scr/k, 1)^-1.200 x 0.9938^age x 1.012 [female]
eGFRcr-cys = 135 x min(Scr/k, 1)^b x max(Scr/k, 1)^-0.544 x min(Scys/0.8, 1)^-0.323
                 x max(Scys/0.8, 1)^-0.778 x 0.9961^age x 0.963 [female]
k = 0.7 female, 0.9 male; a = -0.241 female, -0.302 male; b = -0.219 female, -0.144 male.
"""
import json
import math
import sys
from decimal import ROUND_HALF_UP, Decimal

DISCLAIMER = ("Not medical advice: an arithmetic estimate from the published CKD-EPI 2021 equations. "
              "Verify every result with a qualified clinician before any clinical use.")
UMOL_PER_MGDL = 88.4
STAGES = [(90, "G1", "Normal or high"), (60, "G2", "Mildly decreased"), (45, "G3a", "Mildly to moderately decreased"),
          (30, "G3b", "Moderately to severely decreased"), (15, "G4", "Severely decreased"), (0, "G5", "Kidney failure")]


def fail(message):
    print(json.dumps({"error": message, "disclaimer": DISCLAIMER}))
    sys.exit(2)


def number(value, field, low, high):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        result = float(Decimal(str(value).strip()))
    except ArithmeticError:
        fail(f"{field} must be a number or a decimal string")
    if not math.isfinite(result) or not (low < result <= high):
        fail(f"{field} must be greater than {low} and at most {high}")
    return result


def stage(egfr):
    for low, code, label in STAGES:
        if egfr >= low:
            return {"stage": code, "description": label}


def report(value):
    whole = int(Decimal(repr(value)).quantize(Decimal(1), rounding=ROUND_HALF_UP))
    out = {"egfr": whole, "egfr_unrounded": str(Decimal(repr(value)).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)),
           "reported": "<5" if whole < 5 else ">150" if whole > 150 else str(whole), "unit": "mL/min/1.73 m2"}
    out.update(stage(whole))
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"age", "sex", "creatinine", "creatinine_unit", "cystatin_c"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    if request.get("age") is None:
        fail("age is required (do not calculate without it)")
    age = number(request.get("age"), "age", 0, 130)
    if age < 18:
        fail("age under 18: the CKD-EPI 2021 equations are for adults; do not calculate")
    sex = request.get("sex")
    if sex not in ("female", "male"):
        fail("sex must be female or male (the equations have a coefficient for each)")
    unit = request.get("creatinine_unit", "mg/dL")
    if unit not in ("mg/dL", "umol/L", "µmol/L", "μmol/L"):
        fail("creatinine_unit must be mg/dL or umol/L")
    if request.get("creatinine") is None:
        fail("creatinine is required")
    scr = number(request.get("creatinine"), "creatinine", 0, 40 if unit == "mg/dL" else 40 * UMOL_PER_MGDL)
    if unit != "mg/dL":
        scr /= UMOL_PER_MGDL
    female = sex == "female"
    kappa = 0.7 if female else 0.9
    ratio = scr / kappa
    egfr_cr = (142 * min(ratio, 1) ** (-0.241 if female else -0.302) * max(ratio, 1) ** -1.200
               * 0.9938 ** age * (1.012 if female else 1))
    result = {"creatinine_mg_dl": str(Decimal(repr(scr)).quantize(Decimal("0.001"), rounding=ROUND_HALF_UP)),
              "egfr_cr": report(egfr_cr), "equation": "CKD-EPI 2021 creatinine (race-free)"}
    if request.get("cystatin_c") is not None:
        scys = number(request.get("cystatin_c"), "cystatin_c", 0, 20)
        cys = scys / 0.8
        egfr_cr_cys = (135 * min(ratio, 1) ** (-0.219 if female else -0.144) * max(ratio, 1) ** -0.544
                       * min(cys, 1) ** -0.323 * max(cys, 1) ** -0.778 * 0.9961 ** age * (0.963 if female else 1))
        result["egfr_cr_cys"] = report(egfr_cr_cys)
        result["equation"] += " and CKD-EPI 2021 creatinine-cystatin C"
    result["notes"] = ["Reported in whole numbers, 5 to 150 (values outside print as <5 or >150), as the NKF "
                       "implementation guidance recommends; stage from the whole number (KDIGO 2012 G categories).",
                       "A CKD diagnosis needs persistence over 3 months and albuminuria; one eGFR is not a stage."]
    result["source"] = "Inker et al., N Engl J Med 2021;385:1737-49; KDIGO 2012 CKD guideline"
    result["disclaimer"] = DISCLAIMER
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
