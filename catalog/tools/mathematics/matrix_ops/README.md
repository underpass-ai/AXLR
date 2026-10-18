# matrix_ops

Linear algebra without rounding. Entries may be numbers, decimal strings or fractions
(`"1/3"`); everything is computed over `Fraction` and returned as fraction strings.

Every operation goes through Gauss–Jordan elimination with exact pivots (the first non-zero
entry in the column, since there is no rounding to guard against):

- `determinant`: the product of the pivots, sign-flipped per row swap; 0 when a column has no pivot.
- `inverse`: row-reduces [A | I]; a singular matrix returns `invertible: false` (not an error).
- `rank` (and nullity), `rref` with its pivot columns (1-based).
- `solve` A x = b: row-reduces [A | b]. A pivot in the last column means **none** (rank A <
  rank [A | b]); rank = number of unknowns means **unique**; otherwise **infinite**, described by
  the free variables, one equation per pivot variable (`x2 = 3 - 2*x3`), a particular solution
  (free variables 0) and a basis of the null space, so every solution is particular + Σ tᵢ·basisᵢ.
- `transpose`, `multiply` (`matrix` × `other`).

The tests use the worked examples of Wikipedia's "Gaussian elimination" article (the system
2x + y − z = 8, −3x − y + 2z = −11, −2x + y + 2z = −3 with solution (2, 3, −1), and the inverse of
the tridiagonal [[2, −1, 0], [−1, 2, −1], [0, −1, 2]], which is ¼·[[3, 2, 1], [2, 4, 2], [1, 2, 3]]),
the determinant −1 of the first system computed by cofactors by hand, and the singular 1…9 matrix.

Limits: at most 30 × 30 (exact fractions grow quickly); no eigenvalues, decompositions or
least-squares solutions for inconsistent systems.
