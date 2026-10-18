# grade_stats

Three jobs on a class's marks, in exact fractions where the arithmetic allows it (means,
medians, variances, KR coefficients) and 50-digit decimals for square roots.

**stats**: count, mean, median, minimum, maximum, population SD (divide by n) and sample SD
(n − 1), with the exact variances. With `bands` (`min`, `label`, given in the input) each score
falls in the band with the highest `min` it reaches, with counts and percentages; without them, a
ten-bin histogram over 0..`max_score`.

**curve** (`method`):

- `linear`: `new = target_mean + (x − mean) × target_sd / sd` with the population SD (z-scores
  mapped to a new mean and SD); without `target_sd`, only the mean is shifted;
- `sqrt`: `new = √(x / max) × max` (10√x out of 100);
- `top_score`: `new = x × max / top score`.

With `clip` (default) results stay within 0..`max_score`; the statistics before and after are
given.

**item_analysis** on a students × items matrix of 0/1:

- difficulty `p` (share correct) per item;
- point-biserial `r_pb`, the Pearson correlation of the item with the total score, and the
  corrected item-rest correlation (total without the item);
- upper-lower discrimination `D = p_upper − p_lower`, the groups being the top and bottom
  `group_fraction` (default 27 %, Kelley) of students by total score, `round(0.27 n)` rounded
  half up, at least 1 and at most n/2, ties broken by the order of the rows;
- KR-20 = `k/(k−1) × (1 − Σ p q / σ²)` and KR-21 = `k/(k−1) × (1 − M (k − M) / (k σ²))`, with σ²
  the population variance of the totals (Kuder & Richardson's form; `variance: "sample"` uses
  n − 1, as some software does, and gives a higher value);
- flags for items with p < 0.20 or > 0.90, D < 0.20 or a negative item-rest correlation.

**Worked example** (a test): six students answer three items as 111, 110, 101, 010, 100, 000.
Item p = 4/6, 3/6, 2/6, so Σpq = 2/9 + 1/4 + 2/9 = 25/36. Totals 3, 2, 2, 1, 1, 0 have mean
3/2 and squared deviations summing to 5.5, so σ² = 11/12. KR-20 = 3/2 × (1 − (25/36)/(11/12)) =
3/2 × (1 − 25/33) = 4/11 ≈ 0.364. For item 1 the four students who got it right average 2
and the two who did not 0.5, so r_pb = (2 − 0.5)/√(11/12) × √(2/3 · 1/3) = 0.7385; 27 % of 6
rounds to 2 students per group, and D = 2/2 − 1/2 = 0.5. A second test uses a perfect Guttman
pattern on four items (KR-20 = 4/3 × (1 − 0.8/2) = 0.8, r_pb = 1/√2).

The other tests: the worked example of Wikipedia's *Standard deviation* article, 2, 4, 4, 4, 5, 5, 7, 9 (mean 5, population SD 2), and
each curve by hand.

Limits: dichotomous items only (no Cronbach's alpha for partial credit), no IRT, no
distractor analysis; with small classes the 27 % groups are a handful of students.
