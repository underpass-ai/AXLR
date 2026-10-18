# heart_rate_zones

Training zones from an estimated or measured maximum heart rate.

**Maximum heart rate** estimates for `age` (all three are always shown, `formula` picks the one
used): Tanaka, Monahan & Seals (2001) `208 − 0.7 × age` (the default; from a meta-analysis of
351 studies), Fox, Naughton & Haskell (1971) `220 − age`, and Gellish et al. (2007) `207 − 0.7 ×
age`. A measured `max_hr` replaces the estimate.

**Zones** are bounded by `boundaries`, ascending percentages (default 50, 60, 70, 80, 90, 100:
five zones), with optional `zone_names`:

- Karvonen (heart-rate reserve, needs `resting_hr`): `rest + (max − rest) × p/100`;
- percentage of maximum: `max × p/100`.

The arithmetic is exact (fractions); `low_bpm`/`high_bpm` are rounded half up to whole beats
and `*_exact` keep one decimal. With `resting_hr` both methods are given unless `method` says
otherwise.

The tests check that the Tanaka and Fox equations coincide at age 40 (180 bpm), as Tanaka et
al. point out, the Gellish value 179 at 40, a Karvonen zone worked by hand (age 40, rest 60:
HRR 120, 70–80 % is 144–156 bpm) and a percentage-of-maximum rounding case (182.5 × 0.6 =
109.5 → 110).

Every output, errors included, carries the disclaimer: *Not medical advice: population
formulas that can be off by 10 bpm or more for an individual. Check with a qualified clinician
before exercising, especially with a heart condition or medication.*

Limits: the estimates have a standard error of about 10 bpm; beta-blockers and other drugs
change heart rate; no lactate-threshold or heart-rate-variability methods.
