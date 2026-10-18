# poll_margin_error

Sampling-error arithmetic for opinion polls, under simple random sampling and the normal
approximation.

- **z** for the confidence level: the two-sided quantile `Φ⁻¹(1 − (1 − c) / 2)`, from Peter
  Acklam's rational approximation refined by one Halley step with `math.erfc`, accurate to
  about 1e-15.
- **Margin of error** of a proportion `p` from `n` interviews:
  `z · √(p (1 − p) · deff / n) · fpc`, where `deff` is the design effect (default 1) and, when
  `population` `N` is given, `fpc = √((N − n) / (N − 1))`. Leaving `p` out uses 0.5, the
  widest margin, which is what pollsters quote.
- **Required sample size** for a target `margin` `E`: `n₀ = z² p (1 − p) · deff / E²`, then
  `n = n₀ / (1 + (n₀ − 1) / N)` with a population, both rounded up.
- **Lead** between two candidates in the same poll: their shares are negatively correlated
  (multinomial), so `Var(p₁ − p₂) = (p₁ + p₂ − (p₁ − p₂)²) / n`. The lead is significant when it
  is larger than `z` times that standard error; the two-sided p-value is also given. The margin
  for the lead is close to twice the margin quoted for a single share.

Confidence accepts `0.95`, `95` or `"95%"`; shares accept `0.52` or `"52%"`.

Tests: Wikipedia's *Margin of error* figure for 1000 interviews at 95 % (±3.1 points, z ≈
1.95996), quantiles at 90 %, 99 % and 99.9998 % against Python's `statistics.NormalDist`
(an independent algorithm, Wichura's AS 241), the familiar 1068 interviews for ±3 points, and
lead tests checked by hand.

Limits: no non-response or weighting model beyond a single design effect; the normal
approximation is poor for very small `n` or shares near 0 or 1 (use an exact or Wilson interval
then); sampling error is not the only error in a poll.
