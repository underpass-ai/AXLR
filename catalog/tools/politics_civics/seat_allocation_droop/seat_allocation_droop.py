#!/usr/bin/env python3
"""Largest-remainder seat allocation with a Droop, Hagenbach-Bischoff, Imperiali or Hare quota.

Input (one JSON object on stdin):
  seats      integer > 0
  votes      object {party: votes >= 0}
  quota      "droop" (default) | "hagenbach_bischoff" | "imperiali" | "hare"
  threshold  number in [0, 100): parties under this share of valid votes get no seats (default 0)
  on_overallocation  "smallest_remainders" (default) | "error": what to do when the quota hands
             out more whole seats than exist (possible with Imperiali)
Output (JSON on stdout):
  the quota, each party's quotient, whole seats, remainder and total, the parties given a
  remainder seat, ties at the cut and any seats taken back after an overallocation.
"""
import json
import sys
from fractions import Fraction


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def fmt(value, places=6):
    """A Fraction as an integer string, or a decimal string rounded half up to `places`."""
    if value.denominator == 1:
        return str(value.numerator)
    scaled = value * 10 ** places
    sign = -1 if scaled < 0 else 1
    rounded = sign * ((abs(scaled.numerator) * 2 + abs(scaled.denominator)) // (2 * scaled.denominator))
    text = str(abs(rounded)).rjust(places + 1, "0")
    text = text[:-places] + "." + text[-places:]
    text = text.rstrip("0").rstrip(".")
    return ("-" if rounded < 0 else "") + text


def quota_for(kind, total, seats):
    if kind == "droop":
        return Fraction(total // (seats + 1) + 1)
    if kind == "hagenbach_bischoff":
        return Fraction(total, seats + 1)
    if kind == "imperiali":
        return Fraction(total, seats + 2)
    if kind == "hare":
        return Fraction(total, seats)
    fail("quota must be droop, hagenbach_bischoff, imperiali or hare")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    unknown = set(request) - {"seats", "votes", "quota", "threshold", "on_overallocation"}
    if unknown:
        fail(f"unknown fields: {sorted(unknown)}")
    seats = request.get("seats")
    votes = request.get("votes")
    kind = request.get("quota", "droop")
    threshold = request.get("threshold", 0)
    policy = request.get("on_overallocation", "smallest_remainders")
    if not isinstance(seats, int) or isinstance(seats, bool) or seats <= 0:
        fail("seats must be a positive integer")
    if not isinstance(votes, dict) or not votes or any(
            not isinstance(v, int) or isinstance(v, bool) or v < 0 for v in votes.values()):
        fail("votes must be a non-empty object of non-negative integers")
    if isinstance(threshold, bool) or not isinstance(threshold, (int, float, str)):
        fail("threshold must be a number of percent")
    try:
        threshold = Fraction(str(threshold))
    except (ValueError, ZeroDivisionError):
        fail("threshold must be a number of percent")
    if not 0 <= threshold < 100:
        fail("threshold must be between 0 and 100")
    if policy not in ("smallest_remainders", "error"):
        fail("on_overallocation must be smallest_remainders or error")
    valid = sum(votes.values())
    if valid == 0:
        fail("no valid votes")
    excluded = [p for p, v in votes.items() if Fraction(v * 100, valid) < threshold]
    eligible = {p: v for p, v in votes.items() if p not in excluded}
    counted = sum(eligible.values())
    if counted == 0:
        fail("no party passes the threshold")
    quota = quota_for(kind, counted, seats)
    quotients = {p: Fraction(v) / quota for p, v in eligible.items()}
    whole = {p: int(q) for p, q in quotients.items()}
    remainders = {p: quotients[p] - whole[p] for p in eligible}
    awarded = dict(whole)
    left = seats - sum(whole.values())
    ties, taken_back, remainder_seats = [], [], []
    if left >= 0:
        ranking = sorted(eligible, key=lambda p: (-remainders[p], -eligible[p], p))
        remainder_seats = ranking[:left]
        for p in remainder_seats:
            awarded[p] += 1
        if 0 < left < len(ranking) and remainders[ranking[left - 1]] == remainders[ranking[left]]:
            cut = remainders[ranking[left]]
            tied = [p for p in ranking if remainders[p] == cut]
            ties.append({"between": tied, "awarded_to": [p for p in tied if p in remainder_seats],
                         "rule": "larger vote count, then party name"})
    else:
        if policy == "error":
            fail(f"the {kind} quota hands out {sum(whole.values())} whole seats for {seats} seats")
        holders = sorted((p for p in eligible if whole[p] > 0), key=lambda p: (remainders[p], eligible[p], p))
        excess = -left
        while excess:
            for p in holders:
                if excess and awarded[p] > 0:
                    awarded[p] -= 1
                    taken_back.append(p)
                    excess -= 1
    for p in excluded:
        awarded[p] = 0
    table = [{"party": p, "votes": eligible[p], "quotient": fmt(quotients[p]), "whole_seats": whole[p],
              "remainder": fmt(remainders[p]), "seats": awarded[p]} for p in eligible]
    result = {
        "quota_type": kind,
        "quota": fmt(quota),
        "seats": seats,
        "valid_votes": valid,
        "votes_counted": counted,
        "allocation": dict(sorted(awarded.items(), key=lambda kv: (-kv[1], -votes[kv[0]], kv[0]))),
        "whole_seats_total": sum(whole.values()),
        "remainder_seats": remainder_seats,
        "parties": table,
        "excluded_by_threshold": excluded,
        "ties": ties,
        "overallocation": {"excess": len(taken_back), "taken_back_from": taken_back,
                           "rule": "one seat each from the parties with the smallest remainders"} if taken_back else None,
    }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
