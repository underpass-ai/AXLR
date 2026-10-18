# dice_roll_seeded

Rolls dice reproducibly and works out the exact odds.

**Notation** (case and spaces ignored): terms joined by `+` or `-`. A term is a constant or
`NdM` (N dice of M faces; N defaults to 1; `d%` is d100; `dF` is a Fudge die showing −1, 0 or
+1) followed by modifiers: `khN` or `kN` keep the N highest, `klN` keep the N lowest, `dhN` drop
the N highest, `dlN` drop the N lowest, `!` exploding (a die showing its maximum is rolled again
and added, again and again). Up to 100 dice of up to 1000 faces per term and 20 terms.

**Roll.** With `seed` (an integer or a string) the dice are rolled with Python's
`random.Random(seed)`, one `randint(1, M)` per die in term order, so the same seed and notation
always give the same dice. Each die is shown (an exploded die as the list of its rolls), with
the kept and dropped dice; `rolls` repeats the roll up to 1000 times from the same generator.
Without `seed` there is no roll, only the odds.

**Odds.** Each term's distribution is exact in fractions: plain dice by repeated convolution of
the uniform distribution; keep/drop terms by enumerating the multisets of faces, each weighted
by its multinomial count; exploding dice by the series P(kM + r) = (1/M)^(k+1) for r < M,
tabulated to `explode_depth` explosions per die, with the probability left out reported as
`truncated_probability`. Terms are then convolved (a subtracted term is mirrored). The answer
gives the table of totals, min and max, mode, mean and variance as fractions and decimals, the
standard deviation, and with `target` the probabilities of at least and at most that total.
Mean and variance of exploding dice are exact for the infinite series: with K explosions
(geometric, mean 1/(M−1), variance M/(M−1)²) and a last roll uniform on 1…M−1, a die has mean
M/(M−1) + M/2 and variance M²·M/(M−1)² + ((M−1)² − 1)/12. Keep/drop combined with exploding
dice is rolled but not tabulated.

The tests check textbook values (2d6: P(7) = 1/6; 3d6: mean 10.5, variance 35/4, P(10) = 1/8,
P(18) = 1/216), the published mean of 4d6 drop lowest (15869/1296 ≈ 12.2446), advantage
(2d20 keep highest: P(≥ 15) = 51/100), the exploding d6 (mean 4.2), Fudge dice and a seeded
roll.

Limits: no rerolls, success counting, compounding or penetrating explosions, or arithmetic other
than + and −; very large dice pools may be refused for the table (more than 300 000 multisets
or 200 000 distinct totals).
