#!/usr/bin/env python3
"""Condorcet methods over ranked ballots: the pairwise matrix, the Condorcet winner and
loser, the Smith set, Copeland scores and the Schulze ranking with its strongest paths.

Input (JSON on stdin):
  ballots     [{"ranking": ["A", ["B", "C"], "D"], "count": 3}, ...] or bare rankings
              ["A", "B", "C"]; an inner list is a tie; unranked candidates tie below
              every ranked one
  candidates  ["A", "B", ...]  optional; default every name on a ballot, in first-seen order
"""
import json
import sys


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def read_ballots(request):
    ballots = request.get("ballots")
    if not isinstance(ballots, list) or not ballots:
        fail("ballots must be a non-empty list")
    parsed, seen = [], []
    for i, ballot in enumerate(ballots):
        if isinstance(ballot, list):
            ranking, count = ballot, 1
        elif isinstance(ballot, dict):
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
    if not isinstance(candidates, list) or len(candidates) < 2 or len(set(candidates)) != len(candidates):
        fail("candidates must be a list of at least two distinct names")
    unknown = [c for c in seen if c not in candidates]
    if unknown:
        fail(f"ballots rank candidates not in the candidate list: {unknown}")
    return candidates, parsed


def pairwise(candidates, ballots):
    d = {a: {b: 0 for b in candidates if b != a} for a in candidates}
    for groups, count in ballots:
        rank = {}
        for position, group in enumerate(groups):
            for c in group:
                rank[c] = position
        bottom = len(groups)
        for a in candidates:
            for b in candidates:
                if a != b and rank.get(a, bottom) < rank.get(b, bottom):
                    d[a][b] += count
    return d


def schulze(candidates, d):
    p = {a: {b: (d[a][b] if d[a][b] > d[b][a] else 0) for b in candidates if b != a} for a in candidates}
    for i in candidates:
        for j in candidates:
            if i == j:
                continue
            for k in candidates:
                if k not in (i, j):
                    p[j][k] = max(p[j][k], min(p[j][i], p[i][k]))
    wins = {a: sum(1 for b in candidates if b != a and p[a][b] > p[b][a]) for a in candidates}
    ranking, remaining = [], list(candidates)
    while remaining:
        best = max(wins[c] for c in remaining)
        tier = [c for c in remaining if wins[c] == best]
        ranking.append(tier if len(tier) > 1 else tier[0])
        remaining = [c for c in remaining if c not in tier]
    winners = [a for a in candidates if all(p[a][b] >= p[b][a] for b in candidates if b != a)]
    return p, ranking, winners


def smith_set(candidates, d):
    beats_or_ties = {a: {b for b in candidates if b != a and d[a][b] >= d[b][a]} for a in candidates}
    copeland_order = sorted(candidates, key=lambda a: -len(beats_or_ties[a]))
    for size in range(1, len(candidates) + 1):
        group = set(copeland_order[:size])
        if all(d[a][b] > d[b][a] for a in group for b in candidates if b not in group):
            return [c for c in candidates if c in group]
    return list(candidates)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    candidates, ballots = read_ballots(request)
    d = pairwise(candidates, ballots)
    others = lambda a: [b for b in candidates if b != a]
    winner = next((a for a in candidates if all(d[a][b] > d[b][a] for b in others(a))), None)
    loser = next((a for a in candidates if all(d[a][b] < d[b][a] for b in others(a))), None)
    copeland = {a: sum(1 if d[a][b] > d[b][a] else 0.5 if d[a][b] == d[b][a] else 0 for b in others(a)) for a in candidates}
    top = max(copeland.values())
    paths, ranking, winners = schulze(candidates, d)
    print(json.dumps({
        "candidates": candidates,
        "voters": sum(count for _, count in ballots),
        "pairwise": d,
        "condorcet_winner": winner,
        "condorcet_loser": loser,
        "smith_set": smith_set(candidates, d),
        "copeland": {"scores": copeland, "winners": [a for a in candidates if copeland[a] == top]},
        "schulze": {"winners": winners, "ranking": ranking, "strongest_paths": paths},
    }))


if __name__ == "__main__":
    main()
