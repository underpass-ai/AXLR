# ohms_law

DC resistor arithmetic with exact fractions.

- **solve**: give exactly two of `voltage`, `current`, `resistance`, `power`; the other two
  follow from V = I·R and P = V·I = I²·R = V²/R. Every result is an exact fraction except when
  resistance and power are given, where V = √(P·R) and I = √(P/R) may be irrational: then the
  value is computed to 60 digits and shown to `digits`, its `exact` is null and the answer
  says `"exact": false`.
- **network**: an expression where `+` puts resistors in series and `||` (or `//`) in parallel,
  `||` binding tighter than `+`, with parentheses to any depth (up to 100):
  `"(1k + 2k2) || 4k7 + 100"`. Series adds, parallel adds reciprocals; a 0 Ω branch in parallel
  shorts the group. With `voltage`, the current and power drawn are given too.
- **divider**: Vout = Vin·R2/(R1 + R2); with `load`, R2 is replaced by R2 ∥ load and the unloaded
  output is shown beside it. Also the divider current, the power in R1 and the Thévenin
  resistance R1 ∥ R2.
- **led**: R = (Vsupply − n·Vf)/I and its dissipation for n LEDs in series, plus a suggested power
  rating of twice the dissipation.

Quantities may be numbers or strings with SI prefixes and units (`"4.7k"`, `"4k7"`, `"470R"`,
`"20mA"`, `"3.3V"`, `"250mW"`). Each value comes back as an exact fraction, a decimal with
`digits` significant digits and an SI-prefixed rendering.

The tests check hand-worked circuits (12 V across 4 Ω is 3 A and 36 W; 100 + (220 ∥ 330) + 1k
= 1232 Ω; a 10k/10k divider from 10 V gives 5 V, and 10/3 V with a 10k load; 5 V, 2 V LED at
20 mA needs 150 Ω), and the irrational case.

Limits: DC and ideal resistors only (no AC, reactance, temperature, tolerances or bridges that
are not series-parallel reducible); magnitudes, no signs; the LED forward voltage is taken as
constant.
