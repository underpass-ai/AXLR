#!/usr/bin/env python3
"""Classical hypothesis tests with p-values: Welch, Student (pooled), paired and one-sample t
tests; chi-square goodness of fit and independence; Fisher's exact test on a 2x2 table; one-
and two-proportion z tests. Means, variances and the chi-square statistic are summed over
Fractions; p-values come from the regularized incomplete beta (t) and gamma (chi-square)
functions and the complementary error function (z). Fisher's p-value is an exact fraction.

Input (JSON on stdin):
  test         "welch_t" | "student_t" | "paired_t" | "one_sample_t" | "chi_square_gof" |
               "chi_square_independence" | "fisher_exact" | "one_proportion_z" | "two_proportion_z"
  x, y         samples (numbers or decimal strings); one_sample_t takes x and mu
  mu           hypothesised mean (one_sample_t) or mean difference (t tests), default 0
  observed     counts (chi_square_gof), with expected counts or proportions (default equal)
  ddof         extra degrees of freedom lost to estimated parameters (chi_square_gof)
  table        [[...], ...] counts (chi_square_independence, fisher_exact: 2x2)
  yates        continuity correction on a 2x2 chi-square table, default true (as R and scipy)
  successes, trials   integers (one_proportion_z) or pairs of integers (two_proportion_z)
  p0           hypothesised proportion (one_proportion_z), default 0.5
  alternative  "two-sided" (default) | "less" | "greater"
  confidence   level of the two-sided confidence interval, default 0.95
"""
import json
import math
import sys
from fractions import Fraction
from statistics import NormalDist

MAX_SAMPLE = 100000
MAX_FISHER_N = 10000
TESTS = ("welch_t", "student_t", "paired_t", "one_sample_t", "chi_square_gof", "chi_square_independence",
         "fisher_exact", "one_proportion_z", "two_proportion_z")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(x):
    """A float for display, at 12 significant digits."""
    value = float(x)
    return float(f"{value:.12g}") if math.isfinite(value) else None


def read(values, field, minimum=2):
    if not isinstance(values, list) or len(values) < minimum:
        fail(f"{field} must be a list of at least {minimum} numbers")
    if len(values) > MAX_SAMPLE:
        fail(f"{field} may hold at most {MAX_SAMPLE} values")
    out = []
    for i, v in enumerate(values):
        if isinstance(v, bool) or not isinstance(v, (int, float, str)):
            fail(f"{field}[{i}] is not a number")
        try:
            out.append(Fraction(str(v).strip()))
        except (ValueError, ZeroDivisionError):
            fail(f"{field}[{i}] is not a number")
    return out


def scalar(request, field, default=None):
    value = request.get(field, default)
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number")
    try:
        return Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number")


def counts(values, field, minimum=1):
    if not isinstance(values, list) or len(values) < minimum:
        fail(f"{field} must be a list of at least {minimum} counts")
    for i, v in enumerate(values):
        if isinstance(v, bool) or not isinstance(v, int) or v < 0:
            fail(f"{field}[{i}] must be a non-negative integer")
    return values


# ---------- regularized incomplete beta (Numerical Recipes 6.4), copied from stats_describe ----------

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


# ---------- regularized incomplete gamma (Numerical Recipes 6.2) ----------

def gser(a, x):
    ap, total = a, 1.0 / a
    delta = total
    for _ in range(100000):
        ap += 1
        delta *= x / ap
        total += delta
        if abs(delta) < abs(total) * 1e-16:
            break
    return total * math.exp(-x + a * math.log(x) - math.lgamma(a))


def gcf(a, x):
    tiny = 1e-300
    b = x + 1 - a
    c, d = 1 / tiny, 1 / b
    h = d
    for i in range(1, 100000):
        an = -i * (i - a)
        b += 2
        d = an * d + b
        d = tiny if abs(d) < tiny else d
        c = b + an / c
        c = tiny if abs(c) < tiny else c
        d = 1 / d
        delta = d * c
        h *= delta
        if abs(delta - 1) < 1e-16:
            break
    return math.exp(-x + a * math.log(x) - math.lgamma(a)) * h


def gammq(a, x):
    """Regularized upper incomplete gamma Q(a, x): the chi-square survival is Q(df/2, x/2)."""
    if x <= 0:
        return 1.0
    return 1 - gser(a, x) if x < a + 1 else gcf(a, x)


# ---------- tests ----------

def t_p_value(t, df, alternative):
    if alternative == "two-sided":
        return betai(df / 2, 0.5, df / (df + t * t))
    if alternative == "less":
        return t_cdf(t, df)
    return t_cdf(-t, df)


def z_p_value(z, alternative):
    if alternative == "two-sided":
        return math.erfc(abs(z) / math.sqrt(2))
    if alternative == "less":
        return 0.5 * math.erfc(-z / math.sqrt(2))
    return 0.5 * math.erfc(z / math.sqrt(2))


def mean_var(data):
    n = len(data)
    mean = sum(data) / n
    return mean, sum((v - mean) ** 2 for v in data) / (n - 1)


def t_result(name, t, df, se, estimate, mu, alternative, confidence, extra):
    q = t_quantile(1 - (1 - confidence) / 2, float(df))
    out = {"test": name, "statistic": num(t), "df": num(df), "p_value": num(t_p_value(t, float(df), alternative)),
           "alternative": alternative, "estimate": num(estimate), "standard_error": num(se),
           "confidence_interval": {"level": confidence, "low": num(float(estimate) - q * se), "high": num(float(estimate) + q * se)},
           "null_value": num(mu)}
    out.update(extra)
    return out


def two_sample(request, name, alternative, confidence):
    x, y = read(request.get("x"), "x"), read(request.get("y"), "y")
    mu = scalar(request, "mu", 0)
    mx, vx = mean_var(x)
    my, vy = mean_var(y)
    nx, ny = len(x), len(y)
    if name == "welch_t":
        a, b = vx / nx, vy / ny
        if a + b == 0:
            fail("both samples are constant: the test is undefined")
        se2 = a + b
        df = se2 ** 2 / (a ** 2 / (nx - 1) + b ** 2 / (ny - 1))
    else:
        pooled = ((nx - 1) * vx + (ny - 1) * vy) / (nx + ny - 2)
        if pooled == 0:
            fail("both samples are constant: the test is undefined")
        se2 = pooled * (Fraction(1, nx) + Fraction(1, ny))
        df = Fraction(nx + ny - 2)
    se = math.sqrt(se2)
    t = float(mx - my - mu) / se
    extra = {"mean_x": num(mx), "mean_y": num(my), "variance_x": num(vx), "variance_y": num(vy), "n_x": nx, "n_y": ny,
             "exact": {"mean_x": str(mx), "mean_y": str(my), "variance_x": str(vx), "variance_y": str(vy), "df": str(df)}}
    if name == "student_t":
        extra["exact"]["pooled_variance"] = str(pooled)
    return t_result(name, t, df, se, mx - my, mu, alternative, confidence, extra)


def paired(request, alternative, confidence):
    x, y = read(request.get("x"), "x"), read(request.get("y"), "y")
    if len(x) != len(y):
        fail("x and y must have the same length for a paired test")
    mu = scalar(request, "mu", 0)
    d = [a - b for a, b in zip(x, y)]
    md, vd = mean_var(d)
    if vd == 0:
        fail("the differences are constant: the test is undefined")
    se = math.sqrt(vd / len(d))
    t = float(md - mu) / se
    return t_result("paired_t", t, Fraction(len(d) - 1), se, md, mu, alternative, confidence,
                    {"n": len(d), "mean_difference": num(md), "exact": {"mean_difference": str(md), "variance_difference": str(vd)}})


def one_sample(request, alternative, confidence):
    x = read(request.get("x"), "x")
    mu = scalar(request, "mu", 0)
    mx, vx = mean_var(x)
    if vx == 0:
        fail("the sample is constant: the test is undefined")
    se = math.sqrt(vx / len(x))
    t = float(mx - mu) / se
    return t_result("one_sample_t", t, Fraction(len(x) - 1), se, mx, mu, alternative, confidence,
                    {"n": len(x), "mean": num(mx), "exact": {"mean": str(mx), "variance": str(vx)}})


def chi_gof(request):
    observed = counts(request.get("observed"), "observed", 2)
    total = sum(observed)
    if total == 0:
        fail("observed counts are all zero")
    k = len(observed)
    if "expected" in request and "proportions" in request:
        fail("give expected or proportions, not both")
    if "expected" in request:
        expected = read(request["expected"], "expected")
        if len(expected) != k or any(e <= 0 for e in expected):
            fail("expected must have one positive count per observed count")
        if sum(expected) != total:
            fail(f"expected counts add up to {sum(expected)}, observed to {total}: they must match (or give proportions)")
    else:
        weights = read(request["proportions"], "proportions") if "proportions" in request else [Fraction(1)] * k
        if len(weights) != k or any(w <= 0 for w in weights):
            fail("proportions must have one positive value per observed count")
        expected = [total * w / sum(weights) for w in weights]
    ddof = request.get("ddof", 0)
    if isinstance(ddof, bool) or not isinstance(ddof, int) or ddof < 0:
        fail("ddof must be a non-negative integer")
    df = k - 1 - ddof
    if df < 1:
        fail("no degrees of freedom left")
    stat = sum((o - e) ** 2 / e for o, e in zip(observed, expected))
    out = {"test": "chi_square_gof", "statistic": num(stat), "df": df, "p_value": num(gammq(df / 2, float(stat) / 2)),
           "expected": [num(e) for e in expected], "exact": {"statistic": str(stat)}}
    if min(expected) < 5:
        out["warning"] = "some expected counts are below 5: the chi-square approximation may be poor"
    return out


def table_of(request, two_by_two):
    table = request.get("table")
    if not isinstance(table, list) or len(table) < 2 or not all(isinstance(r, list) for r in table):
        fail("table must be a list of at least two rows")
    width = len(table[0])
    if width < 2 or any(len(r) != width for r in table):
        fail("table rows must all have the same length, at least 2")
    for i, row in enumerate(table):
        counts(row, f"table[{i}]", 2)
    if two_by_two and (len(table) != 2 or width != 2):
        fail("fisher_exact takes a 2x2 table")
    return table


def chi_independence(request):
    table = table_of(request, False)
    rows = [sum(r) for r in table]
    cols = [sum(c) for c in zip(*table)]
    n = sum(rows)
    if 0 in rows or 0 in cols:
        fail("a row or column of the table is all zeros")
    yates = request.get("yates", True)
    if not isinstance(yates, bool):
        fail("yates must be true or false")
    is_2x2 = len(table) == 2 and len(cols) == 2
    expected = [[Fraction(r * c, n) for c in cols] for r in rows]
    stat = Fraction(0)
    for orow, erow in zip(table, expected):
        for o, e in zip(orow, erow):
            dev = abs(o - e)
            if is_2x2 and yates:
                dev = max(dev - Fraction(1, 2), Fraction(0))
            stat += dev ** 2 / e
    df = (len(rows) - 1) * (len(cols) - 1)
    out = {"test": "chi_square_independence", "statistic": num(stat), "df": df, "p_value": num(gammq(df / 2, float(stat) / 2)),
           "yates_correction": is_2x2 and yates, "expected": [[num(e) for e in r] for r in expected],
           "cramers_v": num(math.sqrt(float(stat) / (n * (min(len(rows), len(cols)) - 1)))), "exact": {"statistic": str(stat)}}
    if min(min(r) for r in expected) < 5:
        out["warning"] = "some expected counts are below 5: consider fisher_exact for a 2x2 table"
    return out


def fisher(request, alternative):
    (a, b), (c, d) = table_of(request, True)
    r1, r2, c1 = a + b, c + d, a + c
    n = r1 + r2
    if n > MAX_FISHER_N:
        fail(f"the table total must be at most {MAX_FISHER_N}")
    lo, hi = max(0, c1 - r2), min(r1, c1)
    probs, t = {}, math.comb(r1, lo) * math.comb(r2, c1 - lo)
    for k in range(lo, hi + 1):
        probs[k] = t
        if k < hi:
            t = t * (r1 - k) * (c1 - k) // ((k + 1) * (r2 - c1 + k + 1))
    den = math.comb(n, c1)
    observed = probs[a]
    if alternative == "two-sided":
        p = Fraction(sum(v for v in probs.values() if v <= observed), den)
    elif alternative == "less":
        p = Fraction(sum(v for k, v in probs.items() if k <= a), den)
    else:
        p = Fraction(sum(v for k, v in probs.items() if k >= a), den)
    odds = None if b * c == 0 else Fraction(a * d, b * c)
    return {"test": "fisher_exact", "p_value": num(p), "alternative": alternative,
            "sample_odds_ratio": num(odds) if odds is not None else None,
            "exact": {"p_value": str(p) if p.denominator.bit_length() < 4000 else None,
                      "sample_odds_ratio": str(odds) if odds is not None else None},
            "note": "two-sided p sums every table with the observed margins whose probability is at most the observed one (as R's fisher.test); the odds ratio is the sample ad/bc, not R's conditional estimate"}


def one_prop(request, alternative, confidence):
    x, n = request.get("successes"), request.get("trials")
    if isinstance(x, bool) or isinstance(n, bool) or not isinstance(x, int) or not isinstance(n, int) or n < 1 or not 0 <= x <= n:
        fail("successes and trials must be integers with 0 <= successes <= trials and trials >= 1")
    p0 = scalar(request, "p0", "0.5")
    if not 0 < p0 < 1:
        fail("p0 must be strictly between 0 and 1")
    phat = Fraction(x, n)
    se0 = math.sqrt(p0 * (1 - p0) / n)
    z = float(phat - p0) / se0
    zq = NormalDist().inv_cdf(1 - (1 - confidence) / 2)
    centre = (float(phat) + zq * zq / (2 * n)) / (1 + zq * zq / n)
    half = zq / (1 + zq * zq / n) * math.sqrt(float(phat * (1 - phat)) / n + zq * zq / (4 * n * n))
    return {"test": "one_proportion_z", "statistic": num(z), "p_value": num(z_p_value(z, alternative)), "alternative": alternative,
            "estimate": num(phat), "null_value": num(p0),
            "confidence_interval": {"level": confidence, "method": "Wilson", "low": num(centre - half), "high": num(centre + half)},
            "exact": {"estimate": str(phat)}}


def two_prop(request, alternative, confidence):
    xs, ns = request.get("successes"), request.get("trials")
    if not (isinstance(xs, list) and isinstance(ns, list) and len(xs) == 2 and len(ns) == 2):
        fail("successes and trials must be pairs [group 1, group 2] for two_proportion_z")
    for x, n in zip(xs, ns):
        if isinstance(x, bool) or isinstance(n, bool) or not isinstance(x, int) or not isinstance(n, int) or n < 1 or not 0 <= x <= n:
            fail("each successes and trials pair must be integers with 0 <= successes <= trials and trials >= 1")
    p1, p2 = Fraction(xs[0], ns[0]), Fraction(xs[1], ns[1])
    pooled = Fraction(xs[0] + xs[1], ns[0] + ns[1])
    var0 = pooled * (1 - pooled) * (Fraction(1, ns[0]) + Fraction(1, ns[1]))
    if var0 == 0:
        fail("the pooled proportion is 0 or 1: the test is undefined")
    z = float(p1 - p2) / math.sqrt(var0)
    zq = NormalDist().inv_cdf(1 - (1 - confidence) / 2)
    se = math.sqrt(float(p1 * (1 - p1) / ns[0] + p2 * (1 - p2) / ns[1]))
    return {"test": "two_proportion_z", "statistic": num(z), "p_value": num(z_p_value(z, alternative)), "alternative": alternative,
            "estimate": num(p1 - p2), "proportions": [num(p1), num(p2)], "pooled_proportion": num(pooled),
            "confidence_interval": {"level": confidence, "method": "Wald, unpooled", "low": num(float(p1 - p2) - zq * se), "high": num(float(p1 - p2) + zq * se)},
            "exact": {"estimate": str(p1 - p2), "pooled_proportion": str(pooled)}}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    name = request.get("test")
    if name not in TESTS:
        fail("test must be one of " + ", ".join(TESTS))
    alternative = request.get("alternative", "two-sided")
    if alternative not in ("two-sided", "less", "greater"):
        fail("alternative must be two-sided, less or greater")
    confidence = request.get("confidence", 0.95)
    if isinstance(confidence, bool) or not isinstance(confidence, (int, float)) or not 0 < confidence < 1:
        fail("confidence must be a number between 0 and 1")
    if name in ("welch_t", "student_t"):
        out = two_sample(request, name, alternative, confidence)
    elif name == "paired_t":
        out = paired(request, alternative, confidence)
    elif name == "one_sample_t":
        out = one_sample(request, alternative, confidence)
    elif name == "chi_square_gof":
        out = chi_gof(request)
    elif name == "chi_square_independence":
        out = chi_independence(request)
    elif name == "fisher_exact":
        out = fisher(request, alternative)
    elif name == "one_proportion_z":
        out = one_prop(request, alternative, confidence)
    else:
        out = two_prop(request, alternative, confidence)
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
