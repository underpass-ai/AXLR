# sampling_plan

How big a sample has to be, and how to split it across strata.

**Proportion** (`target: "proportion"`): n₀ = z² p(1 − p) · deff / e², with `margin` e as a
proportion (0.05 is ±5 points), `p` the expected proportion (0.5, the worst case, by default)
and `design_effect` deff. **Mean** (`target: "mean"`): n₀ = (z s / e)² · deff with the expected
`sd` s and `margin` in the data's units; with `distribution: "t"` the z is replaced by Student's
t with n − 1 degrees of freedom and n is iterated until it stops changing.

z is the normal quantile of `confidence` (1.959964 at 95 %) unless a fixed `z` is given, such
as the textbook 1.96; both give 385 for ±5 % at 95 %. With a `population` N the finite-population
correction of Cochran (1977, ch. 4) is applied, n = n₀ / (1 + (n₀ − 1)/N), and n is rounded
up. `response_rate` turns n into the number of people to contact, ⌈n / rate⌉.

**Stratified** (`target: "stratified"`): a total `n` split over `strata` of known `size` N_h:
`proportional` n·N_h/N, `neyman` n·N_h S_h / Σ N_i S_i (needs each stratum's `sd`; Cochran
ch. 5), or `equal` n/H. The exact shares are returned as fractions and rounded by the
largest-remainder method so they add up to n. When every stratum has an `sd`, the variance of
the stratified mean Σ W_h² S_h² / n_h · (1 − n_h/N_h) is reported. A warning appears when a
Neyman allocation exceeds a stratum's size.

The tests check Cochran's 385 and three rows of Krejcie and Morgan's (1970) published table
(N = 100 → 80, 1000 → 278, 10000 → 370; their formula is the same correction with χ² = 3.841),
and a Neyman allocation computed by hand.

Limits: simple random sampling within strata; no cost-weighted (optimum) allocation, cluster
designs beyond a design effect, or power calculations for tests.
