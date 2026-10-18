#!/usr/bin/env python3
"""Partisan-fairness and compactness metrics for a districting plan.

Input (JSON on stdin):
  districts  [{"name": "1", "votes": {"A": 53, "B": 47},
               "polygon": [[x, y], ...]  or  "parts": [[[x, y], ...], ...]}, ...]
             votes: two-party votes per district (both parties in every district);
             polygon / parts: the district outline in PLANAR coordinates (project
             longitude/latitude first); either may be left out
  parties    ["A", "B"]  optional; the first is the reference party for signed figures
  winner_threshold  "half" (default; Stephanopoulos & McGhee: a winner's votes above half the
             district's votes are wasted) | "majority" (votes above floor(total / 2) + 1)
Output: wasted votes per district and party, the efficiency gap, its equal-turnout shortcut,
the mean-median difference, seats against votes, and Polsby-Popper and Reock scores.
"""
import json
import math
import random
import sys
from fractions import Fraction


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def fmt(value, places=6):
    if value.denominator == 1:
        return str(value.numerator)
    scaled = value * 10 ** places
    sign = -1 if scaled < 0 else 1
    rounded = sign * ((abs(scaled.numerator) * 2 + scaled.denominator) // (2 * scaled.denominator))
    text = str(abs(rounded)).rjust(places + 1, "0")
    text = (text[:-places] + "." + text[-places:]).rstrip("0").rstrip(".")
    return ("-" if rounded < 0 and text != "0" else "") + text


def median(values):
    ordered = sorted(values)
    n = len(ordered)
    mid = n // 2
    return ordered[mid] if n % 2 else (ordered[mid - 1] + ordered[mid]) / 2


def read_ring(ring, where):
    if not isinstance(ring, list):
        fail(f"{where} must be a list of [x, y] points")
    points = []
    for point in ring:
        if (not isinstance(point, list) or len(point) != 2
                or not all(isinstance(v, (int, float)) and not isinstance(v, bool) and math.isfinite(v) for v in point)):
            fail(f"{where} must be a list of [x, y] number pairs")
        points.append((float(point[0]), float(point[1])))
    if len(points) > 1 and points[0] == points[-1]:
        points.pop()
    if len(set(points)) < 3:
        fail(f"{where} needs at least three distinct points")
    return points


def ring_area(points):
    s = 0.0
    for (x1, y1), (x2, y2) in zip(points, points[1:] + points[:1]):
        s += x1 * y2 - x2 * y1
    return abs(s) / 2


def ring_perimeter(points):
    return sum(math.hypot(x2 - x1, y2 - y1) for (x1, y1), (x2, y2) in zip(points, points[1:] + points[:1]))


def circle_two(a, b):
    cx, cy = (a[0] + b[0]) / 2, (a[1] + b[1]) / 2
    return cx, cy, math.hypot(a[0] - cx, a[1] - cy)


def circle_three(a, b, c):
    d = 2 * (a[0] * (b[1] - c[1]) + b[0] * (c[1] - a[1]) + c[0] * (a[1] - b[1]))
    if abs(d) < 1e-15:
        pairs = [circle_two(a, b), circle_two(a, c), circle_two(b, c)]
        return max(pairs, key=lambda circle: circle[2])
    a2, b2, c2 = a[0] ** 2 + a[1] ** 2, b[0] ** 2 + b[1] ** 2, c[0] ** 2 + c[1] ** 2
    ux = (a2 * (b[1] - c[1]) + b2 * (c[1] - a[1]) + c2 * (a[1] - b[1])) / d
    uy = (a2 * (c[0] - b[0]) + b2 * (a[0] - c[0]) + c2 * (b[0] - a[0])) / d
    return ux, uy, math.hypot(a[0] - ux, a[1] - uy)


def inside(circle, p):
    return math.hypot(p[0] - circle[0], p[1] - circle[1]) <= circle[2] * (1 + 1e-12) + 1e-12


def enclosing_circle(points):
    """Smallest circle around the points (Welzl's algorithm, iterative, fixed shuffle)."""
    pts = sorted(set(points))
    random.Random(0).shuffle(pts)
    circle = (pts[0][0], pts[0][1], 0.0)
    for i, p in enumerate(pts):
        if inside(circle, p):
            continue
        circle = (p[0], p[1], 0.0)
        for j in range(i):
            q = pts[j]
            if inside(circle, q):
                continue
            circle = circle_two(p, q)
            for k in range(j):
                r = pts[k]
                if not inside(circle, r):
                    circle = circle_three(p, q, r)
    return circle


def compactness(rings):
    area = sum(ring_area(r) for r in rings)
    perimeter = sum(ring_perimeter(r) for r in rings)
    cx, cy, radius = enclosing_circle([p for r in rings for p in r])
    circle_area = math.pi * radius ** 2
    return {
        "area": round(area, 9),
        "perimeter": round(perimeter, 9),
        "polsby_popper": round(4 * math.pi * area / perimeter ** 2, 9) if perimeter else None,
        "reock": round(area / circle_area, 9) if circle_area else None,
        "enclosing_circle": {"center": [round(cx, 9) + 0.0, round(cy, 9) + 0.0], "radius": round(radius, 9)},
    }


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"districts", "parties", "winner_threshold"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    districts = request.get("districts")
    if not isinstance(districts, list) or not districts:
        fail("districts must be a non-empty list")
    rule = request.get("winner_threshold", "half")
    if rule not in ("half", "majority"):
        fail("winner_threshold must be half or majority")
    parties = request.get("parties")
    with_votes = [d for d in districts if isinstance(d, dict) and "votes" in d]
    if parties is None and with_votes:
        votes0 = with_votes[0]["votes"]
        parties = list(votes0) if isinstance(votes0, dict) else None
    if with_votes and (not isinstance(parties, list) or len(parties) != 2 or len(set(parties)) != 2
                       or not all(isinstance(p, str) for p in parties)):
        fail("parties must be two distinct names (two-party votes)")
    rows = []
    for i, d in enumerate(districts):
        if not isinstance(d, dict):
            fail(f"districts[{i}] must be an object")
        unknown = set(d) - {"name", "votes", "polygon", "parts"}
        if unknown:
            fail(f"districts[{i}] has unknown fields {sorted(unknown)}")
        name = d.get("name", str(i + 1))
        if not isinstance(name, str):
            fail(f"districts[{i}].name must be a string")
        row = {"name": name}
        if "votes" in d:
            votes = d["votes"]
            if (not isinstance(votes, dict) or set(votes) != set(parties)
                    or any(not isinstance(v, int) or isinstance(v, bool) or v < 0 for v in votes.values())):
                fail(f"districts[{i}].votes must give non-negative integer votes for exactly {parties}")
            total = votes[parties[0]] + votes[parties[1]]
            if total == 0:
                fail(f"districts[{i}] has no votes")
            a, b = votes[parties[0]], votes[parties[1]]
            needed = Fraction(total, 2) if rule == "half" else Fraction(total // 2 + 1)
            if a == b:
                winner, wasted = None, {parties[0]: Fraction(a), parties[1]: Fraction(b)}
            elif a > b:
                winner, wasted = parties[0], {parties[0]: a - needed, parties[1]: Fraction(b)}
            else:
                winner, wasted = parties[1], {parties[0]: Fraction(a), parties[1]: b - needed}
            row.update({"votes": {p: votes[p] for p in parties}, "total": total, "winner": winner,
                        "_wasted": wasted, "_share": Fraction(a, total)})
        rings = None
        if "polygon" in d and "parts" in d:
            fail(f"districts[{i}] gives both polygon and parts")
        if "polygon" in d:
            rings = [read_ring(d["polygon"], f"districts[{i}].polygon")]
        elif "parts" in d:
            if not isinstance(d["parts"], list) or not d["parts"]:
                fail(f"districts[{i}].parts must be a non-empty list of rings")
            rings = [read_ring(r, f"districts[{i}].parts[{k}]") for k, r in enumerate(d["parts"])]
        if rings:
            if sum(ring_area(r) for r in rings) == 0:
                fail(f"districts[{i}] outline has zero area")
            row["compactness"] = compactness(rings)
        rows.append(row)
    result = {"parties": parties, "winner_threshold": rule}
    voted = [r for r in rows if "votes" in r]
    if voted:
        total = sum(r["total"] for r in voted)
        party_votes = {p: sum(r["votes"][p] for r in voted) for p in parties}
        wasted = {p: sum((r["_wasted"][p] for r in voted), Fraction(0)) for p in parties}
        seats = {p: sum(1 for r in voted if r["winner"] == p) for p in parties}
        signed = (wasted[parties[1]] - wasted[parties[0]]) / total
        shares = [r["_share"] for r in voted]
        mean = sum(shares, Fraction(0)) / len(shares)
        med = median(shares)
        vote_share = {p: Fraction(party_votes[p], total) for p in parties}
        seat_share = {p: Fraction(seats[p], len(voted)) for p in parties}
        a = parties[0]
        result.update({
            "districts_counted": len(voted),
            "votes": party_votes,
            "vote_share": {p: fmt(v) for p, v in vote_share.items()},
            "seats": seats,
            "tied_districts": [r["name"] for r in voted if r["winner"] is None],
            "seat_share": {p: fmt(v) for p, v in seat_share.items()},
            "seat_bonus": {p: fmt(seat_share[p] - vote_share[p]) for p in parties},
            "wasted_votes": {p: fmt(w) for p, w in wasted.items()},
            "efficiency_gap": fmt(abs(signed)),
            "efficiency_gap_favours": None if signed == 0 else (parties[0] if signed > 0 else parties[1]),
            "efficiency_gap_signed": fmt(signed),
            "efficiency_gap_equal_turnout": fmt((seat_share[a] - Fraction(1, 2)) - 2 * (vote_share[a] - Fraction(1, 2))),
            "mean_median": {"party": a, "mean_share": fmt(mean), "median_share": fmt(med),
                            "median_minus_mean": fmt(med - mean)},
        })
    result["districts"] = []
    for r in rows:
        out = {"name": r["name"]}
        if "votes" in r:
            out.update({"votes": r["votes"], "winner": r["winner"], "share": {parties[0]: fmt(r["_share"])},
                        "wasted": {p: fmt(w) for p, w in r["_wasted"].items()}})
        if "compactness" in r:
            out["compactness"] = r["compactness"]
        result["districts"].append(out)
    if any("compactness" in r for r in rows):
        result["coordinates"] = "planar: distances and areas are taken as given; project longitude/latitude first"
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
