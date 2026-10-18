#!/usr/bin/env python3
"""Apportion seats among districts (states, provinces, regions) by population.

Input (JSON on stdin):
  seats        integer > 0
  populations  object {district: population > 0}
  method       "huntington_hill" (default) | "webster" | "jefferson" | "adams" | "hamilton"
  min_seats    integer >= 0, seats every district gets first (default 0; huntington_hill and
               adams always give at least 1)
Output (JSON on stdout):
  seats per district, each district's standard quota, the order of the seats awarded after
  the minimums with their priority values (divisor methods) or remainders (hamilton), and ties.
"""
import json
import math
import sys
from fractions import Fraction


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def fmt(value, places=6):
    if value.denominator == 1:
        return str(value.numerator)
    scaled = value * 10 ** places
    rounded = (scaled.numerator * 2 + scaled.denominator) // (2 * scaled.denominator)
    text = str(rounded).rjust(places + 1, "0")
    return (text[:-places] + "." + text[-places:]).rstrip("0").rstrip(".")


def priority_key(method, population, seats):
    """A value that orders claims to the next seat exactly; for Huntington-Hill the square
    of the priority value p / sqrt(s (s + 1)), so comparisons stay exact."""
    if method == "jefferson":
        return Fraction(population, seats + 1)
    if method == "webster":
        return Fraction(2 * population, 2 * seats + 1)
    if method == "adams":
        return Fraction(population, seats)
    return Fraction(population * population, seats * (seats + 1))


def shown_priority(method, key):
    if method == "huntington_hill":
        return round(math.sqrt(key.numerator) / math.sqrt(key.denominator), 3)
    return fmt(key)


def divisor(method, seats, pops, start):
    awarded = dict(start)
    order = []
    left = seats - sum(awarded.values())
    for _ in range(left):
        keys = {d: priority_key(method, p, awarded[d]) for d, p in pops.items()}
        best = max(keys.values())
        winner = sorted((d for d in pops if keys[d] == best), key=lambda d: (-pops[d], d))[0]
        awarded[winner] += 1
        order.append({"district": winner, "seat_number": awarded[winner], "priority": shown_priority(method, best)})
    ties = []
    keys = {d: priority_key(method, p, awarded[d]) for d, p in pops.items()}
    if order:
        last = order[-1]["district"]
        last_key = priority_key(method, pops[last], awarded[last] - 1)
        rivals = [d for d in pops if keys[d] == last_key]
        if rivals:
            ties.append({"between": [last] + rivals, "awarded_to": last, "rule": "larger population, then name"})
    nxt = max(keys.values())
    return awarded, order, ties, shown_priority(method, nxt)


def hamilton(seats, pops, minimum):
    fixed = {}
    free = dict(pops)
    while True:
        left = seats - sum(fixed.values())
        total = sum(free.values())
        quotas = {d: Fraction(p * left, total) for d, p in free.items()}
        low = [d for d in free if int(quotas[d]) < minimum]
        if not low:
            break
        for d in low:
            fixed[d] = minimum
            del free[d]
        if not free:
            break
    awarded = dict(fixed)
    order, ties = [], []
    if free:
        left = seats - sum(fixed.values())
        whole = {d: int(quotas[d]) for d in free}
        remainders = {d: quotas[d] - whole[d] for d in free}
        awarded.update(whole)
        extra = left - sum(whole.values())
        ranking = sorted(free, key=lambda d: (-remainders[d], -pops[d], d))
        for d in ranking[:extra]:
            awarded[d] += 1
            order.append({"district": d, "seat_number": awarded[d], "remainder": fmt(remainders[d])})
        if 0 < extra < len(ranking) and remainders[ranking[extra - 1]] == remainders[ranking[extra]]:
            cut = remainders[ranking[extra]]
            ties.append({"between": [d for d in ranking if remainders[d] == cut],
                         "awarded_to": [d for d in ranking[:extra] if remainders[d] == cut],
                         "rule": "larger population, then name"})
    return awarded, order, ties, sorted(fixed)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"seats", "populations", "method", "min_seats"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    seats = request.get("seats")
    pops = request.get("populations")
    method = request.get("method", "huntington_hill")
    minimum = request.get("min_seats", 0)
    if not isinstance(seats, int) or isinstance(seats, bool) or seats <= 0:
        fail("seats must be a positive integer")
    if not isinstance(pops, dict) or not pops or any(
            not isinstance(p, int) or isinstance(p, bool) or p <= 0 for p in pops.values()):
        fail("populations must be a non-empty object of positive integers")
    if method not in ("huntington_hill", "webster", "jefferson", "adams", "hamilton"):
        fail("method must be huntington_hill, webster, jefferson, adams or hamilton")
    if not isinstance(minimum, int) or isinstance(minimum, bool) or minimum < 0:
        fail("min_seats must be a non-negative integer")
    effective = max(minimum, 1) if method in ("huntington_hill", "adams") else minimum
    if effective * len(pops) > seats:
        fail(f"{len(pops)} districts with at least {effective} seat(s) each need more than {seats} seats")
    total = sum(pops.values())
    standard = Fraction(total, seats)
    result = {"method": method, "seats": seats, "population": total, "standard_divisor": fmt(standard),
              "min_seats": effective}
    if method == "hamilton":
        awarded, order, ties, raised = hamilton(seats, pops, effective)
        result["raised_to_minimum"] = raised
    else:
        awarded, order, ties, nxt = divisor(method, seats, pops, {d: effective for d in pops})
        result["next_priority"] = nxt
    result["allocation"] = {d: awarded[d] for d in pops}
    result["districts"] = [{"district": d, "population": p, "quota": fmt(Fraction(p) / standard), "seats": awarded[d],
                            "people_per_seat": fmt(Fraction(p, awarded[d])) if awarded[d] else None}
                           for d, p in pops.items()]
    result["awards"] = order
    result["ties"] = ties
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
