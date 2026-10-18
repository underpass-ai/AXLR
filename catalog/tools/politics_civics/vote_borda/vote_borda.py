#!/usr/bin/env python3
"""Borda count and Dowdall scores over ranked ballots, exact.

Input (JSON on stdin):
  ballots     [{"ranking": ["A", ["B", "C"], "D"], "count": 3}, ...] or bare rankings;
              an inner list is a tie; candidates a ballot leaves out are unranked
  candidates  ["A", "B", ...]  optional; default every name on a ballot, in first-seen order
  variant     "borda" (n-1 ... 0, default) | "borda_n" (n ... 1) | "dowdall" (1, 1/2, 1/3 ...)
  partial     how tied and unranked candidates score:
              "lowest"   (default) each candidate in a tie gets the points of the lowest
                         position the tie spans; unranked candidates tie at the bottom
              "average"  each gets the mean of the points of the positions the tie spans
              "modified" unranked candidates get 0 and the k ranked candidates are scored
                         k ... 1 (modified Borda count; ties among them averaged); Borda only
              Under Dowdall unranked candidates always score 0.
"""
import json
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


def read_ballots(request):
    ballots = request.get("ballots")
    if not isinstance(ballots, list) or not ballots:
        fail("ballots must be a non-empty list")
    parsed, seen = [], []
    for i, ballot in enumerate(ballots):
        if isinstance(ballot, list):
            ranking, count = ballot, 1
        elif isinstance(ballot, dict):
            extra = set(ballot) - {"ranking", "count"}
            if extra:
                fail(f"ballots[{i}] has unknown fields {sorted(extra)}")
            ranking, count = ballot.get("ranking"), ballot.get("count", 1)
        else:
            fail(f"ballots[{i}] must be a list or an object with ranking")
        if not isinstance(ranking, list) or not isinstance(count, int) or isinstance(count, bool) or count < 0:
            fail(f"ballots[{i}] needs a ranking list and a non-negative integer count")
        groups = []
        for item in ranking:
            group = item if isinstance(item, list) else [item]
            if not group or not all(isinstance(c, str) and c for c in group):
                fail(f"ballots[{i}] ranks something that is not a candidate name")
            groups.append(group)
            for c in group:
                if c not in seen:
                    seen.append(c)
        flat = [c for g in groups for c in g]
        if len(flat) != len(set(flat)):
            fail(f"ballots[{i}] ranks a candidate twice")
        parsed.append((groups, count))
    candidates = request.get("candidates", seen)
    if (not isinstance(candidates, list) or len(candidates) < 2 or len(set(candidates)) != len(candidates)
            or not all(isinstance(c, str) and c for c in candidates)):
        fail("candidates must be a list of at least two distinct names")
    unknown = [c for c in seen if c not in candidates]
    if unknown:
        fail(f"ballots rank candidates not in the candidate list: {unknown}")
    return candidates, parsed


def ballot_points(groups, candidates, variant, partial):
    n = len(candidates)
    ranked = [c for g in groups for c in g]
    unranked = [c for c in candidates if c not in ranked]
    size = len(ranked) if partial == "modified" else n

    def points(i):
        if variant == "dowdall":
            return Fraction(1, i + 1)
        if partial == "modified":
            return Fraction(size - i)
        return Fraction(n - 1 - i) if variant == "borda" else Fraction(n - i)

    scores = {}
    start = 0
    tiers = [(g, False) for g in groups] + ([(unranked, True)] if unranked else [])
    for group, is_unranked in tiers:
        span = [points(i) for i in range(start, start + len(group))]
        if is_unranked and (variant == "dowdall" or partial == "modified"):
            value = Fraction(0)
        elif partial == "lowest":
            value = span[-1]
        else:
            value = sum(span) / len(span)
        for c in group:
            scores[c] = value
        start += len(group)
    return scores


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"ballots", "candidates", "variant", "partial"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    variant = request.get("variant", "borda")
    partial = request.get("partial", "lowest")
    if variant not in ("borda", "borda_n", "dowdall"):
        fail("variant must be borda, borda_n or dowdall")
    if partial not in ("lowest", "average", "modified"):
        fail("partial must be lowest, average or modified")
    if partial == "modified" and variant == "dowdall":
        fail("partial modified applies to the Borda variants only")
    candidates, ballots = read_ballots(request)
    totals = {c: Fraction(0) for c in candidates}
    for groups, count in ballots:
        for c, value in ballot_points(groups, candidates, variant, partial).items():
            totals[c] += value * count
    order = sorted(candidates, key=lambda c: (-totals[c], candidates.index(c)))
    ranking, i = [], 0
    while i < len(order):
        tier = [c for c in order if totals[c] == totals[order[i]]]
        ranking.append(tier if len(tier) > 1 else tier[0])
        i += len(tier)
    top = totals[order[0]]
    print(json.dumps({
        "variant": variant,
        "partial": partial,
        "candidates": candidates,
        "voters": sum(count for _, count in ballots),
        "scores": {c: fmt(totals[c]) for c in order},
        "exact": {c: str(totals[c]) for c in order},
        "ranking": ranking,
        "winners": [c for c in order if totals[c] == top],
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
