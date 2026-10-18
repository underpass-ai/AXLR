# vote_stv

Counts a multi-winner single transferable vote election.

- **Quota**: Droop, `floor(valid ballots / (seats + 1)) + 1`, where valid ballots are those that
  rank at least one candidate.
- **Election**: a continuing candidate whose votes reach the quota is elected at once (several
  in one round, most votes first).
- **Surplus** (weighted inclusive Gregory method): the largest untransferred surplus goes
  first. Every ballot the elected candidate holds, including ones that reached it by earlier
  transfers, moves to its next continuing preference at its current value times
  `surplus / candidate's votes` (`transfer_value`); the candidate keeps exactly the quota.
- **Exclusion**: with no surplus waiting, the continuing candidate with the fewest votes is
  excluded and all its ballots move on at their current value.
- **Exhausted** ballots (no continuing preference left) are added up per round.
- When the continuing candidates are no more than the seats left, they are all elected
  (`elected_without_quota`).

Tie rule for exclusion (the same as `vote_irv`): among candidates tied for fewest votes,
exclude the one with the fewest first-preference votes; if still tied, the one with fewer votes
in the earliest later round where they differ; if the counts never separate them, use
`tie_order` (a drawing of lots, first listed loses first; by default the last candidate listed
loses). Each tie is reported with how it was resolved.

All values are exact fractions; tallies print as decimals rounded to six places, transfer
values as fractions.

Tests: the food election of Wikipedia's *Single transferable vote* article (23 guests, 3 seats,
quota 6): Pears elected, its surplus of 2 moves at 1/4 to Strawberry, Chocolate is excluded, the
three-way tie at 3 is broken by first preferences against Strawberry, Cake reaches 6 and
Chicken's exclusion gives Hamburger 7. The result, Pears, Cake and Hamburger, matches the
article; a second election with a 5/12 transfer value is checked by hand.

Limits: real statutes differ in detail (Scotland truncates transfer values to five decimals,
Ireland and Malta transfer only the last parcel, Meek's method recalculates keep values); this
tool implements the exact inclusive Gregory count and no other. Strict rankings only; exact
fractions can grow slowly on very large elections.
