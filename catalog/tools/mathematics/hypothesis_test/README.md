# hypothesis_test

The everyday significance tests, with the same conventions as R's `stats` package.

| `test` | Input | Statistic and distribution |
|:--|:--|:--|
| `welch_t` | `x`, `y` | t with Welch–Satterthwaite df (R `t.test` default) |
| `student_t` | `x`, `y` | t with pooled variance, df = n₁ + n₂ − 2 (`var.equal = TRUE`) |
| `paired_t` | `x`, `y` paired | t on the differences, df = n − 1 |
| `one_sample_t` | `x`, `mu` | t, df = n − 1 |
| `chi_square_gof` | `observed`, `expected` counts or `proportions` | Σ(O − E)²/E, df = k − 1 − `ddof` |
| `chi_square_independence` | `table` r×c | Pearson chi-square, df = (r − 1)(c − 1); Yates' correction on a 2×2 unless `yates: false` (R and scipy default) |
| `fisher_exact` | `table` 2×2 | hypergeometric, exact |
| `one_proportion_z` | `successes`, `trials`, `p0` | z with the null standard error; Wilson interval |
| `two_proportion_z` | `successes`, `trials` as pairs | z with the pooled standard error; unpooled Wald interval |

`mu` is the null mean (or mean difference), `alternative` is `two-sided`, `less` or
`greater`, and the t and proportion tests return a two-sided confidence interval at
`confidence` (95 % by default) whatever the alternative.

Method: means, variances, the Welch df, expected counts and the chi-square statistic are summed
over `Fraction` and also returned exactly. The t p-value is I_{df/(df+t²)}(df/2, ½), the
regularized incomplete beta (Numerical Recipes 6.4, the same code as `stats_describe`); the
chi-square p-value is Q(df/2, χ²/2), the regularized incomplete gamma (NR 6.2); z p-values use
`erfc`. Fisher's p-value is an exact fraction: two-sided sums every table with the observed
margins whose probability does not exceed the observed one, as R does (R adds a 10⁻⁷ relative
tolerance because it works in floats; here the comparison is exact). The odds ratio returned is
the sample ad/bc, not R's conditional maximum-likelihood estimate.

The tests reproduce R's documented examples: `t.test(extra ~ group, data = sleep)` (Welch t =
−1.8608, df = 17.776, p = 0.07939, interval −3.3654832 to 0.2054832; pooled p = 0.07919;
paired t = −4.0621, p = 0.002833), the `chisq.test` example from Agresti (2007) (X² = 30.07,
df = 2, p = 2.954e−07), the `fisher.test` tea-tasting example (p = 0.4857 = 17/35; one-sided
0.2429), and Mendel's dihybrid counts 315:108:101:32 against 9:3:3:1 (χ² = 0.470, p = 0.925).

Limits: no multiple-comparison correction, ANOVA or non-parametric tests; Fisher only on 2×2
tables with a total up to 10000; samples up to 100000 values; displayed floats carry 12
significant digits. A warning is added when an expected count is below 5.
