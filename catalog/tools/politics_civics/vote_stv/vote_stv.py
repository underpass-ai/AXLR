#!/usr/bin/env python3
"""Single transferable vote with the Droop quota and Gregory fractional surplus transfers.

Input (JSON on stdin):
  seats       integer > 0
  ballots     [{"ranking": ["A", "B", "C"], "count": 3}, ...] or bare rankings; strict
              rankings that may stop early
  candidates  ["A", "B", ...]  optional; default every name on a ballot, in first-seen order
  tie_order   ["C", "B", ...]  optional; who loses a tie the counts cannot break, first listed
              first (a drawing of lots); default the reverse candidate order
Rules (weighted inclusive Gregory method, exact fractions):
  quota = floor(valid ballots / (seats + 1)) + 1. A continuing candidate whose votes reach the
  quota is elected. The largest untransferred surplus is transferred first: every ballot the
  elected candidate holds moves to its next continuing preference at its current value times
  surplus / candidate's votes. With no surplus to transfer, the continuing candidate with the
  fewest votes is excluded and its ballots move on at their current value. A tie for fewest
  goes to the one with fewer first-preference votes, then fewer votes in each earlier round
  from the second onward, then tie_order. When the continuing candidates are no more than the
  seats left, they are all elected.
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
        for c in ranking:
            if isinstance(c, list):
                fail(f"ballots[{i}] ranks a tie; STV needs strict rankings")
            if not isinstance(c, str) or not c:
                fail(f"ballots[{i}] ranks something that is not a candidate name")
            if c not in seen:
                seen.append(c)
        if len(ranking) != len(set(ranking)):
            fail(f"ballots[{i}] ranks a candidate twice")
        if ranking and count:
            parsed.append((ranking, count))
    candidates = request.get("candidates", seen)
    if (not isinstance(candidates, list) or len(candidates) < 2 or len(set(candidates)) != len(candidates)
            or not all(isinstance(c, str) and c for c in candidates)):
        fail("candidates must be a list of at least two distinct names")
    unknown = [c for c in seen if c not in candidates]
    if unknown:
        fail(f"ballots rank candidates not in the candidate list: {unknown}")
    return candidates, parsed


class Count:
    def __init__(self, candidates, ballots, seats, lot):
        self.candidates = candidates
        self.seats = seats
        self.lot = lot
        self.continuing = list(candidates)
        self.elected = []
        self.excluded = []
        self.piles = {c: [] for c in candidates}
        self.exhausted = Fraction(0)
        self.history = []
        self.rounds = []
        self.pending = []
        valid = sum(count for _, count in ballots)
        self.valid = valid
        self.quota = Fraction(valid // (seats + 1) + 1)
        for ranking, count in ballots:
            self.place(ranking, 0, Fraction(count))

    def place(self, ranking, start, value):
        for position in range(start, len(ranking)):
            c = ranking[position]
            if c in self.continuing:
                self.piles[c].append((ranking, position, value))
                return
        self.exhausted += value

    def votes(self, c):
        return sum((value for _, _, value in self.piles[c]), Fraction(0))

    def move(self, c, factor):
        papers, self.piles[c] = self.piles[c], []
        for ranking, position, value in papers:
            self.place(ranking, position + 1, value * factor)

    def record(self, action, **extra):
        tally = {c: self.votes(c) for c in self.candidates if c not in self.excluded}
        self.history.append({c: self.votes(c) for c in self.candidates})
        entry = {"round": len(self.rounds) + 1, "action": action}
        entry.update(extra)
        entry["tally"] = {c: fmt(v) for c, v in sorted(tally.items(), key=lambda kv: (-kv[1], self.candidates.index(kv[0])))}
        entry["exhausted"] = fmt(self.exhausted)
        self.rounds.append(entry)
        return entry

    def elect_reached(self, entry):
        reached = [c for c in self.continuing if self.votes(c) >= self.quota]
        reached.sort(key=lambda c: (-self.votes(c), self.candidates.index(c)))
        for c in reached:
            if len(self.elected) >= self.seats:
                break
            self.continuing.remove(c)
            self.elected.append(c)
            if self.votes(c) > self.quota:
                self.pending.append(c)
        if reached:
            entry["elected"] = reached

    def lowest(self):
        low = min(self.votes(c) for c in self.continuing)
        tied = [c for c in self.continuing if self.votes(c) == low]
        if len(tied) == 1:
            return tied[0], None
        pool = list(tied)
        for index, earlier in enumerate(self.history[:-1]):
            fewest = min(earlier[c] for c in pool)
            pool = [c for c in pool if earlier[c] == fewest]
            if len(pool) == 1:
                how = "first preferences" if index == 0 else f"round {index + 1} votes"
                return pool[0], {"between": tied, "excluded": pool[0], "resolved_by": how}
        pool.sort(key=lambda c: self.lot[c])
        return pool[0], {"between": tied, "excluded": pool[0], "resolved_by": "tie_order (unresolved by the counts)"}

    def run(self):
        entry = self.record("first preferences")
        self.elect_reached(entry)
        while len(self.elected) < self.seats:
            left = self.seats - len(self.elected)
            if len(self.continuing) <= left:
                entry = self.rounds[-1]
                entry.setdefault("elected_without_quota", [])
                for c in sorted(self.continuing, key=lambda c: (-self.votes(c), self.candidates.index(c))):
                    entry["elected_without_quota"].append(c)
                    self.elected.append(c)
                self.continuing = []
                break
            if self.pending:
                self.pending.sort(key=lambda c: -self.votes(c))
                c = self.pending.pop(0)
                total = self.votes(c)
                surplus = total - self.quota
                factor = surplus / total
                self.move(c, factor)
                self.piles[c] = [(None, 0, self.quota)]
                entry = self.record(f"transfer surplus of {c}", surplus=fmt(surplus), transfer_value=str(factor))
            else:
                c, tie = self.lowest()
                self.continuing.remove(c)
                self.excluded.append(c)
                self.move(c, Fraction(1))
                entry = self.record(f"exclude {c}")
                if tie:
                    entry["tie"] = tie
            self.elect_reached(entry)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"seats", "ballots", "candidates", "tie_order"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    seats = request.get("seats")
    if not isinstance(seats, int) or isinstance(seats, bool) or seats <= 0:
        fail("seats must be a positive integer")
    candidates, ballots = read_ballots(request)
    if seats >= len(candidates):
        fail("seats must be fewer than the candidates")
    if not ballots:
        fail("no ballot ranks anyone")
    tie_order = request.get("tie_order", list(reversed(candidates)))
    if not isinstance(tie_order, list) or sorted(tie_order, key=str) != sorted(candidates, key=str):
        fail("tie_order must list every candidate exactly once")
    count = Count(candidates, ballots, seats, {c: i for i, c in enumerate(tie_order)})
    count.run()
    print(json.dumps({
        "seats": seats,
        "valid_ballots": count.valid,
        "quota": fmt(count.quota),
        "elected": count.elected,
        "excluded": count.excluded,
        "rounds": count.rounds,
        "round_count": len(count.rounds),
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
