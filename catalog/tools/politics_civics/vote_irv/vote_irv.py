#!/usr/bin/env python3
"""Instant-runoff voting (single-winner ranked choice) with every round reported.

Input (JSON on stdin):
  ballots     [{"ranking": ["A", "B", "C"], "count": 3}, ...] or bare rankings ["A", "B"];
              rankings are strict (no ties) and may stop early
  candidates  ["A", "B", ...]  optional; default every name on a ballot, in first-seen order
  tie_order   ["C", "B", ...]  optional; the order in which candidates lose a tie that the
              rules below cannot break (for example the result of a drawing of lots);
              default the reverse of the candidate order (the last listed loses first)
Rules:
  Each round every ballot counts for its highest-ranked continuing candidate; a ballot with
  none left is exhausted. A candidate with more than half of the continuing (non-exhausted)
  votes wins. Otherwise the candidate with the fewest votes is eliminated; a tie for fewest
  goes to the one with fewer first-preference votes, then fewer votes in each earlier round
  from the second onward, then tie_order, and every tie is reported.
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
            extra = set(ballot) - {"ranking", "count"}
            if extra:
                fail(f"ballots[{i}] has unknown fields {sorted(extra)}")
            ranking, count = ballot.get("ranking"), ballot.get("count", 1)
        else:
            fail(f"ballots[{i}] must be a list or an object with ranking")
        if not isinstance(ranking, list) or not isinstance(count, int) or isinstance(count, bool) or count < 0:
            fail(f"ballots[{i}] needs a ranking list and a non-negative integer count")
        for c in ranking:
            if isinstance(c, list):
                fail(f"ballots[{i}] ranks a tie; instant runoff needs strict rankings")
            if not isinstance(c, str) or not c:
                fail(f"ballots[{i}] ranks something that is not a candidate name")
            if c not in seen:
                seen.append(c)
        if len(ranking) != len(set(ranking)):
            fail(f"ballots[{i}] ranks a candidate twice")
        parsed.append((ranking, count))
    candidates = request.get("candidates", seen)
    if (not isinstance(candidates, list) or len(candidates) < 2 or len(set(candidates)) != len(candidates)
            or not all(isinstance(c, str) and c for c in candidates)):
        fail("candidates must be a list of at least two distinct names")
    unknown = [c for c in seen if c not in candidates]
    if unknown:
        fail(f"ballots rank candidates not in the candidate list: {unknown}")
    return candidates, parsed


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"ballots", "candidates", "tie_order"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    candidates, ballots = read_ballots(request)
    tie_order = request.get("tie_order", list(reversed(candidates)))
    if not isinstance(tie_order, list) or sorted(tie_order, key=str) != sorted(candidates, key=str):
        fail("tie_order must list every candidate exactly once")
    lot = {c: i for i, c in enumerate(tie_order)}
    total = sum(count for _, count in ballots)
    if total == 0:
        fail("no ballots to count")
    continuing = list(candidates)
    history = []
    rounds = []
    winner, tied_winners = None, None
    while True:
        tally = {c: 0 for c in continuing}
        exhausted = 0
        for ranking, count in ballots:
            top = next((c for c in ranking if c in tally), None)
            if top is None:
                exhausted += count
            else:
                tally[top] += count
        history.append(tally)
        active = total - exhausted
        record = {"round": len(rounds) + 1, "tally": dict(sorted(tally.items(), key=lambda kv: (-kv[1], candidates.index(kv[0])))),
                  "continuing_votes": active, "exhausted": exhausted}
        rounds.append(record)
        leader = max(tally.values())
        leaders = [c for c in continuing if tally[c] == leader]
        if active and 2 * leader > active:
            winner = leaders[0]
            record["result"] = f"{winner} has a majority of continuing votes"
            break
        if len(continuing) == 2 and len(leaders) == 2:
            tied_winners = leaders
            record["result"] = "the last two candidates are tied"
            break
        if len(continuing) == 1:
            winner = continuing[0]
            record["result"] = f"{winner} is the last candidate left"
            break
        low = min(tally.values())
        tied = [c for c in continuing if tally[c] == low]
        loser, how = tied[0], None
        if len(tied) > 1:
            pool = list(tied)
            for earlier in [history[0]] + history[1:-1]:
                fewest = min(earlier.get(c, 0) for c in pool)
                pool = [c for c in pool if earlier.get(c, 0) == fewest]
                if len(pool) == 1:
                    how = "first preferences" if earlier is history[0] else "earlier round"
                    break
            if len(pool) > 1:
                pool.sort(key=lambda c: lot[c])
                how = "tie_order (unresolved by the counts)"
            loser = pool[0]
            record["tie"] = {"between": tied, "eliminated": loser, "resolved_by": how}
        record["eliminated"] = loser
        continuing.remove(loser)
    result = {
        "candidates": candidates,
        "ballots": total,
        "winner": winner,
        "tied_winners": tied_winners,
        "rounds": rounds,
        "round_count": len(rounds),
        "elimination_order": [r["eliminated"] for r in rounds if "eliminated" in r],
    }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
