#!/usr/bin/env python3
"""Integer number theory: factorisation (trial division, then Pollard's rho in Brent's form with
fixed seeds), primality (Miller-Rabin: deterministic below 3.3e24, strong probable prime with
stated bases above), gcd, lcm and the extended Euclidean algorithm, modular inverse and power,
Euler's totient, divisors, the Chinese remainder theorem (moduli need not be coprime), and the
next and previous prime.

Input (JSON on stdin):
  operation  "factor" | "is_prime" | "gcd" | "lcm" | "egcd" | "mod_inverse" | "mod_pow" |
             "totient" | "divisors" | "crt" | "next_prime" | "prev_prime"
  n          integer (factor, is_prime, totient, divisors, next_prime, prev_prime)
  values     [integers] (gcd, lcm)
  a, b       integers (egcd; mod_inverse takes a and m)
  base, exponent, m   (mod_pow; a negative exponent uses the inverse)
  residues, moduli    lists (crt)
Integers may be JSON numbers or decimal strings (up to 200 digits); integer results are strings.
"""
import json
import math
import sys
import time

MAX_DIGITS = 200
BUDGET_SECONDS = 3.0
DETERMINISTIC_LIMIT = 3317044064679887385961981  # Sorenson and Webster (2015): bases 2..41 suffice below this
DET_BASES = [2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41]
PROB_BASES = DET_BASES + [43, 47, 53, 59, 61, 67, 71, 73, 79, 83, 89, 97]
SMALL_PRIMES = []


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def integer(request, field, low=None):
    if field not in request:
        fail(f"{field} is required")
    return as_int(request[field], field, low)


def as_int(value, field, low=None):
    if isinstance(value, bool):
        fail(f"{field} must be an integer")
    if isinstance(value, str):
        text = value.strip().replace("_", "")
        body = text[1:] if text[:1] in "+-" else text
        if not body.isdigit() or not body.isascii():
            fail(f"{field} must be an integer or a string of decimal digits")
        if len(body) > MAX_DIGITS:
            fail(f"{field} may have at most {MAX_DIGITS} digits")
        value = int(text)
    elif not isinstance(value, int):
        fail(f"{field} must be an integer")
    elif value and len(str(abs(value))) > MAX_DIGITS:
        fail(f"{field} may have at most {MAX_DIGITS} digits")
    if low is not None and value < low:
        fail(f"{field} must be at least {low}")
    return value


def int_list(request, field, minimum=1):
    values = request.get(field)
    if not isinstance(values, list) or len(values) < minimum or len(values) > 1000:
        fail(f"{field} must be a list of {minimum} to 1000 integers")
    return [as_int(v, f"{field}[{i}]") for i, v in enumerate(values)]


def sieve(limit):
    flags = bytearray([1]) * (limit + 1)
    flags[0:2] = b"\x00\x00"
    for i in range(2, math.isqrt(limit) + 1):
        if flags[i]:
            flags[i * i::i] = bytearray(len(flags[i * i::i]))
    return [i for i, f in enumerate(flags) if f]


SMALL_PRIMES = sieve(10000)
SMALL_SET = set(SMALL_PRIMES)


def strong_probable_prime(n, a):
    d, s = n - 1, 0
    while d % 2 == 0:
        d, s = d // 2, s + 1
    x = pow(a, d, n)
    if x in (1, n - 1):
        return True
    for _ in range(s - 1):
        x = x * x % n
        if x == n - 1:
            return True
    return False


def is_prime(n):
    """(is prime, certainty) where certainty is "deterministic" or "probable"."""
    if n < 2:
        return False, "deterministic"
    if n <= SMALL_PRIMES[-1]:
        return n in SMALL_SET, "deterministic"
    for p in SMALL_PRIMES[:50]:
        if n % p == 0:
            return False, "deterministic"
    bases = DET_BASES if n < DETERMINISTIC_LIMIT else PROB_BASES
    for a in bases:
        if not strong_probable_prime(n, a):
            return False, "deterministic"
    return True, "deterministic" if n < DETERMINISTIC_LIMIT else "probable"


def brent(n, c, deadline):
    """Pollard's rho, Brent's variant, f(x) = x^2 + c, x0 = 2. A factor, n on failure, None on timeout."""
    y, r, q, g, m = 2, 1, 1, 1, 128
    x = ys = y
    while g == 1:
        x = y
        for _ in range(r):
            y = (y * y + c) % n
        k = 0
        while k < r and g == 1:
            ys = y
            for _ in range(min(m, r - k)):
                y = (y * y + c) % n
                q = q * abs(x - y) % n
            g = math.gcd(q, n)
            k += m
            if time.monotonic() > deadline:
                return None
        r *= 2
    if g == n:
        g = 1
        while g == 1:
            ys = (ys * ys + c) % n
            g = math.gcd(abs(x - ys), n)
    return g


def factorise(n):
    """{prime: exponent}, unfactored composites, and whether any factor is only a probable prime."""
    deadline = time.monotonic() + BUDGET_SECONDS
    factors, left, probable = {}, [], False
    for p in SMALL_PRIMES:
        if p * p > n:
            break
        while n % p == 0:
            factors[p] = factors.get(p, 0) + 1
            n //= p
    stack = [n] if n > 1 else []
    while stack:
        m = stack.pop()
        prime, certainty = is_prime(m)
        if prime:
            factors[m] = factors.get(m, 0) + 1
            probable = probable or certainty == "probable"
            continue
        root = math.isqrt(m)
        if root * root == m:
            stack += [root, root]
            continue
        found = None
        for c in range(1, 50):
            d = brent(m, c, deadline)
            if d is None:
                break
            if 1 < d < m:
                found = d
                break
        if found is None:
            left.append(m)
        else:
            stack += [found, m // found]
    return dict(sorted(factors.items())), sorted(left), probable


def complete_factors(n, what):
    factors, left, probable = factorise(n)
    if left:
        fail(f"{what} needs the full factorisation, and {left[0]} was not split within {BUDGET_SECONDS:g} s")
    return factors, probable


def egcd(a, b):
    x0, y0, x1, y1 = 1, 0, 0, 1
    while b:
        q, a, b = a // b, b, a % b
        x0, x1 = x1, x0 - q * x1
        y0, y1 = y1, y0 - q * y1
    return a, x0, y0


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    op = request.get("operation")
    s = str
    if op == "factor":
        n = integer(request, "n")
        if n == 0:
            fail("0 has no factorisation")
        factors, left, probable = factorise(abs(n))
        out = {"operation": op, "n": s(n), "sign": -1 if n < 0 else 1,
               "factors": [{"prime": s(p), "exponent": e} for p, e in factors.items()],
               "complete": not left, "unfactored": [s(m) for m in left],
               "product": " * ".join(f"{p}^{e}" if e > 1 else s(p) for p, e in factors.items()) or "1"}
        if probable:
            out["note"] = "some factors are strong probable primes (above 3.3e24), not proven primes"
        if left:
            out["note"] = f"the listed composites were not split within {BUDGET_SECONDS:g} s"
    elif op == "is_prime":
        n = integer(request, "n")
        prime, certainty = is_prime(n)
        out = {"operation": op, "n": s(n), "is_prime": prime, "certainty": certainty}
        if n >= 2 and n > SMALL_PRIMES[-1]:
            used = DET_BASES if n < DETERMINISTIC_LIMIT else PROB_BASES
            out["method"] = f"Miller-Rabin with bases {used[0]} to {used[-1]}" + (
                " (deterministic below 3317044064679887385961981)" if n < DETERMINISTIC_LIMIT else ": a strong probable prime to the first 25 prime bases; composites passing all of them can exist, so this is not a proof")
        if not prime and n >= 2:
            small = next((p for p in SMALL_PRIMES if n % p == 0 and p != n), None)
            if small:
                out["smallest_factor"] = s(small)
    elif op in ("gcd", "lcm"):
        values = int_list(request, "values", 2)
        if op == "gcd":
            result = math.gcd(*values)
        else:
            result = 0 if 0 in values else abs(math.lcm(*values))
        out = {"operation": op, "values": [s(v) for v in values], "result": s(result)}
    elif op == "egcd":
        a, b = integer(request, "a"), integer(request, "b")
        g, x, y = egcd(abs(a), abs(b))
        x, y = (x if a >= 0 else -x), (y if b >= 0 else -y)
        out = {"operation": op, "gcd": s(g), "x": s(x), "y": s(y), "identity": f"{a}*({x}) + {b}*({y}) = {g}"}
    elif op == "mod_inverse":
        a, m = integer(request, "a"), integer(request, "m", 1)
        g, x, _ = egcd(a % m, m)
        out = {"operation": op, "a": s(a), "m": s(m), "gcd": s(g), "inverse": s(x % m) if g == 1 else None}
        if g != 1:
            out["reason"] = f"gcd({a}, {m}) = {g}: no inverse exists"
    elif op == "mod_pow":
        base, e, m = integer(request, "base"), integer(request, "exponent"), integer(request, "m", 1)
        if e < 0 and math.gcd(base, m) != 1:
            fail("a negative exponent needs base and m coprime")
        out = {"operation": op, "result": s(pow(base, e, m)), "formula": f"{base}^{e} mod {m}"}
    elif op in ("totient", "divisors"):
        n = integer(request, "n", 1)
        factors, probable = complete_factors(n, op)
        if op == "totient":
            phi = n
            for p in factors:
                phi = phi // p * (p - 1)
            out = {"operation": op, "n": s(n), "totient": s(phi), "factors": [{"prime": s(p), "exponent": e} for p, e in factors.items()]}
        else:
            count = math.prod(e + 1 for e in factors.values())
            sigma = math.prod((p ** (e + 1) - 1) // (p - 1) for p, e in factors.items())
            out = {"operation": op, "n": s(n), "count": count, "sum": s(sigma)}
            if count <= 10000:
                divs = [1]
                for p, e in factors.items():
                    divs = [d * p ** k for d in divs for k in range(e + 1)]
                out["divisors"] = [s(d) for d in sorted(divs)]
            else:
                out["divisors"] = None
                out["note"] = "more than 10000 divisors: only count and sum"
        if probable:
            out["note"] = "the factorisation relies on strong probable primes"
    elif op == "crt":
        residues, moduli = int_list(request, "residues"), int_list(request, "moduli")
        if len(residues) != len(moduli):
            fail("residues and moduli must have the same length")
        if any(m < 1 for m in moduli):
            fail("moduli must be positive")
        x, mod = 0, 1
        for r, m in zip(residues, moduli):
            g, p, _ = egcd(mod, m)
            if (r - x) % g:
                out = {"operation": op, "solution": None, "reason": f"x = {x} (mod {mod}) and x = {r} (mod {m}) are incompatible: {r} - {x} is not divisible by gcd {g}"}
                break
            lcm = mod // g * m
            x = (x + (r - x) // g * p % (m // g) * mod) % lcm
            mod = lcm
        else:
            out = {"operation": op, "solution": s(x), "modulus": s(mod), "statement": f"x = {x} (mod {mod})"}
    elif op in ("next_prime", "prev_prime"):
        n = integer(request, "n")
        step = 1 if op == "next_prime" else -1
        if op == "prev_prime" and n <= 2:
            out = {"operation": op, "n": s(n), "prime": None, "reason": "no prime is smaller than 2"}
        else:
            k = max(n + 1, 2) if step == 1 else n - 1
            deadline = time.monotonic() + BUDGET_SECONDS
            while not is_prime(k)[0]:
                k += step
                if time.monotonic() > deadline:
                    fail("search exceeded the time budget")
            out = {"operation": op, "n": s(n), "prime": s(k), "gap": abs(k - n), "certainty": is_prime(k)[1]}
    else:
        fail("operation must be factor, is_prime, gcd, lcm, egcd, mod_inverse, mod_pow, totient, divisors, crt, next_prime or prev_prime")
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
