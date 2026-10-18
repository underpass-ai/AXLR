# vote_irv

Counts a single-winner instant-runoff election (ranked-choice voting, the alternative vote).

Each round, every ballot counts for its highest-ranked candidate still in the race; a ballot
whose ranked candidates are all eliminated is **exhausted** and counted as such in that round.
A candidate with more than half of the **continuing** votes (all ballots minus the exhausted
ones) wins. Otherwise the candidate with the fewest votes is eliminated and the next round
begins. One candidate is eliminated per round (no batch elimination).

Tie rule for elimination: among candidates tied for fewest votes, eliminate the one with the
fewest first-preference votes; if still tied, the one with fewer votes in the earliest later
round where they differ (round 2, then 3, …); if the counts never separate them, use
`tie_order` (the result of a drawing of lots, first listed loses first; by default the last
candidate in the candidate list loses). Every tie is reported in its round with how it was
resolved. When the last two candidates tie, both are reported in `tied_winners` and no winner
is chosen.

Ballots are strict rankings and may stop early; `count` weights a ballot. A candidate who
appears on no ballot can be named in `candidates`.

Test: the Tennessee capital example used throughout Wikipedia's voting-method articles (42 %
Memphis, 26 % Nashville, 15 % Chattanooga, 17 % Knoxville, as 100 voters): Chattanooga is
eliminated, Knoxville reaches 32, Nashville is eliminated and Knoxville wins 58 to 42.

Limits: no ties within a ballot; no rules for skipped rankings or overvotes (clean the ballots
first); no multi-winner variant (use `vote_stv`).
