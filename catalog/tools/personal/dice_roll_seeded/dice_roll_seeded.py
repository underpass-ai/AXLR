#!/usr/bin/env python3
"""Dice notation: roll reproducibly with a seed and compute the exact probability distribution of
the total, its mean and variance, with fractions.

Notation (case-insensitive, spaces ignored): terms joined by + or -
  NdM      N dice with M faces (N defaults to 1; d% is d100; dF is a Fudge die -1, 0, +1)
  K        a constant
  modifiers after a dice term, in any order:
    khN / kN   keep the N highest      klN   keep the N lowest
    dhN        drop the N highest      dlN   drop the N lowest
    !          exploding: a die showing its maximum is rolled again and added (repeatedly)
  examples: "2d6+3", "4d6dl1", "4d6kh3", "2d20kh1" (advantage), "3d6!", "d%", "1d8+1d6-2", "4dF"

Input (JSON on stdin):
  notation   the dice expression
  seed       integer or string: roll with random.Random(seed) (omit for no roll)
  rolls      how many times to roll (default 1, at most 1000)
  target     report P(total >= target) and P(total <= target)
  explode_depth  explosions per die kept in the exact distribution (default 8; the rest is
                 reported as truncated probability; mean and variance stay exact)
  distribution   false: skip the table of totals
"""
import itertools
import json
import math
import random
import re
import sys
from decimal import Decimal, getcontext
from fractions import Fraction

getcontext().prec = 30
MAX_DICE, MAX_FACES, MAX_TERMS = 100, 1000, 20
TERM = re.compile(r"(\d*)d(\d+|%|f)((?:kh\d+|kl\d+|k\d+|dh\d+|dl\d+|!)*)$")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def dec(x, places=10):
    q = Decimal(x.numerator) / Decimal(x.denominator)
    return format(q.quantize(Decimal(1).scaleb(-places)), "f")


def frac(x):
    return str(x.numerator) if x.denominator == 1 else f"{x.numerator}/{x.denominator}"


def parse(notation):
    text = notation.replace(" ", "").lower()
    if not text:
        fail("notation is empty")
    parts = re.findall(r"[+-]?[^+-]+", text)
    if "".join(parts) != text or not parts:
        fail(f"cannot read {notation!r}")
    terms = []
    for raw in parts:
        sign = -1 if raw.startswith("-") else 1
        body = raw.lstrip("+-")
        if not body:
            fail(f"cannot read {notation!r}: an operator without a term")
        if body.isdigit():
            terms.append({"kind": "const", "sign": sign, "value": int(body), "text": raw})
            continue
        m = TERM.match(body)
        if not m:
            fail(f"cannot read the term {body!r} (use NdM with kh/kl/dh/dl/! modifiers, or a number)")
        n = int(m.group(1)) if m.group(1) else 1
        faces_text = m.group(2)
        fudge = faces_text == "f"
        faces = 100 if faces_text == "%" else 3 if fudge else int(faces_text)
        if not 1 <= n <= MAX_DICE:
            fail(f"{body}: 1 to {MAX_DICE} dice")
        if not 1 <= faces <= MAX_FACES:
            fail(f"{body}: 1 to {MAX_FACES} faces")
        keep, explode = None, False
        for mod, num in re.findall(r"(kh|kl|k|dh|dl|!)(\d*)", m.group(3)):
            if mod == "!":
                if explode:
                    fail(f"{body}: '!' given twice")
                explode = True
                continue
            count = int(num)
            if keep is not None:
                fail(f"{body}: only one keep or drop modifier")
            if mod in ("kh", "k"):
                keep = ("high", count)
            elif mod == "kl":
                keep = ("low", count)
            elif mod == "dh":
                keep = ("low", n - count)
            else:
                keep = ("high", n - count)
            if not 0 < keep[1] <= n:
                fail(f"{body}: keep or drop count must leave between 1 and {n} dice")
        if explode and (fudge or faces < 2):
            fail(f"{body}: only dice with 2 or more numbered faces can explode")
        values = [-1, 0, 1] if fudge else list(range(1, faces + 1))
        terms.append({"kind": "dice", "sign": sign, "n": n, "faces": faces, "values": values, "fudge": fudge,
                      "keep": keep, "explode": explode, "text": raw})
    if len(terms) > MAX_TERMS:
        fail(f"at most {MAX_TERMS} terms")
    return terms


def convolve(a, b):
    out = {}
    for x, p in a.items():
        for y, q in b.items():
            out[x + y] = out.get(x + y, 0) + p * q
    return out


def die_dist(term, depth):
    values = term["values"]
    p = Fraction(1, len(values))
    if not term["explode"]:
        return {v: p for v in values}, Fraction(0)
    m = term["faces"]
    dist = {}
    for k in range(depth + 1):
        for r in range(1, m):
            dist[k * m + r] = p ** (k + 1)
    return dist, p ** (depth + 1)


def term_dist(term, depth):
    """(distribution {total: probability}, truncated mass) of one term, or (None, reason)."""
    if term["kind"] == "const":
        return {term["sign"] * term["value"]: Fraction(1)}, Fraction(0)
    one, lost = die_dist(term, depth)
    n = term["n"]
    if term["keep"] is None:
        dist = {0: Fraction(1)}
        for _ in range(n):
            dist = convolve(dist, one)
            if len(dist) > 200000:
                return None, "too many distinct totals"
        mass = 1 - (1 - lost) ** n
    else:
        if term["explode"]:
            return None, "keep or drop combined with exploding dice is rolled but not tabulated"
        values = term["values"]
        how, count = term["keep"]
        multisets = math.comb(len(values) + n - 1, n)
        if multisets > 300000:
            return None, f"{multisets} combinations to enumerate (limit 300000)"
        dist = {}
        total_ways = len(values) ** n
        for combo in itertools.combinations_with_replacement(values, n):
            ways = math.factorial(n)
            for v in set(combo):
                ways //= math.factorial(combo.count(v))
            kept = combo[-count:] if how == "high" else combo[:count]
            s = sum(kept)
            dist[s] = dist.get(s, 0) + Fraction(ways, total_ways)
        mass = Fraction(0)
    if term["sign"] < 0:
        dist = {-k: v for k, v in dist.items()}
    return dist, mass


def exact_moments(term):
    """Exact (mean, variance) of a term without keep/drop, including infinite explosions."""
    sign = term["sign"]
    if term["kind"] == "const":
        return Fraction(sign * term["value"]), Fraction(0)
    values = term["values"]
    if term["explode"]:
        m = term["faces"]
        mean_k = Fraction(1, m - 1)
        var_k = Fraction(m, (m - 1) ** 2)
        mean_r = Fraction(m, 2)
        var_r = Fraction((m - 1) ** 2 - 1, 12)
        mean1, var1 = m * mean_k + mean_r, m * m * var_k + var_r
    else:
        k = len(values)
        mean1 = Fraction(sum(values), k)
        var1 = Fraction(sum(v * v for v in values), k) - mean1 ** 2
    return sign * term["n"] * mean1, term["n"] * var1


def roll(term, rng):
    if term["kind"] == "const":
        return {"term": term["text"], "total": term["sign"] * term["value"]}
    dice = []
    for _ in range(term["n"]):
        if term["fudge"]:
            dice.append({"rolls": [rng.randint(1, 3) - 2]})
            continue
        rolls = [rng.randint(1, term["faces"])]
        while term["explode"] and rolls[-1] == term["faces"] and len(rolls) < 100:
            rolls.append(rng.randint(1, term["faces"]))
        dice.append({"rolls": rolls})
    values = [sum(d["rolls"]) for d in dice]
    kept = set(range(len(dice)))
    if term["keep"]:
        how, count = term["keep"]
        order = sorted(range(len(dice)), key=lambda i: (values[i], i))
        kept = set(order[-count:] if how == "high" else order[:count])
    total = sum(values[i] for i in kept)
    out = {"term": term["text"], "dice": [d["rolls"][0] if len(d["rolls"]) == 1 else d["rolls"] for d in dice],
           "total": term["sign"] * total}
    if term["keep"]:
        out["kept"] = [values[i] for i in sorted(kept)]
        out["dropped"] = [values[i] for i in range(len(dice)) if i not in kept]
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"notation", "seed", "rolls", "target", "explode_depth", "distribution"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    notation = request.get("notation")
    if not isinstance(notation, str) or len(notation) > 200:
        fail("notation must be a string of at most 200 characters")
    terms = parse(notation)
    depth = request.get("explode_depth", 8)
    if isinstance(depth, bool) or not isinstance(depth, int) or not 0 <= depth <= 50:
        fail("explode_depth must be an integer from 0 to 50")
    want_table = request.get("distribution", True)
    if not isinstance(want_table, bool):
        fail("distribution must be a boolean")
    result = {"notation": notation, "terms": [t["text"] for t in terms]}

    if "seed" in request:
        seed = request["seed"]
        if isinstance(seed, bool) or not isinstance(seed, (int, str)):
            fail("seed must be an integer or a string")
        n_rolls = request.get("rolls", 1)
        if isinstance(n_rolls, bool) or not isinstance(n_rolls, int) or not 1 <= n_rolls <= 1000:
            fail("rolls must be an integer from 1 to 1000")
        rng = random.Random(seed)
        outcomes = []
        for _ in range(n_rolls):
            parts = [roll(t, rng) for t in terms]
            outcomes.append({"total": sum(p["total"] for p in parts), "parts": parts})
        result["seed"] = seed
        result["generator"] = "Python random.Random(seed) (Mersenne Twister), randint per die in term order"
        if n_rolls == 1:
            result["roll"] = outcomes[0]
        else:
            result["rolls"] = outcomes
            result["totals"] = [o["total"] for o in outcomes]

    dist = {0: Fraction(1)}
    truncated = Fraction(0)
    reason = None
    for t in terms:
        d, extra_mass = term_dist(t, depth)
        if d is None:
            reason = f"{t['text']}: {extra_mass}"
            dist = None
            break
        dist = convolve(dist, d)
        truncated = 1 - (1 - truncated) * (1 - extra_mass)
        if len(dist) > 200000:
            reason = "too many distinct totals"
            dist = None
            break
    if dist is not None:
        support = sorted(dist)
        result["min"], result["max"] = support[0], support[-1]
        if not truncated:
            mean = sum(v * p for v, p in dist.items())
            var = sum(v * v * p for v, p in dist.items()) - mean ** 2
        else:
            mean = sum(exact_moments(t)[0] for t in terms)
            var = sum(exact_moments(t)[1] for t in terms)
            result["max"] = None
            result["max_note"] = "unbounded (exploding dice); the table stops at explode_depth"
            result["truncated_probability"] = {"exact": frac(truncated), "decimal": dec(truncated, 12)}
        result["mean"] = {"exact": frac(mean), "decimal": dec(mean)}
        result["variance"] = {"exact": frac(var), "decimal": dec(var)}
        result["std_dev"] = format(math.sqrt(var), ".10f")
        mode_p = max(dist.values())
        modes = [v for v in support if dist[v] == mode_p]
        if len(modes) <= 20:
            result["mode"] = modes
        else:
            result["mode_range"] = [modes[0], modes[-1]]
            result["mode_count"] = len(modes)
        if want_table:
            if len(support) <= 5000:
                result["distribution"] = [{"total": v, "p": frac(dist[v]), "p_decimal": dec(dist[v])} for v in support]
            else:
                result["distribution_note"] = f"{len(support)} totals: table omitted"
        if "target" in request:
            target = request["target"]
            if isinstance(target, bool) or not isinstance(target, int):
                fail("target must be an integer")
            ge = sum(p for v, p in dist.items() if v >= target)
            le = sum(p for v, p in dist.items() if v <= target)
            result["target"] = {"value": target, "p_at_least": frac(ge), "p_at_least_decimal": dec(ge),
                                "p_at_most": frac(le), "p_at_most_decimal": dec(le)}
            if truncated:
                result["target"]["note"] = "p_at_least excludes the truncated tail (add up to truncated_probability)"
    else:
        result["distribution"] = None
        result["distribution_note"] = reason
        if all(t["kind"] == "const" or t["keep"] is None for t in terms):
            mean = sum(exact_moments(t)[0] for t in terms)
            var = sum(exact_moments(t)[1] for t in terms)
            result["mean"] = {"exact": frac(mean), "decimal": dec(mean)}
            result["variance"] = {"exact": frac(var), "decimal": dec(var)}
    print(json.dumps(result))


if __name__ == "__main__":
    main()
