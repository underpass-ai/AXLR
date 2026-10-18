#!/usr/bin/env python3
"""Class grade statistics, grade curves and classical item analysis.

Input (JSON on stdin):
  operation  "stats" | "curve" | "item_analysis"
  scores     [numbers]                         stats, curve
  max_score  default 100                       stats (histogram), curve
  bands      [{"min": 90, "label": "A"}, ...]  stats: distribution by band (else 10-point bins)
  method     "linear" | "sqrt" | "top_score"   curve
  target_mean, target_sd                       curve linear (target_sd optional: shift only)
  clip       default true: curved scores kept within 0..max_score
  matrix     [[0/1 per item] per student]      item_analysis
  group_fraction  default 0.27 (upper-lower groups)
  variance   "population" | "sample"           SD in KR-20 / KR-21, default population
  decimals   default 2 (4 for item statistics)

Means, medians and variances are exact fractions; square roots in 50-digit decimals.
"""
import json
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, getcontext
from fractions import Fraction

getcontext().prec = 50
DECIMAL = re.compile(r"\s*[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d{1,3})?\s*")


def fail(message):
    print(json.dumps({"error": message}))
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
    return result


def dec(value):
    return Decimal(value.numerator) / Decimal(value.denominator) if isinstance(value, Fraction) else value


def text(value, places):
    if value is None:
        return None
    return str(dec(value).quantize(Decimal(1).scaleb(-places), rounding=ROUND_HALF_UP))


def sqrt(value):
    return dec(value).sqrt()


def mean(xs):
    return sum(xs, Fraction(0)) / len(xs)


def variance(xs, sample=False):
    m = mean(xs)
    ss = sum(((x - m) ** 2 for x in xs), Fraction(0))
    if sample:
        return ss / (len(xs) - 1) if len(xs) > 1 else None
    return ss / len(xs)


def median(xs):
    s = sorted(xs)
    n = len(s)
    return s[n // 2] if n % 2 else (s[n // 2 - 1] + s[n // 2]) / 2


def exact(value):
    return str(value) if value is not None else None


def summary(xs, places):
    vp, vs = variance(xs), variance(xs, sample=True)
    return {"n": len(xs), "mean": text(mean(xs), places), "mean_exact": exact(mean(xs)),
            "median": text(median(xs), places), "min": text(min(xs), places), "max": text(max(xs), places),
            "sd_population": text(sqrt(vp), places + 2), "sd_sample": text(sqrt(vs), places + 2) if vs is not None else None,
            "variance_population_exact": exact(vp), "variance_sample_exact": exact(vs)}


def read_scores(request):
    scores = request.get("scores")
    if not isinstance(scores, list) or not scores or len(scores) > 100000:
        fail("scores must be a non-empty list of numbers")
    return [number(s, f"scores[{i}]") for i, s in enumerate(scores)]


def stats(request, places):
    xs = read_scores(request)
    out = summary(xs, places)
    top = number(request.get("max_score", 100), "max_score")
    bands = request.get("bands")
    if bands is not None:
        if not isinstance(bands, list) or not bands:
            fail("bands must be a list of {min, label}")
        rows = []
        for i, b in enumerate(bands):
            if not isinstance(b, dict) or set(b) != {"min", "label"} or not isinstance(b["label"], str):
                fail(f"bands[{i}] must be {{min, label}}")
            rows.append((number(b["min"], f"bands[{i}].min"), b["label"]))
        rows.sort(key=lambda r: r[0], reverse=True)
        counts = {label: 0 for _, label in rows}
        below = 0
        for x in xs:
            band = next((label for low, label in rows if x >= low), None)
            if band is None:
                below += 1
            else:
                counts[band] += 1
        out["distribution"] = [{"label": label, "min": text(low, places), "count": counts[label],
                                "percent": text(Fraction(counts[label] * 100, len(xs)), 1)} for low, label in rows]
        if below:
            out["below_lowest_band"] = below
    else:
        if top <= 0:
            fail("max_score must be positive")
        width = top / 10
        bins = []
        for k in range(10):
            low, high = width * k, width * (k + 1)
            count = sum(1 for x in xs if low <= x < high or (k == 9 and x == high))
            bins.append({"from": text(low, places), "to": text(high, places), "count": count})
        out["histogram"] = bins
        out["outside_0_to_max"] = sum(1 for x in xs if x < 0 or x > top)
    return out


def curve(request, places):
    xs = read_scores(request)
    top = number(request.get("max_score", 100), "max_score")
    if top <= 0:
        fail("max_score must be positive")
    clip = request.get("clip", True)
    if not isinstance(clip, bool):
        fail("clip must be true or false")
    method = request.get("method")
    if method == "linear":
        if request.get("target_mean") is None:
            fail("linear curve needs target_mean (and optionally target_sd)")
        target_mean = dec(number(request["target_mean"], "target_mean"))
        m, sd = dec(mean(xs)), sqrt(variance(xs))
        if request.get("target_sd") is not None:
            target_sd = dec(number(request["target_sd"], "target_sd"))
            if target_sd < 0:
                fail("target_sd must not be negative")
            if sd == 0:
                fail("all scores are equal: the SD cannot be rescaled")
            ratio = target_sd / sd
        else:
            ratio = Decimal(1)
        new = [target_mean + (dec(x) - m) * ratio for x in xs]
        rule = "new = target_mean + (x - mean) x target_sd / sd (population SD)"
    elif method == "sqrt":
        if any(x < 0 for x in xs):
            fail("the square-root curve needs scores of zero or more")
        new = [sqrt(x / top) * dec(top) for x in xs]
        rule = "new = sqrt(x / max_score) x max_score (10 x sqrt(x) on 100)"
    elif method == "top_score":
        best = max(xs)
        if best <= 0:
            fail("the top score must be positive")
        new = [dec(x * top / best) for x in xs]
        rule = "new = x x max_score / top score"
    else:
        fail("method must be linear, sqrt or top_score")
    clipped = 0
    if clip:
        bounded = [min(max(v, Decimal(0)), dec(top)) for v in new]
        clipped = sum(1 for a, b in zip(new, bounded) if a != b)
        new = bounded
    rounded = [Fraction(text(v, places)) for v in new]
    return {"method": method, "rule": rule, "curved": [text(v, places) for v in new],
            "clipped": clipped, "before": summary(xs, places), "after": summary(rounded, places)}


def pearson(xs, ys):
    mx, my = mean(xs), mean(ys)
    sxy = sum(((x - mx) * (y - my) for x, y in zip(xs, ys)), Fraction(0))
    sxx = sum(((x - mx) ** 2 for x in xs), Fraction(0))
    syy = sum(((y - my) ** 2 for y in ys), Fraction(0))
    if sxx == 0 or syy == 0:
        return None
    r = sqrt(sxy * sxy / (sxx * syy))
    return r if sxy >= 0 else -r


def item_analysis(request, places):
    matrix = request.get("matrix")
    if not isinstance(matrix, list) or len(matrix) < 2:
        fail("matrix must be a list of at least 2 students, each a list of 0/1 item scores")
    k = len(matrix[0]) if isinstance(matrix[0], list) else 0
    if k < 2:
        fail("matrix rows must have at least 2 items")
    for i, row in enumerate(matrix):
        if not isinstance(row, list) or len(row) != k or any(v not in (0, 1) or isinstance(v, bool) for v in row):
            fail(f"matrix[{i}] must be a list of {k} values, each 0 or 1")
    sample = request.get("variance", "population")
    if sample not in ("population", "sample"):
        fail("variance must be population or sample")
    fraction = number(request.get("group_fraction", "0.27"), "group_fraction")
    if not 0 < fraction <= Fraction(1, 2):
        fail("group_fraction must be above 0 and at most 0.5")
    n = len(matrix)
    totals = [Fraction(sum(row)) for row in matrix]
    var_t = variance(totals, sample == "sample")
    g = int(dec(fraction * n).quantize(Decimal(1), rounding=ROUND_HALF_UP))
    g = min(max(g, 1), n // 2)
    order = sorted(range(n), key=lambda i: (-totals[i], i))
    upper, lower = order[:g], order[-g:]
    items, sum_pq = [], Fraction(0)
    p4 = 4
    for j in range(k):
        col = [Fraction(row[j]) for row in matrix]
        p = mean(col)
        sum_pq += p * (1 - p)
        rest = [t - c for t, c in zip(totals, col)]
        pu = Fraction(sum(matrix[i][j] for i in upper), g)
        pl = Fraction(sum(matrix[i][j] for i in lower), g)
        r_pb, r_rest = pearson(col, totals), pearson(col, rest)
        flags = []
        if p < Fraction(1, 5):
            flags.append("very hard (p < 0.20)")
        if p > Fraction(9, 10):
            flags.append("very easy (p > 0.90)")
        if pu - pl < Fraction(1, 5):
            flags.append("low discrimination (D < 0.20)")
        if r_rest is not None and r_rest < 0:
            flags.append("negative item-rest correlation")
        items.append({"item": j + 1, "difficulty_p": text(p, p4), "p_exact": exact(p),
                      "point_biserial": text(r_pb, p4), "point_biserial_corrected": text(r_rest, p4),
                      "upper_p": text(pu, p4), "lower_p": text(pl, p4), "discrimination_d": text(pu - pl, p4),
                      "d_exact": exact(pu - pl), "flags": flags})
    out = {"students": n, "items": k, "totals": summary(totals, places), "item_statistics": items,
           "groups": {"fraction": exact(fraction), "size": g, "upper": [i + 1 for i in upper],
                      "lower": [i + 1 for i in lower]},
           "variance_used": sample, "sum_pq_exact": exact(sum_pq)}
    if var_t:
        kr20 = Fraction(k, k - 1) * (1 - sum_pq / var_t)
        m = mean(totals)
        kr21 = Fraction(k, k - 1) * (1 - m * (k - m) / (k * var_t))
        out.update({"kr20": text(kr20, p4), "kr20_exact": exact(kr20), "kr21": text(kr21, p4), "kr21_exact": exact(kr21),
                    "total_variance_exact": exact(var_t)})
    else:
        out.update({"kr20": None, "kr21": None, "note": "total scores have no variance: reliability undefined"})
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"operation", "scores", "max_score", "bands", "method", "target_mean", "target_sd",
                                   "clip", "matrix", "group_fraction", "variance", "decimals"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    places = request.get("decimals", 2)
    if isinstance(places, bool) or not isinstance(places, int) or not 0 <= places <= 10:
        fail("decimals must be an integer from 0 to 10")
    operation = request.get("operation")
    handlers = {"stats": stats, "curve": curve, "item_analysis": item_analysis}
    if not isinstance(operation, str) or operation not in handlers:
        fail("operation must be stats, curve or item_analysis")
    result = {"operation": operation}
    result.update(handlers[operation](request, places))
    print(json.dumps(result))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
