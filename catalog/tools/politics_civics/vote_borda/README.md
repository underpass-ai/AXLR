# vote_borda

Positional scoring of ranked ballots. With `n` candidates, position `i` (0 for first) earns:

| `variant` | points |
|---|---|
| `borda` (default) | `n − 1 − i` (3, 2, 1, 0 for four candidates) |
| `borda_n` | `n − i` (4, 3, 2, 1) |
| `dowdall` | `1 / (i + 1)` (1, ½, ⅓, ¼), Nauru's rule |

Totals are exact fractions (`exact`) and decimals rounded to six places (`scores`). The ranking
lists tied totals as inner lists; `winners` holds every top scorer.

Ballots may tie candidates (an inner list) and may leave candidates out; the unranked
candidates are treated as one tie at the bottom. `partial` decides how a tie scores:

- `lowest` (default): every candidate in a tie gets the points of the lowest position the tie
  spans, so unranked candidates get 0 under `borda`;
- `average`: each gets the mean of the points of the positions the tie spans (for `borda` this
  equals "tournament" Borda: one point per candidate beaten, half per candidate tied);
- `modified` (Borda variants only): unranked candidates get 0 and the `k` ranked candidates are
  scored `k … 1`, the modified Borda count (Slovenia's minority seats); ties among ranked
  candidates are averaged.

Under Dowdall, unranked candidates always score 0.

Tests: the Tennessee capital example of Wikipedia's *Borda count* article, 100 voters (Nashville
194, Chattanooga 173, Memphis 126, Knoxville 107) and its Dowdall version (Nashville 173/3 ≈
57.667, Memphis 56.5, Chattanooga 50.5, Knoxville 131/3 ≈ 43.667; the article prints one cell,
Knoxville from Nashville voters, as 8.333 where 26/3 ≈ 8.667 is right, but its totals agree with
these); partial-ballot scoring checked by hand.

Limits: no truncation rules such as "score only the top three"; no tie breaking among equal
totals.
