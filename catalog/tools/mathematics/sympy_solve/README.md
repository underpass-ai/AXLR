# sympy_solve

Symbolic mathematics on one expression: `solve` (an equation `lhs = rhs`, or an
expression equal to zero), `simplify`, `expand`, `factor`, `differentiate` (with
`order`), `integrate` (indefinite, or definite with `lower` and `upper`), `limit`
(at `point`, one-sided with `direction`) and `series` (at `point`, to `order`).
Results come back as sympy text, as LaTeX and, when numeric, as a decimal to 15
significant digits; the arithmetic is exact.

The variable is inferred when the expression has exactly one free symbol; name
it with `symbol` otherwise. `^` is accepted as a power and `2x` as `2*x`.

This is the catalog's one package-tier tool: it needs `sympy` (the registry's
sandbox pins 1.13.3, and the console's install card warns when the package is
missing on the machine). An expression may contain letters, digits, operators,
parentheses, commas, dots and `=` only, so the parser never sees code.

Limits: one expression per call; `solve` returns explicit solutions only (no
inequalities, no systems); series and limits in one variable; no plotting.
