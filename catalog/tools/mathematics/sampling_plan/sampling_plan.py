#!/usr/bin/env python3
"""Survey sample sizes: the sample needed to estimate a proportion or a mean to a given margin
of error, with the finite-population correction, a design effect and the expected response
rate; and the allocation of a total sample across strata (proportional, Neyman or equal).

Input (JSON on stdin):
  target      "proportion" | "mean" | "stratified"
  margin      margin of error: a proportion (0.05 = five points) for proportion, data units for mean
  confidence  0.95 by default (z from the normal quantile, or give z)
  z           optional fixed z (e.g. 1.96), overrides confidence
  p           expected proportion, default 0.5 (the most conservative)
  sd          expected standard deviation (mean)
  distribution "z" (default) or "t" (mean: iterate on Student's t with n - 1 df)
  population  population size N for the finite-population correction (optional)
  design_effect  default 1
  response_rate  expected response rate, default 1: the number to contact is n / rate
  n           total sample (stratified)
  strata      [{"name", "size", "sd"}] (stratified; sd needed for neyman and for the variance)
  method      "proportional" (default) | "neyman" | "equal"
"""
import json
import math
import sys
from fractions import Fraction
from statistics import NormalDist


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(x):
    value = float(x)
    return float(f"{value:.12g}") if math.isfinite(value) else None


def frac(request, field, default=None, low=None, high=None, low_open=False, required=False):
    if field not in request:
        if required:
            fail(f"{field} is required")
        return None if default is None else Fraction(default)
    value = request[field]
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number")
    try:
        out = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number")
    if low is not None and (out < low or (low_open and out == low)):
        fail(f"{field} must be {'greater than' if low_open else 'at least'} {low}")
    if high is not None and out > high:
        fail(f"{field} must be at most {high}")
    return out


# ---------- Student t quantile through the regularized incomplete beta (copied from stats_describe) ----------

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


# ---------- sizes ----------

def common(request):
    confidence = frac(request, "confidence", "0.95", 0, 1, low_open=True)
    if confidence >= 1:
        fail("confidence must be between 0 and 1")
    if "z" in request:
        z = float(frac(request, "z", low=0, low_open=True))
    else:
        z = NormalDist().inv_cdf(1 - (1 - float(confidence)) / 2)
    population = request.get("population")
    if population is not None and (isinstance(population, bool) or not isinstance(population, int) or population < 1):
        fail("population must be a positive integer")
    deff = frac(request, "design_effect", 1, low=0, low_open=True)
    rate = frac(request, "response_rate", 1, low=0, high=1, low_open=True)
    return confidence, z, population, deff, rate


def fpc(n0, population):
    """Cochran (1977), ch. 4: n = n0 / (1 + (n0 - 1) / N)."""
    return n0 if population is None else n0 / (1 + (n0 - 1) / population)


def finish(out, n_float, population, rate):
    n = max(1, math.ceil(n_float - 1e-9))
    if population is not None:
        n = min(n, population)
    out["n"] = n
    out["n_unrounded"] = num(n_float)
    out["response_rate"] = num(rate)
    out["n_to_contact"] = math.ceil(Fraction(n) / rate)
    return out


def proportion(request):
    confidence, z, population, deff, rate = common(request)
    margin = frac(request, "margin", low=0, high=1, low_open=True, required=True)
    p = frac(request, "p", "0.5", 0, 1)
    if p in (0, 1):
        fail("p must be strictly between 0 and 1")
    n0 = z * z * float(p * (1 - p)) / float(margin) ** 2 * float(deff)
    out = {"target": "proportion", "confidence": num(confidence), "z": num(z), "margin": num(margin), "p": num(p),
           "design_effect": num(deff), "n0": num(n0), "formula": "n0 = z^2 p (1 - p) deff / e^2; n = n0 / (1 + (n0 - 1) / N)"}
    return finish(out, fpc(n0, population), population, rate)


def mean(request):
    confidence, z, population, deff, rate = common(request)
    margin = frac(request, "margin", low=0, low_open=True, required=True)
    sd = frac(request, "sd", low=0, low_open=True, required=True)
    dist = request.get("distribution", "z")
    if dist not in ("z", "t"):
        fail("distribution must be z or t")
    ratio = float(sd) / float(margin)
    n0 = (z * ratio) ** 2 * float(deff)
    out = {"target": "mean", "confidence": num(confidence), "margin": num(margin), "sd": num(sd), "design_effect": num(deff),
           "distribution": dist, "z": num(z), "n0": num(n0), "formula": "n0 = (z s / e)^2 deff; n = n0 / (1 + (n0 - 1) / N)"}
    n_float = fpc(n0, population)
    if dist == "t":
        if "z" in request:
            fail("give confidence, not z, with distribution t")
        n = max(2, math.ceil(n_float))
        for _ in range(100):
            t = t_quantile(1 - (1 - float(confidence)) / 2, n - 1)
            n_float = fpc((t * ratio) ** 2 * float(deff), population)
            new = max(2, math.ceil(n_float - 1e-9))
            if new == n:
                break
            n = new
        out["t"] = num(t)
        out["df"] = n - 1
    return finish(out, n_float, population, rate)


def largest_remainder(shares, total):
    floors = [math.floor(s) for s in shares]
    left = total - sum(floors)
    order = sorted(range(len(shares)), key=lambda i: (-(shares[i] - floors[i]), i))
    for i in order[:left]:
        floors[i] += 1
    return floors


def stratified(request):
    n = request.get("n")
    if isinstance(n, bool) or not isinstance(n, int) or n < 1:
        fail("n must be a positive integer (the total sample)")
    strata = request.get("strata")
    if not isinstance(strata, list) or len(strata) < 2 or len(strata) > 1000:
        fail("strata must be a list of 2 to 1000 strata")
    method = request.get("method", "proportional")
    if method not in ("proportional", "neyman", "equal"):
        fail("method must be proportional, neyman or equal")
    sizes, sds, names = [], [], []
    for i, s in enumerate(strata):
        if not isinstance(s, dict) or set(s) - {"name", "size", "sd"}:
            fail(f"strata[{i}] must be an object with name, size and sd")
        size = s.get("size")
        if isinstance(size, bool) or not isinstance(size, int) or size < 1:
            fail(f"strata[{i}].size must be a positive integer")
        sizes.append(size)
        names.append(str(s.get("name", f"stratum {i + 1}")))
        sds.append(frac(s, "sd", low=0) if "sd" in s else None)
    total_N = sum(sizes)
    if n > total_N:
        fail(f"n ({n}) is larger than the population ({total_N})")
    if method == "neyman":
        if any(sd is None for sd in sds):
            fail("neyman allocation needs sd for every stratum")
        weights = [Fraction(N) * sd for N, sd in zip(sizes, sds)]
        if sum(weights) == 0:
            fail("all standard deviations are zero")
    elif method == "proportional":
        weights = [Fraction(N) for N in sizes]
    else:
        weights = [Fraction(1)] * len(sizes)
    shares = [n * w / sum(weights) for w in weights]
    alloc = largest_remainder(shares, n)
    rows, warnings = [], []
    for name, N, sd, share, nh in zip(names, sizes, sds, shares, alloc):
        rows.append({"name": name, "size": N, "sd": num(sd) if sd is not None else None, "exact_share": str(share), "n": nh})
        if nh > N:
            warnings.append(f"{name}: allocation {nh} exceeds the stratum size {N}; take the whole stratum and reallocate the rest")
        if nh == 0:
            warnings.append(f"{name}: no units allocated")
    out = {"target": "stratified", "method": method, "n": n, "population": total_N, "strata": rows,
           "formula": {"proportional": "n_h = n N_h / N", "neyman": "n_h = n N_h S_h / sum(N_i S_i)", "equal": "n_h = n / H"}[method],
           "rounding": "largest remainder, so the allocations add up to n"}
    if all(sd is not None for sd in sds) and all(0 < nh for nh in alloc):
        var = sum((Fraction(N, total_N)) ** 2 * sd * sd / nh * (1 - Fraction(nh, N)) for N, sd, nh in zip(sizes, sds, alloc))
        out["variance_of_mean"] = num(var)
        out["standard_error_of_mean"] = num(math.sqrt(var))
    if warnings:
        out["warnings"] = warnings
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    target = request.get("target")
    if target == "proportion":
        out = proportion(request)
    elif target == "mean":
        out = mean(request)
    elif target == "stratified":
        out = stratified(request)
    else:
        fail("target must be proportion, mean or stratified")
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
