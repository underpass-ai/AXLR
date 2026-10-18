# rubric_grade

Scores students against a weighted analytic rubric, in exact fractions.

A rubric has criteria, each with a `weight` (default 1) and levels with `points`, a `label` and a
`descriptor`. A student's score for a criterion is a level label or a number of points from 0 to
the criterion's maximum (the highest level's points). For each criterion:

- `share = weight × points / max_points`, so a criterion contributes its weight in full at the
  top level;
- `percentage = Σ shares / Σ weights × 100` (exact in `percentage_exact`, rounded half up to
  `decimals` in `percentage`); `raw_points` / `raw_max` are the plain sums, unweighted;
- with `scale` (for example 10), the result is also given on 0..scale;
- with `bands` (`min_percent`, `grade`, given in the input because grading scales differ), the
  grade is the band with the highest `min_percent` not above the percentage.

Feedback is one line per criterion, "Criterion: Level (points/max) - descriptor". Points between
two levels get the descriptor of the level below, marked as such. With several students a
summary gives the mean, minimum and maximum percentage and the count per grade.

The tests are worked by hand: Content (weight 2) Good 3/4, Organization Excellent 4/4, Language
2/3 is 2·¾ + 1 + ⅔ = 19/6 of 4 = 79.17 %, grade C on the bands given, 7.92 on 10; a class of three
with a mean of exactly 75 %; points between levels; equal weights.

Limits: holistic or single-point rubrics are not modelled; no rounding of the grade before
banding (the exact percentage decides); weights apply per criterion, not per level.
