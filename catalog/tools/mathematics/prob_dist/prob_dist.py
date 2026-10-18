#!/usr/bin/env python3
"""Probability distributions and counting: pmf or pdf, cdf, survival function, interval
probability and quantile of the binomial, Poisson, normal, hypergeometric, geometric,
negative binomial, exponential and uniform distributions, with mean and variance; and exact
counting (n choose k, permutations, multisets, multinomial coefficients).

Discrete distributions with rational parameters are computed exactly over Fractions; the
Poisson, normal and exponential values are floats (the Poisson pmf is also given in symbolic
form, e.g. e^(-2) * 2^0 / 0!).

Input (JSON on stdin), one of:
  distribution  "binomial" (n, p) | "poisson" (lambda) | "normal" (mean, sd) |
                "hypergeometric" (population, successes, draws) | "geometric" (p; X = trials
                up to and including the first success, 1, 2, ...) | "negative_binomial" (r, p;
                X = failures before the r-th success, 0, 1, ...) | "exponential" (rate) |
                "uniform" (low, high; continuous)
    x           point: pmf/pdf, cdf P(X <= x) and survival P(X > x)
    between     [a, b]: P(a <= X <= b)
    q           probability: quantile (discrete: the smallest k with P(X <= k) >= q)
  operation     "choose" (n, k, repetition) | "permutations" (n, k optional, repetition) |
                "multinomial" (counts)
Parameters are numbers or decimal or fraction strings ("0.3", "1/3").
"""
import json
import math
import sys
from fractions import Fraction
from statistics import NormalDist

if hasattr(sys, "set_int_max_str_digits"):
    sys.set_int_max_str_digits(0)

MAX_N = 2000          # binomial n, hypergeometric population, negative binomial x and r
MAX_UNBOUNDED = 100000  # geometric x, Poisson x
MAX_LAMBDA = 100000
MAX_COUNT = 10000     # combinatorics
EXACT_BITS = 1000     # exact fractions longer than this (about 300 digits) are omitted


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(x):
    """A float for display, at 12 significant digits."""
    value = float(x)
    return float(f"{value:.12g}") if math.isfinite(value) else None


def exact(fr):
    fr = Fraction(fr)
    if fr.numerator.bit_length() + fr.denominator.bit_length() > EXACT_BITS:
        return None
    return str(fr)


def frac(request, field, low=None, high=None, low_open=False, required=True):
    if field not in request:
        if required:
            fail(f"{field} is required")
        return None
    value = request[field]
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal or fraction string")
    try:
        out = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number or a decimal or fraction string")
    if low is not None and (out < low or (low_open and out == low)):
        fail(f"{field} must be {'greater than' if low_open else 'at least'} {low}")
    if high is not None and out > high:
        fail(f"{field} must be at most {high}")
    return out


def integer(request, field, low=0, high=None, required=True):
    if field not in request:
        if required:
            fail(f"{field} is required")
        return None
    value = request[field]
    if isinstance(value, bool) or not isinstance(value, int):
        fail(f"{field} must be an integer")
    if value < low or (high is not None and value > high):
        fail(f"{field} must be an integer from {low} to {high}" if high is not None else f"{field} must be an integer >= {low}")
    return value


# ---------- incomplete gamma (Numerical Recipes 6.2) for the Poisson cdf ----------

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


def gammp(a, x):
    """Regularized lower incomplete gamma P(a, x)."""
    if x <= 0:
        return 0.0
    return gser(a, x) if x < a + 1 else 1 - gcf(a, x)


def gammq(a, x):
    if x <= 0:
        return 1.0
    return 1 - gser(a, x) if x < a + 1 else gcf(a, x)


# ---------- discrete distributions over Fractions ----------

class Finite:
    """A discrete distribution on lo..hi given by integer numerators over one denominator."""

    def __init__(self, lo, hi, terms, denominator):
        self.lo, self.hi, self.terms, self.den = lo, hi, terms, denominator
        self.cache = None

    def numerators(self):
        if self.cache is None:
            self.cache = list(self.terms())
        return self.cache

    def pmf(self, k):
        if k < self.lo or k > self.hi:
            return Fraction(0)
        return Fraction(self.numerators()[k - self.lo], self.den)

    def cdf(self, k):
        if k < self.lo:
            return Fraction(0)
        if k >= self.hi:
            return Fraction(1)
        return Fraction(sum(self.numerators()[: k - self.lo + 1]), self.den)

    def quantile(self, q):
        target, running = q * self.den, 0
        for i, t in enumerate(self.numerators()):
            running += t
            if running >= target:
                return self.lo + i
        return self.hi


def binomial(n, p):
    a, b = p.numerator, p.denominator
    c = b - a

    def terms():
        if a == 0 or c == 0:
            for k in range(n + 1):
                yield math.comb(n, k) * a ** k * c ** (n - k)
            return
        t = c ** n
        for k in range(n + 1):
            yield t
            if k < n:
                t = t * (n - k) * a // ((k + 1) * c)
    return Finite(0, n, terms, b ** n)


def hypergeometric(N, K, n):
    lo, hi = max(0, n - (N - K)), min(n, K)

    def terms():
        t = math.comb(K, lo) * math.comb(N - K, n - lo)
        for k in range(lo, hi + 1):
            yield t
            if k < hi:
                t = t * (K - k) * (n - k) // ((k + 1) * (N - K - n + k + 1))
    return Finite(lo, hi, terms, math.comb(N, n))


def negbin_numerators(r, p, upto):
    """Integer numerators N_k of the pmf of k = 0..upto failures, all over b^(r + upto) (p = a/b):
    N_k = C(k + r - 1, k) a^r c^k b^(upto - k), c = b - a."""
    a, b = p.numerator, p.denominator
    c = b - a
    t = a ** r * b ** upto
    out = []
    for k in range(upto + 1):
        out.append(t)
        if k < upto:
            t = t * (k + r) * c // ((k + 1) * b)
    return out, b ** (r + upto)


def discrete_point(dist, x):
    return {"pmf": dist.pmf(x), "cdf": dist.cdf(x), "sf": 1 - dist.cdf(x)}


def evaluate(request):
    name = request["distribution"]
    x = request.get("x")
    between = request.get("between")
    q = frac(request, "q", 0, 1, required=False)
    if between is not None:
        if not isinstance(between, list) or len(between) != 2:
            fail("between must be [a, b]")
        between = [frac({"between": v}, "between") for v in between]
        if between[0] > between[1]:
            fail("between must be [a, b] with a <= b")
    exact_out, floats, extra = {}, {}, {}

    def put(key, value, is_exact=True):
        if is_exact:
            exact_out[key] = exact(value)
        floats[key] = num(value)

    discrete = name in ("binomial", "hypergeometric", "geometric", "negative_binomial", "poisson")
    if discrete:
        if x is not None:
            x = frac(request, "x")
            if x.denominator != 1:
                fail("x must be an integer for a discrete distribution")
            x = int(x)
        if between is not None:
            if any(v.denominator != 1 for v in between):
                fail("between must hold integers for a discrete distribution")
            between = [int(v) for v in between]

    if name == "binomial":
        n = integer(request, "n", 0, MAX_N)
        p = frac(request, "p", 0, 1)
        dist = binomial(n, p)
        params = {"n": n, "p": str(p)}
        mean, var = n * p, n * p * (1 - p)
    elif name == "hypergeometric":
        N = integer(request, "population", 1, MAX_N)
        K = integer(request, "successes", 0, N)
        n = integer(request, "draws", 0, N)
        dist = hypergeometric(N, K, n)
        params = {"population": N, "successes": K, "draws": n}
        mean = Fraction(n * K, N)
        var = Fraction(n * K * (N - K) * (N - n), N * N * (N - 1)) if N > 1 else Fraction(0)
    elif name == "geometric":
        p = frac(request, "p", 0, 1, low_open=True)
        params = {"p": str(p)}
        mean, var = 1 / p, (1 - p) / p ** 2
        dist = None
    elif name == "negative_binomial":
        r = integer(request, "r", 1, MAX_N)
        p = frac(request, "p", 0, 1, low_open=True)
        params = {"r": r, "p": str(p)}
        mean, var = r * (1 - p) / p, r * (1 - p) / p ** 2
        dist = None
    elif name == "poisson":
        lam = frac(request, "lambda", 0, MAX_LAMBDA, low_open=True)
        params = {"lambda": str(lam)}
        mean, var = lam, lam
        dist = None
    elif name == "normal":
        mu = frac(request, "mean")
        sd = frac(request, "sd", 0, low_open=True)
        params = {"mean": str(mu), "sd": str(sd)}
        mean, var = mu, sd * sd
    elif name == "exponential":
        rate = frac(request, "rate", 0, low_open=True)
        params = {"rate": str(rate)}
        mean, var = 1 / rate, 1 / rate ** 2
    elif name == "uniform":
        lo, hi = frac(request, "low"), frac(request, "high")
        if lo >= hi:
            fail("low must be less than high")
        params = {"low": str(lo), "high": str(hi)}
        mean, var = (lo + hi) / 2, (hi - lo) ** 2 / 12
    else:
        fail("distribution must be binomial, poisson, normal, hypergeometric, geometric, negative_binomial, exponential or uniform")

    put("mean", mean)
    put("variance", var)

    # ----- point, interval, quantile -----
    if name in ("binomial", "hypergeometric"):
        if x is not None:
            for key, value in discrete_point(dist, x).items():
                put(key, value)
        if between is not None:
            put("between", dist.cdf(between[1]) - dist.cdf(between[0] - 1))
        if q is not None:
            floats["quantile"] = dist.quantile(q)
    elif name == "geometric":
        cdf = lambda k: Fraction(0) if k < 1 else 1 - (1 - p) ** k
        for k in [v for v in ([x] if x is not None else []) + (between or [])]:
            if k > MAX_UNBOUNDED:
                fail(f"x must be at most {MAX_UNBOUNDED} for the geometric distribution")
        if x is not None:
            put("pmf", (1 - p) ** (x - 1) * p if x >= 1 else Fraction(0))
            put("cdf", cdf(x))
            put("sf", 1 - cdf(x))
        if between is not None:
            put("between", cdf(between[1]) - cdf(between[0] - 1))
        if q is not None:
            if q == 0:
                floats["quantile"] = 1
            elif q == 1:
                floats["quantile"] = None if p < 1 else 1
                if p < 1:
                    extra["quantile_note"] = "the quantile at q = 1 is infinite"
            elif p == 1:
                floats["quantile"] = 1
            else:
                k = max(1, math.ceil(math.log1p(-float(q)) / math.log1p(-float(p))))
                if k > MAX_UNBOUNDED:
                    floats["quantile"] = k
                    extra["quantile_note"] = "computed in floating point"
                else:
                    while k > 1 and cdf(k - 1) >= q:
                        k -= 1
                    while cdf(k) < q:
                        k += 1
                    floats["quantile"] = k
    elif name == "negative_binomial":
        points = ([x] if x is not None else []) + (between or [])
        if any(k > MAX_N for k in points):
            fail(f"x must be at most {MAX_N} for the negative binomial distribution")
        top = max(points) if points else 0
        nums, den = negbin_numerators(r, p, max(top, 0))
        cdf = lambda k: Fraction(0) if k < 0 else Fraction(sum(nums[: k + 1]), den)
        if x is not None:
            put("pmf", Fraction(nums[x], den) if x >= 0 else Fraction(0))
            put("cdf", cdf(x))
            put("sf", 1 - cdf(x))
        if between is not None:
            put("between", cdf(between[1]) - cdf(between[0] - 1))
        if q is not None:
            if q == 1 and p < 1:
                floats["quantile"] = None
                extra["quantile_note"] = "the quantile at q = 1 is infinite"
            else:
                # floats in log space: exact Fractions with tiny p grow without bound
                qf, lp, lq = float(q), math.log(float(p)), (math.log1p(-float(p)) if p < 1 else 0.0)
                running, k = 0.0, 0
                while True:
                    running += math.exp(math.lgamma(k + r) - math.lgamma(r) - math.lgamma(k + 1) + r * lp + k * lq)
                    if running >= qf * (1 - 1e-12):
                        break
                    k += 1
                    if k > MAX_UNBOUNDED:
                        fail(f"the quantile is beyond {MAX_UNBOUNDED}")
                extra["quantile_note"] = "computed in floating point"
                floats["quantile"] = k
    elif name == "poisson":
        lf = float(lam)
        logpmf = lambda k: k * math.log(lf) - lf - math.lgamma(k + 1)
        cdf = lambda k: 0.0 if k < 0 else gammq(k + 1, lf)
        for k in ([x] if x is not None else []) + (between or []):
            if k > MAX_UNBOUNDED:
                fail(f"x must be at most {MAX_UNBOUNDED} for the Poisson distribution")
        if x is not None:
            floats["pmf"] = num(math.exp(logpmf(x))) if x >= 0 else 0.0
            floats["cdf"] = num(cdf(x))
            floats["sf"] = num(1 - cdf(x) if x < 0 else gammp(x + 1, lf))
            if x >= 0:
                exact_out["pmf_symbolic"] = f"e^(-{lam}) * {lam}^{x} / {x}!"
        if between is not None:
            floats["between"] = num(cdf(between[1]) - cdf(between[0] - 1))
        if q is not None:
            if q == 1:
                floats["quantile"] = None
                extra["quantile_note"] = "the quantile at q = 1 is infinite"
            else:
                k, running = 0, 0.0
                qf = float(q)
                while True:
                    running += math.exp(logpmf(k))
                    if running >= qf or k > lf + 60 * math.sqrt(lf) + 100:
                        break
                    k += 1
                floats["quantile"] = k
    elif name == "normal":
        m, s = float(mu), float(sd)
        cdf = lambda v: 0.5 * math.erfc(-(float(v) - m) / (s * math.sqrt(2)))
        if x is not None:
            xv = float(frac(request, "x"))
            floats["pdf"] = num(math.exp(-0.5 * ((xv - m) / s) ** 2) / (s * math.sqrt(2 * math.pi)))
            floats["cdf"] = num(cdf(xv))
            floats["sf"] = num(0.5 * math.erfc((xv - m) / (s * math.sqrt(2))))
            floats["z"] = num((xv - m) / s)
        if between is not None:
            floats["between"] = num(cdf(between[1]) - cdf(between[0]))
        if q is not None:
            floats["quantile"] = num(NormalDist(m, s).inv_cdf(float(q))) if 0 < q < 1 else None
            if q in (0, 1):
                extra["quantile_note"] = "the quantile at q = 0 or 1 is infinite"
    elif name == "exponential":
        rf = float(rate)
        if x is not None:
            xv = float(frac(request, "x"))
            floats["pdf"] = num(rf * math.exp(-rf * xv)) if xv >= 0 else 0.0
            floats["cdf"] = num(-math.expm1(-rf * xv)) if xv >= 0 else 0.0
            floats["sf"] = num(math.exp(-rf * xv)) if xv >= 0 else 1.0
        if between is not None:
            a, b = (max(float(v), 0.0) for v in between)
            floats["between"] = num(math.exp(-rf * a) - math.exp(-rf * b))
        if q is not None:
            if q == 1:
                floats["quantile"] = None
                extra["quantile_note"] = "the quantile at q = 1 is infinite"
            else:
                floats["quantile"] = num(-math.log1p(-float(q)) / rf)
    elif name == "uniform":
        cdf = lambda v: min(max((v - lo) / (hi - lo), Fraction(0)), Fraction(1))
        if x is not None:
            xv = frac(request, "x")
            put("pdf", 1 / (hi - lo) if lo <= xv <= hi else Fraction(0))
            put("cdf", cdf(xv))
            put("sf", 1 - cdf(xv))
        if between is not None:
            put("between", cdf(between[1]) - cdf(between[0]))
        if q is not None:
            put("quantile", lo + q * (hi - lo))

    result = {"distribution": name, "parameters": params}
    result.update(floats)
    result["exact"] = exact_out
    result.update(extra)
    if discrete and name != "poisson" and any(v is None for v in exact_out.values()):
        result["note"] = f"exact fractions over {EXACT_BITS} bits are omitted"
    return result


def combinatorics(request):
    op = request["operation"]
    rep = request.get("repetition", False)
    if not isinstance(rep, bool):
        fail("repetition must be true or false")
    if op == "choose":
        n, k = integer(request, "n", 0, MAX_COUNT), integer(request, "k", 0, MAX_COUNT)
        value = math.comb(n + k - 1, k) if rep else math.comb(n, k)
        formula = "C(n + k - 1, k)" if rep else "n! / (k! (n - k)!)"
        out = {"operation": op, "n": n, "k": k, "repetition": rep}
    elif op == "permutations":
        n = integer(request, "n", 0, MAX_COUNT)
        k = integer(request, "k", 0, MAX_COUNT, required=False)
        k = n if k is None else k
        if rep:
            if k > 2000:
                fail("k must be at most 2000 with repetition")
            value, formula = n ** k, "n^k"
        else:
            value, formula = math.perm(n, k), "n! / (n - k)!"
        out = {"operation": op, "n": n, "k": k, "repetition": rep}
    elif op == "multinomial":
        counts = request.get("counts")
        if not isinstance(counts, list) or not counts or any(isinstance(c, bool) or not isinstance(c, int) or c < 0 for c in counts):
            fail("counts must be a non-empty list of non-negative integers")
        if sum(counts) > MAX_COUNT:
            fail(f"the counts must add up to at most {MAX_COUNT}")
        value, rest = 1, 0
        for c in counts:
            rest += c
            value *= math.comb(rest, c)
        formula = "(k1 + ... + km)! / (k1! ... km!)"
        out = {"operation": op, "counts": counts}
    else:
        fail("operation must be choose, permutations or multinomial")
    out.update({"result": str(value), "digits": len(str(value)), "formula": formula})
    return out



def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if "distribution" in request and "operation" in request:
        fail("give distribution or operation, not both")
    if "distribution" in request:
        try:
            out = evaluate(request)
        except (OverflowError, ZeroDivisionError):
            fail("a parameter or x is too large for the floating-point computation")
        print(json.dumps(out))
    elif "operation" in request:
        print(json.dumps(combinatorics(request)))
    else:
        fail("give a distribution or an operation (choose, permutations, multinomial)")


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
