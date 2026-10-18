#!/usr/bin/env python3
"""Margin of error of a poll proportion, required sample size, and whether a lead between two
candidates in the same poll is larger than sampling error.

Input (JSON on stdin), every field optional except what the wanted result needs:
  confidence     0.95, 95 or "95%" (default 0.95)
  n              sample size (margin of error, lead)
  p              the proportion, 0.52 or "52%" (default 0.5, the widest margin)
  population     population size N for the finite-population correction
  design_effect  variance inflation from weighting or clustering (default 1)
  margin         target margin of error, 0.03 or "3%" (required sample size)
  p1, p2         two candidates' shares in the same poll (lead test; needs n)
"""
import json
import math
import sys


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def inverse_normal(q):
    """Standard normal quantile: Acklam's rational approximation and one Halley step."""
    a = [-3.969683028665376e+01, 2.209460984245205e+02, -2.759285104469687e+02,
         1.383577518672690e+02, -3.066479806614716e+01, 2.506628277459239e+00]
    b = [-5.447609879822406e+01, 1.615858368580409e+02, -1.556989798598866e+02,
         6.680131188771972e+01, -1.328068155288572e+01]
    c = [-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e+00,
         -2.549671010114536e+00, 4.374664141464968e+00, 2.938163982698783e+00]
    d = [7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e+00,
         3.754408661907416e+00]
    low = 0.02425
    if q < low:
        t = math.sqrt(-2 * math.log(q))
        x = (((((c[0] * t + c[1]) * t + c[2]) * t + c[3]) * t + c[4]) * t + c[5]) / \
            ((((d[0] * t + d[1]) * t + d[2]) * t + d[3]) * t + 1)
    elif q <= 1 - low:
        s = q - 0.5
        r = s * s
        x = (((((a[0] * r + a[1]) * r + a[2]) * r + a[3]) * r + a[4]) * r + a[5]) * s / \
            (((((b[0] * r + b[1]) * r + b[2]) * r + b[3]) * r + b[4]) * r + 1)
    else:
        t = math.sqrt(-2 * math.log(1 - q))
        x = -(((((c[0] * t + c[1]) * t + c[2]) * t + c[3]) * t + c[4]) * t + c[5]) / \
            ((((d[0] * t + d[1]) * t + d[2]) * t + d[3]) * t + 1)
    e = 0.5 * math.erfc(-x / math.sqrt(2)) - q
    u = e * math.sqrt(2 * math.pi) * math.exp(x * x / 2)
    return x - u / (1 + x * u / 2)


def fraction_of(value, field, percent_if_over_one=False):
    """0.52, "0.52" or "52%" -> 0.52; with percent_if_over_one, 95 -> 0.95."""
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a percent string")
    text = str(value).strip()
    try:
        if text.endswith("%"):
            number = float(text[:-1]) / 100
        else:
            number = float(text)
            if percent_if_over_one and number > 1:
                number /= 100
    except ValueError:
        fail(f"{field} must be a number or a percent string")
    if not math.isfinite(number):
        fail(f"{field} must be finite")
    return number


def positive_int(request, field):
    value = request.get(field)
    if value is None:
        return None
    if not isinstance(value, int) or isinstance(value, bool) or value <= 0:
        fail(f"{field} must be a positive integer")
    return value


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"confidence", "n", "p", "population", "design_effect", "margin", "p1", "p2"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    confidence = fraction_of(request.get("confidence", 0.95), "confidence", percent_if_over_one=True)
    if not 0 < confidence < 1:
        fail("confidence must be between 0 and 100 percent, exclusive")
    n = positive_int(request, "n")
    population = positive_int(request, "population")
    deff = fraction_of(request.get("design_effect", 1), "design_effect")
    if deff <= 0:
        fail("design_effect must be positive")
    p = fraction_of(request.get("p", 0.5), "p")
    if not 0 <= p <= 1:
        fail("p must be between 0 and 1")
    if n is not None and population is not None and n > population:
        fail("n cannot exceed population")
    z = inverse_normal(1 - (1 - confidence) / 2)
    fpc = 1.0
    if population is not None and n is not None:
        fpc = math.sqrt((population - n) / (population - 1)) if population > 1 else 0.0
    result = {"confidence": confidence, "z": round(z, 6)}
    did = False
    if n is not None and ("p1" not in request or "p" in request):
        se = math.sqrt(p * (1 - p) * deff / n) * fpc
        moe = z * se
        result.update({
            "n": n, "p": p, "standard_error": round(se, 6), "margin_of_error": round(moe, 6),
            "margin_of_error_points": round(moe * 100, 2),
            "interval": [round(max(0.0, p - moe), 6), round(min(1.0, p + moe), 6)],
            "finite_population_correction": round(fpc, 6) if population is not None else None,
        })
        did = True
    if "margin" in request:
        margin = fraction_of(request["margin"], "margin")
        if not 0 < margin < 1:
            fail("margin must be between 0 and 1 (or 0% and 100%)")
        n0 = z * z * p * (1 - p) * deff / (margin * margin)
        needed = n0 if population is None else n0 / (1 + (n0 - 1) / population)
        result["required_sample_size"] = {"margin": margin, "p": p, "infinite_population": math.ceil(n0 - 1e-9),
                                          "n": max(1, math.ceil(needed - 1e-9)), "population": population}
        did = True
    if "p1" in request or "p2" in request:
        if n is None:
            fail("the lead test needs n")
        p1 = fraction_of(request.get("p1"), "p1")
        p2 = fraction_of(request.get("p2"), "p2")
        if not (0 <= p1 <= 1 and 0 <= p2 <= 1 and p1 + p2 <= 1):
            fail("p1 and p2 must be shares between 0 and 1 that add up to at most 1")
        lead = p1 - p2
        variance = (p1 + p2 - lead * lead) * deff / n
        se = math.sqrt(variance) * fpc
        moe = z * se
        stat = abs(lead) / se if se else math.inf
        p_value = math.erfc(stat / math.sqrt(2)) if se else None
        result["lead"] = {
            "n": n, "p1": p1, "p2": p2, "lead": round(lead, 6), "lead_points": round(lead * 100, 2),
            "standard_error": round(se, 6), "margin_of_error": round(moe, 6),
            "margin_of_error_points": round(moe * 100, 2), "z_statistic": round(stat, 4) if se else None,
            "p_value": round(p_value, 6) if p_value is not None else None, "significant": abs(lead) > moe,
            "candidate_margins": {"p1": round(z * math.sqrt(p1 * (1 - p1) * deff / n) * fpc, 6),
                                  "p2": round(z * math.sqrt(p2 * (1 - p2) * deff / n) * fpc, 6)},
        }
        did = True
    if not did:
        fail("give n (margin of error), margin (sample size) or n with p1 and p2 (lead)")
    result["method"] = ("normal approximation to the binomial (multinomial for the lead), simple random sampling"
                        + (", design effect applied" if deff != 1 else ""))
    print(json.dumps(result))


if __name__ == "__main__":
    main()
