#!/usr/bin/env python3
"""Descriptive statistics with exact sums: count, mean, median, modes, variance and standard
deviation (sample and population), quartiles, IQR, skewness, a Student-t confidence interval
for the mean, and, with paired x and y, Pearson and Spearman correlation and the least-squares
line. Means, variances and the regression coefficients are computed over Fractions; square
roots and t quantiles are floats.

Input (JSON on stdin):
  data        [numbers or decimal strings]          one variable, or
  x, y        two lists of the same length            paired data
  confidence  0.95 by default
  quantile_method  "exclusive" (default, as R type 6 and Minitab) or "inclusive" (type 7, Excel)
"""
import json
import math
import statistics
import sys
from fractions import Fraction


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(x):
    """A float for display, at 12 significant digits."""
    value = float(x)
    return float(f"{value:.12g}") if math.isfinite(value) else None


def read(values, field):
    if not isinstance(values, list) or len(values) < 2:
        fail(f"{field} must be a list of at least two numbers")
    out = []
    for i, v in enumerate(values):
        if isinstance(v, bool) or not isinstance(v, (int, float, str)):
            fail(f"{field}[{i}] is not a number")
        try:
            out.append(Fraction(str(v)))
        except (ValueError, ZeroDivisionError):
            fail(f"{field}[{i}] is not a number")
    return out


def betacf(a, b, x):
    tiny, qab, qap, qam = 1e-300, a + b, a + 1, a - 1
    c, d = 1.0, 1 - qab * x / qap
    d = 1 / (d if abs(d) > tiny else tiny)
    h = d
    for m in range(1, 300):
        m2 = 2 * m
        aa = m * (b - m) * x / ((qam + m2) * (a + m2))
        d = 1 + aa * d
        d = 1 / (d if abs(d) > tiny else tiny)
        c = 1 + aa / c
        c = c if abs(c) > tiny else tiny
        h *= d * c
        aa = -(a + m) * (qab + m) * x / ((a + m2) * (qap + m2))
        d = 1 + aa * d
        d = 1 / (d if abs(d) > tiny else tiny)
        c = 1 + aa / c
        c = c if abs(c) > tiny else tiny
        delta = d * c
        h *= delta
        if abs(delta - 1) < 1e-15:
            break
    return h


def betai(a, b, x):
    if x <= 0:
        return 0.0
    if x >= 1:
        return 1.0
    front = math.exp(math.lgamma(a + b) - math.lgamma(a) - math.lgamma(b) + a * math.log(x) + b * math.log(1 - x))
    if x < (a + 1) / (a + b + 2):
        return front * betacf(a, b, x) / a
    return 1 - front * betacf(b, a, 1 - x) / b


def t_cdf(t, df):
    tail = 0.5 * betai(df / 2, 0.5, df / (df + t * t))
    return 1 - tail if t > 0 else tail


def t_quantile(p, df):
    lo, hi = 0.0, 1e6
    for _ in range(200):
        mid = (lo + hi) / 2
        if t_cdf(mid, df) < p:
            lo = mid
        else:
            hi = mid
    return (lo + hi) / 2


def describe(data, confidence, method):
    n = len(data)
    ordered = sorted(data)
    mean = sum(data) / n
    ss = sum((v - mean) ** 2 for v in data)
    var_s, var_p = ss / (n - 1), ss / n
    sd_s, sd_p = math.sqrt(var_s), math.sqrt(var_p)
    q1, q2, q3 = statistics.quantiles(ordered, n=4, method=method)
    counts = {}
    for v in data:
        counts[v] = counts.get(v, 0) + 1
    top = max(counts.values())
    modes = [num(v) for v in sorted(counts) if counts[v] == top] if top > 1 else []
    m3 = sum((v - mean) ** 3 for v in data) / n
    skew = float(m3) / float(var_p) ** 1.5 if var_p else None
    t = t_quantile(1 - (1 - confidence) / 2, n - 1)
    half = t * sd_s / math.sqrt(n)
    return {
        "count": n, "sum": num(sum(data)), "mean": num(mean), "median": num(statistics.median(ordered)), "modes": modes,
        "min": num(ordered[0]), "max": num(ordered[-1]), "range": num(ordered[-1] - ordered[0]),
        "variance_sample": num(var_s), "variance_population": num(var_p), "sd_sample": num(sd_s), "sd_population": num(sd_p),
        "standard_error": num(sd_s / math.sqrt(n)), "coefficient_of_variation": num(sd_s / float(mean)) if mean else None,
        "quartiles": {"q1": num(q1), "q2": num(q2), "q3": num(q3), "iqr": num(q3 - q1), "method": method},
        "skewness": num(skew) if skew is not None else None,
        "confidence_interval": {"level": confidence, "t": num(t), "df": n - 1, "low": num(float(mean) - half), "high": num(float(mean) + half)},
        "exact": {"mean": str(mean), "variance_sample": str(var_s), "variance_population": str(var_p)},
    }


def ranks(values):
    order = sorted(range(len(values)), key=lambda i: values[i])
    out = [Fraction(0)] * len(values)
    i = 0
    while i < len(order):
        j = i
        while j + 1 < len(order) and values[order[j + 1]] == values[order[i]]:
            j += 1
        for k in range(i, j + 1):
            out[order[k]] = Fraction(i + j + 2, 2)
        i = j + 1
    return out


def pearson(x, y):
    mx, my = sum(x) / len(x), sum(y) / len(y)
    sxy = sum((a - mx) * (b - my) for a, b in zip(x, y))
    sxx, syy = sum((a - mx) ** 2 for a in x), sum((b - my) ** 2 for b in y)
    if not sxx or not syy:
        return None, mx, my, sxy, sxx
    return float(sxy) / math.sqrt(float(sxx) * float(syy)), mx, my, sxy, sxx


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    confidence = request.get("confidence", 0.95)
    if isinstance(confidence, bool) or not isinstance(confidence, (int, float)) or not 0 < confidence < 1:
        fail("confidence must be a number between 0 and 1")
    method = request.get("quantile_method", "exclusive")
    if method not in ("exclusive", "inclusive"):
        fail("quantile_method must be exclusive or inclusive")
    if "data" in request:
        print(json.dumps(describe(read(request["data"], "data"), confidence, method)))
        return
    if "x" not in request or "y" not in request:
        fail("give data, or x and y")
    x, y = read(request["x"], "x"), read(request["y"], "y")
    if len(x) != len(y):
        fail("x and y must have the same length")
    r, mx, my, sxy, sxx = pearson(x, y)
    rho, _, _, _, _ = pearson(ranks(x), ranks(y))
    result = {"x": describe(x, confidence, method), "y": describe(y, confidence, method), "pearson_r": num(r) if r is not None else None,
              "spearman_rho": num(rho) if rho is not None else None}
    if sxx:
        slope = sxy / sxx
        intercept = my - slope * mx
        result["regression"] = {"slope": num(slope), "intercept": num(intercept), "r_squared": num(r * r) if r is not None else None,
                                "exact": {"slope": str(slope), "intercept": str(intercept)}}
        if len(x) > 2 and r is not None and abs(r) < 1:
            t = r * math.sqrt((len(x) - 2) / (1 - r * r))
            result["regression"]["p_value_slope"] = num(2 * (1 - t_cdf(abs(t), len(x) - 2)))
    print(json.dumps(result))


if __name__ == "__main__":
    main()
