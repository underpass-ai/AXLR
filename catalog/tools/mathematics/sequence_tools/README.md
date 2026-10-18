# sequence_tools

Guess the rule behind a list of numbers, honestly. Terms may be numbers, decimal or fraction
strings and are compared exactly as `Fraction`s.

Models tried, each only when the terms over-determine it (at least one term beyond those needed
to fix it):

| Model | Terms that fix it | Closed form |
|:--|:--|:--|
| constant / arithmetic | 2 | a(n) = a + d n |
| geometric (all terms non-zero) | 2 | a(n) = C r^n |
| polynomial of degree d, 2 ≤ d ≤ len − 2 (the d-th finite differences are constant) | d + 1 | Newton's forward-difference formula expanded in powers of n |
| linear recurrence a(n) = c₁a(n−1) + … + c_k a(n−k), k ≤ 10 and 2k + 1 ≤ len | 2k (k coefficients solved from k equations, k initial terms) | for k = 2 with rational characteristic roots, A r₁ⁿ + B r₂ⁿ (or (A + Bn) rⁿ); otherwise none, with the characteristic polynomial |

The model fixed by the fewest terms wins (ties: arithmetic, geometric, polynomial, recurrence).
The answer reports `determined_by` and `checked_on` — how many of the given terms fix the model
and how many more agree with it — and says plainly that this is the simplest fit, not a proof.
Other models that also fit are listed in `also_fits`, with `agrees_on_next` telling whether they
predict the same continuation. `start_index` sets the n of the first term (1 by default), which
changes the closed form but not the next terms.

The tests use OEIS sequences: squares (A000290), triangular numbers (A000217), Fibonacci
(A000045), powers of two (A000079), Lucas numbers (A000032) and 2ⁿ − 1 (A000225); factorials
match none of the models and get no answer.

Limits: no non-linear, holonomic or non-constant-coefficient recurrences (factorials,
Catalan numbers), no interleaved or periodic-with-offset patterns beyond what a recurrence
captures; 3 to 200 terms, up to 100 further terms.
