# prob_dist

Distribution values and counting without table lookups.

With `distribution` and its parameters, `x` gives the pmf (discrete) or pdf (continuous), the
cdf P(X ≤ x) and the survival function P(X > x); `between: [a, b]` gives P(a ≤ X ≤ b); `q`
gives the quantile. Mean and variance are always returned. Parameters may be numbers or
decimal or fraction strings (`"0.3"`, `"1/3"`).

| Distribution | Parameters | Support and convention |
|:--|:--|:--|
| `binomial` | `n` (≤ 2000), `p` | successes in n trials |
| `poisson` | `lambda` (≤ 100000) | 0, 1, 2, … |
| `normal` | `mean`, `sd` | real line |
| `hypergeometric` | `population` N (≤ 2000), `successes` K, `draws` n | successes in n draws without replacement |
| `geometric` | `p` | trials up to and including the first success: 1, 2, … (as `scipy.stats.geom`) |
| `negative_binomial` | `r`, `p` | failures before the r-th success: 0, 1, … (as `scipy.stats.nbinom`) |
| `exponential` | `rate` | x ≥ 0 |
| `uniform` | `low`, `high` | continuous on [low, high] |

Exactness: the binomial, hypergeometric, geometric, negative binomial and uniform values are
computed over `Fraction` and returned in `exact` as fractions (binomial(10, 1/2) P(X = 5) =
63/256), next to 12-significant-digit floats; fractions longer than 1000 bits are left out.
The Poisson pmf is a float plus a symbolic form (`e^(-2) * 2^0 / 0!`), its cdf the regularized
incomplete gamma function Q(k + 1, λ) (Numerical Recipes 6.2); the normal cdf is
`0.5 erfc(−z/√2)` and its quantile `statistics.NormalDist.inv_cdf`; the exponential is in
closed form. A discrete quantile is the smallest k with P(X ≤ k) ≥ q, compared exactly (for the negative binomial, in log-space floating point, since its exact terms grow without bound for small p).

With `operation`: `choose` (n, k; `repetition` for multisets C(n + k − 1, k)), `permutations`
(n, optional k; `repetition` for n^k) and `multinomial` (`counts`), all exact integers, n up to
10000.

The tests check textbook values: binomial(10, ½) P(X = 5) = 63/256, Poisson λ = 2 P(X = 0) =
e⁻² = 0.13533528, the standard normal table value Φ(1.96) = 0.9750021 and its 97.5 % point
1.959964, the probability of exactly one ace in a poker hand (778320/2598960), C(52, 5) =
2598960 and the 34650 arrangements of MISSISSIPPI.

Limits: the size caps above keep the exact sums fast; the floats carry about 15 correct digits
at best, fewer deep in the tails of the Poisson cdf.
