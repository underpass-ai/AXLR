# vote_condorcet

Condorcet analysis of ranked ballots. For every pair of candidates it counts how many voters
prefer one to the other (`pairwise[a][b]`), then reports:

- the **Condorcet winner** (beats every other candidate head to head) and **loser**, or `null`;
- the **Smith set**, the smallest group that beats everyone outside it;
- **Copeland** scores (1 per pairwise win, ½ per tie) and their winners;
- the **Schulze** method: strongest paths by the widest-path algorithm (winning votes), the
  winners (`p[x][y] ≥ p[y][x]` for every `y`) and the full ranking, ties as inner lists.

Ballots are rankings; an inner list is a tie (`["A", ["B", "C"]]`), and candidates a ballot
leaves out tie below every candidate it ranks. `count` weights a ballot.

Method: Schulze, *A new monotonic, clone-independent, reversal symmetric, and
Condorcet-consistent single-winner election method*, Social Choice and Welfare 36 (2011); the
first test is the paper's 45-voter example (winner E, ranking E > A > C > B > D).

Limits: Schulze uses winning votes; margins and other variants are not offered. No random tie
breaking: tied Schulze tiers are reported as ties.
