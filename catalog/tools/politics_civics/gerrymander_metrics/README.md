# gerrymander_metrics

Fairness and shape metrics for a districting plan.

**Votes** (two parties per district, exact fractions, printed rounded to six decimals):

- *Wasted votes*: every vote for a district's loser, plus the winner's votes beyond what it
  needed. `winner_threshold: "half"` (default) counts the winner's votes above half of the
  district's two-party votes, as Stephanopoulos and McGhee (2015) do; `"majority"` counts those
  above `floor(total / 2) + 1`, the convention of Wikipedia's worked example. In a tied
  district neither party wins and all votes of both count as wasted (so the tie does not move
  the gap).
- *Efficiency gap*: `(wasted by the second party − wasted by the first) / all votes`;
  `efficiency_gap` is its size, `efficiency_gap_favours` the party that wasted fewer votes and
  `efficiency_gap_signed` is positive when the first party gains. `efficiency_gap_equal_turnout`
  is the shortcut `(S − ½) − 2 (V − ½)` for the first party's seat share `S` and vote share `V`,
  which equals the gap only when every district has the same turnout.
- *Mean-median*: the first party's mean and median district vote share; a positive
  `median_minus_mean` means its median district does better than its average, an advantage.
- *Seats against votes*: vote share, seat share and their difference per party.

**Shapes** (`polygon`, or `parts` for a district in pieces; no holes): area by the shoelace
formula, perimeter as the sum of edge lengths, **Polsby-Popper** `4πA / P²` and **Reock**
`A / (area of the smallest circle enclosing the district)`; the circle comes from Welzl's
algorithm over the vertices. Both are 1 for a circle and fall towards 0 as the shape stretches.
Coordinates are planar: the tool takes distances as given, so project longitude and latitude
(to an equal-area projection for Reock) before calling it.

Tests: Wikipedia's *Efficiency gap* sample calculation (five districts of 100, four won 53–47
and one lost 15–85: wasted 23 and 222, gap 39.8 % for A under the 51-vote rule), the same plan
under the half rule (27 and 223, 0.392, equal to the shortcut), and closed-form compactness:
square π/4 and 2/π, regular hexagon π√3/6 and 3√3/(2π), a 64-gon inscribed in a circle
π/(64 tan(π/64)) and 32 sin(π/32)/π, and an L-shape by hand.

Limits: two parties only (drop or reassign third-party votes first); no uncontested-seat
imputation; no polygon holes or validity checks (self-intersecting outlines give meaningless
areas); no geodesic measures.
