# compound_interest

Time-value-of-money formulas in `Decimal` (50 digits; money rounded to the cent, rates to ten
places and percents to four). Amounts are positive: there is no spreadsheet sign convention.
`rate` is a nominal annual rate; with `compounding` m the period rate is `i = rate / m` and the
term is `n = years × m` periods (or `periods` directly). Payments fall once per period.

- **future_value**: `PV·(1 + i)^n + PMT·((1 + i)^n − 1)/i`, the annuity part times `(1 + i)`
  when `due` (payments at the start of each period); `PV·e^(rate·years)` with continuous
  compounding. A lump sum may run a fractional number of periods; an annuity needs whole ones.
- **present_value**: the same discounted, `FV/(1 + i)^n + PMT·(1 − (1 + i)^−n)/i`.
- **effective_rate**: `(1 + rate/m)^m − 1`, or `e^rate − 1` continuous.
- **nominal_rate**: `m·((1 + effective)^(1/m) − 1)`, or `ln(1 + effective)` continuous.
- **doubling_time**: exactly `ln 2 / (m·ln(1 + rate/m))` years, beside the rules of 72, 70 and
  69.3 (`k / rate%`) and each rule's error.
- **real_rate**: Fisher's `(1 + nominal)/(1 + inflation) − 1`, beside the approximation
  `nominal − inflation`.

The tests reproduce the standard textbook figures (1,000 at 5 % for 10 years is 1,628.89; 12 %
monthly is 12.6825 % effective) and the examples of Excel's help pages for FV (2,581.40), PV
(59,777.15), EFFECT (0.0535427) and NOMINAL (0.05250032).

Limits: one constant rate; payments and compounding share the same frequency; no growing
annuities, no solving for the rate or the term (see `npv_irr` and `loan_amortization`).
