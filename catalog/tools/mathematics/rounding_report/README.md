# rounding_report

Rounding without binary-float surprises, and honest reporting of measured values.

`operation: "round"` rounds `value` (a number or decimal string, read in `Decimal`) to
`sig_figs` significant figures or to `decimals` places (negative to tens, hundreds…) with
`mode`:

| mode | 2.5 | −2.5 | rule |
|:--|:--|:--|:--|
| `half_even` (default, IEEE 754 and Python's `round`) | 2 | −2 | ties to the even digit |
| `half_up` | 3 | −3 | ties away from zero (school rounding) |
| `half_down` | 2 | −2 | ties toward zero |
| `up` | 3 | −3 | away from zero |
| `down` | 2 | −2 | toward zero (truncate) |
| `ceiling` | 3 | −2 | toward +∞ |
| `floor` | 2 | −3 | toward −∞ |

The answer gives the result in positional, scientific (`1.23e-3`) and engineering (exponent a
multiple of 3) notation, and the result in every mode (`by_mode`). A carry is handled: 9.96 to
two significant figures is 10 (`1.0e1`).

`operation: "report"` takes a `value` and its standard `uncertainty`, rounds the uncertainty to
`uncertainty_sig_figs` (1 by default, or 2) and the value to the same decimal place:
9.81 ± 0.0234 → 9.81 ± 0.02.

`operation: "propagate"` combines independent inputs to first order (GUM 5.1.2, uncorrelated):
`kind: "sum"`, q = Σ cᵢxᵢ with u(q) = √Σ(cᵢuᵢ)² (`coefficient` per term, −1 to subtract);
`kind: "product"`, q = c·Πxᵢ^pᵢ with u(q)/|q| = √Σ(pᵢuᵢ/xᵢ)² (`power` per term, `constant` c);
`quotient` (two terms, the second divides) and `power` (one term) are shorthands. The full
value and uncertainty are returned (15 significant digits) and then reported as above.

The tests check Python's documented rounding examples (`round(2.5)` is 2; `Decimal('7.325')`
quantized to 7.32 with `ROUND_DOWN` and to 8 with `ROUND_UP`), the GUM 7.2.2 reporting example
(m_S = 100.021 47 g with u_c = 0.35 mg), and sums, products, quotients and powers worked by hand
with the quadrature rules.

Limits: first order only, so strongly non-linear functions or large relative uncertainties are
underestimated; no correlations, coverage factors or expanded uncertainty; one formula shape
per call (chain calls for mixed expressions).
