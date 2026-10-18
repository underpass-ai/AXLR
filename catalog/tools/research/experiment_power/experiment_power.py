#!/usr/bin/env python3
"""Statistical power and sample size for two-sample, one-sample and paired t tests and for the
comparison of two proportions: solve for the sample size, the power or the smallest detectable
effect.

t designs use the noncentral t distribution (method "t", default), computed with the series of
Lenth (1989, AS 243) over regularized incomplete beta functions, or the normal approximation
(method "normal"). Two proportions use the normal approximation with pooled variance under H0
(method "normal", as R's power.prop.test) or Cohen's arcsine effect size h (method "arcsine").
Two-sided power counts both rejection tails.

Input (JSON on stdin):
  design     "two_sample" | "one_sample" | "paired" | "two_proportions"
  solve      "n" | "power" | "effect"   (default: "power" when n is given, else "n")
  d          Cohen's d (t designs), or mean1 + mean2 + sd (two_sample: d = (mean1-mean2)/sd),
             mean + mu0 + sd (one_sample), mean_diff + sd_diff (paired: d_z)
  p1, p2     proportions (two_proportions; solve effect finds p2 > p1)
  alpha      significance level, default 0.05
  sides      2 (default) or 1
  power      target power (solve n, effect), default 0.8
  n          group 1 size (two_sample, two_proportions), or number of subjects/pairs
  ratio      n2 / n1, default 1
  method     "t" | "normal" (t designs), "normal" | "arcsine" (two_proportions)
"""
import json
import math
import sys
from statistics import NormalDist

Z = NormalDist()
DESIGNS = ("two_sample", "one_sample", "paired", "two_proportions")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(x, digits=10):
    return float(f"{x:.{digits}g}")


# ---------- regularized incomplete beta (Numerical Recipes 6.4), copied from hypothesis_test ----------

def betacf(a, b, x):
    tiny, qab, qap, qam = 1e-300, a + b, a + 1, a - 1
    c, d = 1.0, 1 - qab * x / qap
    d = 1 / (d if abs(d) > tiny else tiny)
    h = d
    for m in range(1, 500):
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
    lo, hi = -1e3, 1e3
    for _ in range(200):
        mid = (lo + hi) / 2
        if t_cdf(mid, df) < p:
            lo = mid
        else:
            hi = mid
    return (lo + hi) / 2


def nct_cdf(t, df, delta):
    """P(T <= t) for the noncentral t with df degrees of freedom and noncentrality delta
    (Lenth 1989, Applied Statistics algorithm AS 243: Poisson-weighted incomplete beta series)."""
    if t < 0:
        return 1 - nct_cdf(-t, df, -delta)
    if delta == 0:
        return t_cdf(t, df)
    x = t * t / (t * t + df)
    lam = delta * delta / 2
    total = 0.0
    j_peak = int(lam)
    j_min = max(0, int(lam - 12 * math.sqrt(lam) - 60))  # Poisson weights below are < 1e-30
    j_max = int(lam + 12 * math.sqrt(lam) + 60)
    for j in range(j_min, j_max + 1):
        log_w = -lam + j * math.log(lam) if lam > 0 else (0.0 if j == 0 else -math.inf)
        p = math.exp(log_w - math.lgamma(j + 1))
        q = delta / math.sqrt(2) * math.exp(log_w - math.lgamma(j + 1.5))
        term = p * betai(j + 0.5, df / 2, x) + q * betai(j + 1, df / 2, x)
        total += term
        if j > j_peak and abs(p) + abs(q) < 1e-17:
            break
    value = Z.cdf(-delta) + 0.5 * total
    return min(1.0, max(0.0, value))


# ---------- power functions: effect size and sizes -> power ----------

def t_power(design, d, n1, n2, alpha, sides, method):
    if design == "two_sample":
        df = n1 + n2 - 2
        ncp = abs(d) * math.sqrt(n1 * n2 / (n1 + n2))
    else:
        df = n1 - 1
        ncp = abs(d) * math.sqrt(n1)
    if method == "normal":
        crit = Z.inv_cdf(1 - alpha / sides)
        power = Z.cdf(ncp - crit) + (Z.cdf(-ncp - crit) if sides == 2 else 0)
        return power, {"df": None, "ncp": ncp, "critical_value": crit}
    crit = t_quantile(1 - alpha / sides, df)
    power = 1 - nct_cdf(crit, df, ncp)
    if sides == 2:
        power += nct_cdf(-crit, df, ncp)
    return power, {"df": df, "ncp": ncp, "critical_value": crit}


def prop_power(p1, p2, n1, n2, alpha, sides, method):
    crit = Z.inv_cdf(1 - alpha / sides)
    if method == "arcsine":
        h = 2 * math.asin(math.sqrt(p1)) - 2 * math.asin(math.sqrt(p2))
        shift = abs(h) * math.sqrt(n1 * n2 / (n1 + n2))
        power = Z.cdf(shift - crit) + (Z.cdf(-shift - crit) if sides == 2 else 0)
        return power, {"h": h, "critical_value": crit}
    pbar = (n1 * p1 + n2 * p2) / (n1 + n2)
    null_sd = math.sqrt(pbar * (1 - pbar) * (1 / n1 + 1 / n2))
    alt_sd = math.sqrt(p1 * (1 - p1) / n1 + p2 * (1 - p2) / n2)
    diff = abs(p1 - p2)
    power = Z.cdf((diff - crit * null_sd) / alt_sd)
    if sides == 2:
        power += Z.cdf((-diff - crit * null_sd) / alt_sd)
    return power, {"pooled_p": pbar, "critical_value": crit}


# ---------- input ----------

def number(request, field, low=None, high=None, low_open=True, high_open=True, required=True, default=None):
    if field not in request:
        if required:
            fail(f"{field} is required")
        return default
    v = request[field]
    if isinstance(v, bool) or not isinstance(v, (int, float, str)):
        fail(f"{field} must be a number")
    try:
        v = float(str(v).strip())
    except ValueError:
        fail(f"{field} must be a number")
    if not math.isfinite(v):
        fail(f"{field} must be finite")
    if low is not None and (v < low or (low_open and v == low)):
        fail(f"{field} must be {'>' if low_open else '>='} {low}")
    if high is not None and (v > high or (high_open and v == high)):
        fail(f"{field} must be {'<' if high_open else '<='} {high}")
    return v


def effect_size(request, design, solve):
    """Cohen's d for t designs, from d or from means and SD."""
    if solve == "effect":
        return None, "solved"
    if "d" in request:
        d = number(request, "d")
        source = "d"
    elif design == "two_sample" and {"mean1", "mean2", "sd"} <= set(request):
        d = (number(request, "mean1") - number(request, "mean2")) / number(request, "sd", 0)
        source = "(mean1 - mean2) / sd"
    elif design == "one_sample" and {"mean", "mu0", "sd"} <= set(request):
        d = (number(request, "mean") - number(request, "mu0")) / number(request, "sd", 0)
        source = "(mean - mu0) / sd"
    elif design == "paired" and {"mean_diff", "sd_diff"} <= set(request):
        d = number(request, "mean_diff") / number(request, "sd_diff", 0)
        source = "mean_diff / sd_diff (d_z)"
    else:
        need = {"two_sample": "mean1, mean2 and sd", "one_sample": "mean, mu0 and sd", "paired": "mean_diff and sd_diff"}[design]
        fail(f"give d, or {need}")
    if d == 0:
        fail("the effect size is zero: no sample size reaches the power")
    return d, source


def sizes(n1, ratio, design):
    if design in ("two_sample", "two_proportions"):
        return n1, n1 * ratio
    return n1, None


def bisect(f, lo, hi, increasing=True, steps=200):
    for _ in range(steps):
        mid = (lo + hi) / 2
        if (f(mid) < 0) == increasing:
            lo = mid
        else:
            hi = mid
    return (lo + hi) / 2


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"design", "solve", "d", "mean1", "mean2", "sd", "mean", "mu0", "mean_diff", "sd_diff", "p1", "p2",
               "alpha", "sides", "power", "n", "ratio", "method"}
    extra = set(request) - allowed
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    design = request.get("design")
    if design not in DESIGNS:
        fail("design must be two_sample, one_sample, paired or two_proportions")
    solve = request.get("solve", "power" if "n" in request else "n")
    if solve not in ("n", "power", "effect"):
        fail("solve must be n, power or effect")
    alpha = number(request, "alpha", 0, 1, required=False, default=0.05)
    sides = request.get("sides", 2)
    if sides not in (1, 2) or isinstance(sides, bool):
        fail("sides must be 1 or 2")
    ratio = number(request, "ratio", 0, required=False, default=1.0)
    if design in ("one_sample", "paired") and "ratio" in request:
        fail("ratio is for two_sample and two_proportions")
    proportions = design == "two_proportions"
    method = request.get("method", "normal" if proportions else "t")
    if method not in (("normal", "arcsine") if proportions else ("t", "normal")):
        fail("method must be " + ("normal or arcsine" if proportions else "t or normal"))
    minimum = 2
    n = None
    if solve in ("power", "effect"):
        if "n" not in request:
            fail(f"solve {solve} needs n")
        n = request["n"]
        if isinstance(n, bool) or not isinstance(n, int) or not minimum <= n <= 10_000_000:
            fail(f"n must be an integer from {minimum} to 10000000")
    target = None
    if solve in ("n", "effect"):
        target = number(request, "power", 0, 1, required=False, default=0.8)
        if target <= alpha / sides:
            fail("power must exceed alpha / sides")

    if proportions:
        p1 = number(request, "p1", 0, 1)
        p2 = None if solve == "effect" else number(request, "p2", 0, 1)
        if p2 is not None and p1 == p2:
            fail("p1 equals p2: no sample size reaches the power")
        power_of = lambda a, b, p2_=None: prop_power(p1, p2 if p2_ is None else p2_, a, b, alpha, sides, method)
        d, d_source = None, None
    else:
        d, d_source = effect_size(request, design, solve)
        power_of = lambda a, b, d_=None: t_power(design, d if d_ is None else d_, a, b, alpha, sides, method)

    out = {"design": design, "solve": solve, "alpha": alpha, "sides": sides,
           "method": {"t": "noncentral t (Lenth 1989, AS 243)", "normal": "normal approximation",
                      "arcsine": "normal approximation on Cohen's arcsine effect size h"}[method]
           if not (proportions and method == "normal") else "normal approximation, pooled variance under H0 (as R power.prop.test)"}
    if proportions:
        out["p1"] = p1
        if p2 is not None:
            out["p2"] = p2
            out["h"] = num(2 * math.asin(math.sqrt(p1)) - 2 * math.asin(math.sqrt(p2)))
    elif d is not None:
        out["d"] = num(d)
        out["d_from"] = d_source

    two_groups = design in ("two_sample", "two_proportions")
    if solve in ("power", "effect"):
        n1, n2 = n, None
        if two_groups:
            n2 = n * ratio
            if abs(n2 - round(n2)) > 1e-9:
                fail("n * ratio must be a whole number (the size of group 2)")
            n2 = int(round(n2))
    if solve == "power":
        power, extra_info = power_of(n1, n2)
    elif solve == "n":
        f = lambda m: power_of(*sizes(m, ratio, design))[0] - target
        lo = 2.0 if not proportions else 1.0
        hi = 4.0
        while f(hi) < 0:
            hi *= 2
            if hi > 1e9:
                fail("the required sample size exceeds 10^9")
        n_exact = bisect(f, lo, hi) if f(lo) < 0 else lo
        # the smallest whole sizes reaching the target: n1 rounded up, n2 = ceil(ratio * n1)
        whole = lambda m: (m, math.ceil(m * ratio - 1e-9)) if two_groups else (m, None)
        n1 = max(minimum, math.ceil(n_exact - 1e-9))
        while n1 > minimum and power_of(*whole(n1 - 1))[0] >= target:
            n1 -= 1
        while power_of(*whole(n1))[0] < target:
            n1 += 1
        n1, n2 = whole(n1)
        power, extra_info = power_of(n1, n2)
        out["n_exact"] = num(n_exact, 8)
    else:  # effect
        if proportions:
            g = lambda q: power_of(n1, n2, q)[0] - target
            if g(1 - 1e-12) < 0:
                fail("no p2 between p1 and 1 reaches the power with this n")
            p2 = bisect(g, p1, 1 - 1e-12)
            out["p2"] = num(p2, 8)
            out["h"] = num(2 * math.asin(math.sqrt(p1)) - 2 * math.asin(math.sqrt(p2)))
            power, extra_info = power_of(n1, n2, p2)
        else:
            g = lambda e: power_of(n1, n2, e)[0] - target
            hi = 1.0
            while g(hi) < 0:
                hi *= 2
                if hi > 1e6:
                    fail("no effect size reaches the power with this n")
            d = bisect(g, 1e-12, hi)
            out["d"] = num(d, 8)
            out["d_from"] = "solved"
            power, extra_info = power_of(n1, n2, d)

    if two_groups:
        out.update(n1=n1, n2=n2, total_n=n1 + n2, ratio=ratio)
    else:
        out["n"] = n1
    out["power"] = num(power)
    if target is not None:
        out["target_power"] = target
    for key, value in extra_info.items():
        out[key] = None if value is None else (value if isinstance(value, int) else num(value))
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
