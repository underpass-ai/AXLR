#!/usr/bin/env python3
"""Recognise a number sequence from its terms: arithmetic, geometric, polynomial (finite
differences) or a linear recurrence with constant coefficients, solved exactly over Fractions.
The simplest model that the given terms over-determine wins (fewest terms needed to fix it,
with at least one more term to check it); the answer says how many terms fix the model and how
many confirm it, and claims nothing beyond the given terms. Next terms and, where there is one,
a closed form in n.

Input (JSON on stdin):
  terms        [numbers or decimal/fraction strings], 3 to 200
  next         how many further terms to produce, default 5 (0 to 100)
  start_index  the index n of the first term, default 1
"""
import json
import math
import sys
from fractions import Fraction

if hasattr(sys, "set_int_max_str_digits"):
    sys.set_int_max_str_digits(0)

MAX_TERMS = 200
MAX_NEXT = 100
MAX_ORDER = 10


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_terms(values):
    if not isinstance(values, list) or not 3 <= len(values) <= MAX_TERMS:
        fail(f"terms must be a list of 3 to {MAX_TERMS} numbers")
    out = []
    for i, v in enumerate(values):
        if isinstance(v, bool) or not isinstance(v, (int, float, str)):
            fail(f"terms[{i}] is not a number")
        try:
            out.append(Fraction(str(v).strip()))
        except (ValueError, ZeroDivisionError):
            fail(f"terms[{i}] is not a number")
    return out


def fmt(c):
    return str(c)


def paren(c):
    return f"({c})" if c < 0 or c.denominator != 1 else str(c)


def poly_string(coefs):
    """coefs[k] multiplies n^k."""
    parts = []
    for k in range(len(coefs) - 1, -1, -1):
        c = coefs[k]
        if c == 0:
            continue
        mono = "" if k == 0 else ("n" if k == 1 else f"n^{k}")
        mag = abs(c)
        body = str(mag) if not mono else (mono if mag == 1 else f"{mag}*{mono}")
        if not parts:
            parts.append(("-" if c < 0 else "") + body)
        else:
            parts.append(("- " if c < 0 else "+ ") + body)
    return " ".join(parts) if parts else "0"


def solve_linear(m, rhs):
    """Unique solution of a square system over Fractions, or None when singular."""
    k = len(m)
    a = [row[:] + [r] for row, r in zip(m, rhs)]
    for c in range(k):
        p = next((i for i in range(c, k) if a[i][c] != 0), None)
        if p is None:
            return None
        a[c], a[p] = a[p], a[c]
        pv = a[c][c]
        a[c] = [v / pv for v in a[c]]
        for i in range(k):
            if i != c and a[i][c] != 0:
                f = a[i][c]
                a[i] = [x - f * y for x, y in zip(a[i], a[c])]
    return [a[i][k] for i in range(k)]


def binom_poly(shift, k):
    """Coefficients of C(n - shift, k) as a polynomial in n."""
    coefs = [Fraction(1)]
    for j in range(k):
        # multiply by (n - shift - j) / (j + 1)
        new = [Fraction(0)] * (len(coefs) + 1)
        for i, c in enumerate(coefs):
            new[i + 1] += c / (j + 1)
            new[i] += c * (-shift - j) / (j + 1)
        coefs = new
    return coefs


def polynomial_fit(terms, start, degree):
    """(coefficients in n) if the terms fit a polynomial of exactly this degree, else None."""
    rows, diffs = [terms[:]], terms[:]
    for _ in range(degree):
        diffs = [b - a for a, b in zip(diffs, diffs[1:])]
        rows.append(diffs)
    last = rows[degree]
    if len(last) < 2 or any(v != last[0] for v in last) or last[0] == 0:
        return None
    coefs = [Fraction(0)] * (degree + 1)
    for k in range(degree + 1):
        for i, c in enumerate(binom_poly(start, k)):
            coefs[i] += rows[k][0] * c
    return coefs


def poly_value(coefs, n):
    return sum(c * Fraction(n) ** k for k, c in enumerate(coefs))


def recurrence_fit(terms, order):
    if 2 * order + 1 > len(terms):
        return None
    m = [[terms[i - j] for j in range(1, order + 1)] for i in range(order, 2 * order)]
    coefs = solve_linear(m, terms[order:2 * order])
    if coefs is None or all(c == 0 for c in coefs):
        return None
    for i in range(2 * order, len(terms)):
        if terms[i] != sum(c * terms[i - j] for j, c in enumerate(coefs, 1)):
            return None
    return coefs


def recurrence_string(coefs):
    parts = []
    for j, c in enumerate(coefs, 1):
        if c == 0:
            continue
        body = f"a(n-{j})" if abs(c) == 1 else f"{abs(c)}*a(n-{j})"
        parts.append((("-" if c < 0 else "") + body) if not parts else (("- " if c < 0 else "+ ") + body))
    return "a(n) = " + " ".join(parts)


def rational_sqrt(q):
    if q < 0:
        return None
    a, b = math.isqrt(q.numerator), math.isqrt(q.denominator)
    return Fraction(a, b) if a * a == q.numerator and b * b == q.denominator else None


def characteristic(coefs):
    parts = [f"x^{len(coefs)}"]
    for j, c in enumerate(coefs, 1):
        k = len(coefs) - j
        if c == 0:
            continue
        mono = "" if k == 0 else ("x" if k == 1 else f"x^{k}")
        mag = abs(c)
        body = str(mag) if not mono else (mono if mag == 1 else f"{mag}*{mono}")
        parts.append(("+ " if c < 0 else "- ") + body)
    return " ".join(parts)


def order_two_closed_form(coefs, terms, start):
    """a(n) for a(n) = c1 a(n-1) + c2 a(n-2) when the characteristic roots are rational."""
    c1, c2 = coefs
    disc = c1 * c1 + 4 * c2
    root = rational_sqrt(disc)
    if root is None:
        kind = "complex" if disc < 0 else "irrational"
        return None, f"characteristic roots are {kind}: ({c1} +- sqrt({disc}))/2"
    if disc == 0:
        r = c1 / 2
        if r == 0:
            return None, None
        ab = solve_linear([[r ** start, start * r ** start], [r ** (start + 1), (start + 1) * r ** (start + 1)]], terms[:2])
        pieces = [(ab[0], r, ""), (ab[1], r, "n*")]
    else:
        r1, r2 = (c1 + root) / 2, (c1 - root) / 2
        if r1 == 0 or r2 == 0:
            return None, None
        ab = solve_linear([[r1 ** start, r2 ** start], [r1 ** (start + 1), r2 ** (start + 1)]], terms[:2])
        pieces = [(ab[0], r1, ""), (ab[1], r2, "")]
    parts = []
    for coef, base, n_factor in pieces:
        if coef == 0:
            continue
        power = "" if base == 1 else f"{paren(base)}^n"
        factor = (n_factor + power).rstrip("*") or ""
        mag = abs(coef)
        if factor:
            body = factor if mag == 1 else f"{mag}*{factor}"
        else:
            body = str(mag)
        parts.append((("-" if coef < 0 else "") + body) if not parts else (("- " if coef < 0 else "+ ") + body))
    return "a(n) = " + (" ".join(parts) if parts else "0"), None


def candidates(terms, start):
    n = len(terms)
    found = []
    d = terms[1] - terms[0]
    if all(b - a == d for a, b in zip(terms, terms[1:])):
        coefs = [terms[0] - d * start, d]
        found.append({"model": "arithmetic" if d else "constant", "determined_by": 2, "difference": fmt(d),
                      "closed_form": "a(n) = " + poly_string(coefs), "_next": lambda k, c=coefs: poly_value(c, k)})
    if all(t != 0 for t in terms):
        r = terms[1] / terms[0]
        if r != 1 and all(b == a * r for a, b in zip(terms, terms[1:])):
            const = terms[0] / r ** start
            power = f"{paren(r)}^n"
            closed = power if const == 1 else (f"-{power}" if const == -1 else f"{paren(const)} * {power}")
            found.append({"model": "geometric", "determined_by": 2, "ratio": fmt(r), "closed_form": "a(n) = " + closed,
                          "_next": lambda k, c=const, r=r: c * r ** k})
    for degree in range(2, n - 1):
        coefs = polynomial_fit(terms, start, degree)
        if coefs is not None:
            found.append({"model": "polynomial", "degree": degree, "determined_by": degree + 1,
                          "coefficients": [fmt(c) for c in coefs], "closed_form": "a(n) = " + poly_string(coefs),
                          "_next": lambda k, c=coefs: poly_value(c, k)})
            break
    for order in range(2, min(MAX_ORDER, (n - 1) // 2) + 1):
        coefs = recurrence_fit(terms, order)
        if coefs is not None:
            closed, note = order_two_closed_form(coefs, terms, start) if order == 2 else (None, None)
            model = {"model": "linear_recurrence", "order": order, "determined_by": 2 * order,
                     "coefficients": [fmt(c) for c in coefs], "recurrence": recurrence_string(coefs),
                     "initial_terms": [fmt(t) for t in terms[:order]], "characteristic_polynomial": characteristic(coefs),
                     "closed_form": closed, "_rec": coefs}
            if note:
                model["closed_form_note"] = note
            found.append(model)
            break
    return found


def extend(model, terms, start, count):
    if "_rec" in model:
        seq, coefs = terms[:], model["_rec"]
        for _ in range(count):
            seq.append(sum(c * seq[-j] for j, c in enumerate(coefs, 1)))
        return seq[len(terms):]
    return [model["_next"](start + len(terms) + i) for i in range(count)]


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    terms = read_terms(request.get("terms"))
    count = request.get("next", 5)
    if isinstance(count, bool) or not isinstance(count, int) or not 0 <= count <= MAX_NEXT:
        fail(f"next must be an integer from 0 to {MAX_NEXT}")
    start = request.get("start_index", 1)
    if isinstance(start, bool) or not isinstance(start, int) or abs(start) > 10 ** 6:
        fail("start_index must be an integer")
    found = candidates(terms, start)
    n = len(terms)
    out = {"terms_given": n, "start_index": start}
    if not found:
        out.update({"model": None, "next": [],
                    "note": f"no arithmetic, geometric, polynomial (degree up to {n - 2}) or linear recurrence (order up to {min(MAX_ORDER, (n - 1) // 2)}) fits with at least one term to spare; more terms might reveal one"})
        print(json.dumps(out))
        return
    order = {"constant": 0, "arithmetic": 0, "geometric": 1, "polynomial": 2, "linear_recurrence": 3}
    found.sort(key=lambda m: (m["determined_by"], order[m["model"]]))
    best = found[0]
    nexts = extend(best, terms, start, count)
    checks = n - best["determined_by"]
    public = lambda m: {k: v for k, v in m.items() if not k.startswith("_")}
    out.update(public(best))
    out["checked_on"] = checks
    agree = f"{checks} more term agrees" if checks == 1 else f"the other {checks} agree"
    out["fits"] = (f"fits all {n} given terms: the first {best['determined_by']} determine it and {agree}; "
                   "it is the simplest such model, not a proof that the sequence continues this way")
    out["next"] = [fmt(v) for v in nexts]
    others = []
    for m in found[1:]:
        other_next = extend(m, terms, start, count)
        entry = public(m)
        entry["checked_on"] = n - m["determined_by"]
        entry["agrees_on_next"] = other_next == nexts
        if not entry["agrees_on_next"]:
            entry["next"] = [fmt(v) for v in other_next]
        others.append(entry)
    out["also_fits"] = others
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
