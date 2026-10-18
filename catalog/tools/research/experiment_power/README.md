# experiment_power

Power analysis for the common two-group and one-group designs: the sample size needed for a
target power (`solve: n`, the default without `n`), the power of a given size (`solve: power`,
the default with `n`), or the smallest effect detectable with a given size and power
(`solve: effect`).

## Designs and formulas

- `two_sample` (independent groups, equal variances): `df = n1 + n2 − 2`, noncentrality
  `δ = |d| √(n1 n2 / (n1 + n2))`. The effect is `d`, or `(mean1 − mean2) / sd`.
- `one_sample` and `paired`: `df = n − 1`, `δ = |d| √n`; the effect is `d`, `(mean − mu0) / sd`
  or, paired, `d_z = mean_diff / sd_diff` (the SD of the differences).
- `two_proportions`: `method: normal` (default) uses the pooled proportion under H0 and the
  unpooled variance under H1, `z_β = (|p1 − p2| − z_α SE0) / SE1`, which is R's
  `power.prop.test`; `method: arcsine` uses Cohen's `h = 2 asin √p1 − 2 asin √p2` with
  `z_β = |h| √(n1 n2 / (n1 + n2)) − z_α`.

`method: t` (default for the t designs) computes power from the noncentral t distribution:
`power = P(T′ > t_crit) + P(T′ < −t_crit)` for two sides, with `t_crit` the central t quantile
at `1 − α/2`. The noncentral t CDF is Lenth's (1989) series, Applied Statistics algorithm AS
243: `Φ(−δ) + ½ Σ_j [p_j I_x(j + ½, ν/2) + q_j I_x(j + 1, ν/2)]` with Poisson weights in `δ²/2`
and `x = t² / (t² + ν)`, over the regularized incomplete beta function (continued fraction,
Numerical Recipes 6.4). It agrees with direct numerical integration to about 1e-9. `method:
normal` replaces t by z, which gives the textbook formulas `n = ((z_α + z_β) / d)²` for one
sample and `n = 2 ((z_α + z_β) / d)²` per group for two equal groups.
The output's `method` says which was used. Two-sided power counts both tails (R's
`strict = TRUE`); the lower tail is negligible in practice, so R's default answers agree.

## Solving

`n_exact` is the continuous size where the power equals the target (bisection, as R reports
it); `n1` (or `n`) is the smallest whole size reaching the target, `n2 = ⌈ratio × n1⌉` with
`ratio = n2 / n1` (default 1), and `power` is the power actually achieved by those sizes. With
`solve: power` or `effect`, `n × ratio` must be a whole number. `solve: effect` returns `d` for
the t designs and, for proportions, the `p2 > p1` that reaches the power. One-sided tests
(`sides: 1`) assume the effect lies in the tested direction.

## Tests

Published values: Cohen (1988) and G*Power, d = 0.5, α = .05 two-sided, power .80 needs 64 per
group (128 in all, actual power 0.8014596); the examples of R's help pages
`power.t.test(n = 20, delta = 1)` (power 0.8689528), `power.t.test(power = .90, delta = 1)`
(n = 22.02110) and its one-sided variant (n = 17.84713), `power.prop.test(p1 = .50,
p2 = .75, power = .90)` (n = 76.7) and `power.prop.test(n = 50, p1 = .50, p2 = .75)` (power
0.740); and the arcsine and normal-approximation formulas worked out by hand.

## Limits

Equal-variance t tests only (no Welch power), no cluster, crossover or ANOVA designs, no
continuity correction for proportions, and no dropout inflation (divide the size by the
expected retention yourself).
