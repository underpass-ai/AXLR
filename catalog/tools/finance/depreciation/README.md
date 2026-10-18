# depreciation

Depreciation schedule of one asset in `Decimal`: each row has the year, the book value at the
start, the year's charge, the accumulated depreciation and the book value at the end. The
amounts are kept exact and each one is rounded to the cent on output, as Excel does, so a column
of rounded charges can differ from the rounded total by a cent.

With `first_year_months` m below 12 the asset is used m/12 of the first year, so the schedule
has `life + 1` rows and the last one takes the remaining `(12 − m)/12` of a year.

- **straight_line**: `(cost − salvage) / life` a year, prorated in the first and last rows.
- **sum_of_years_digits**: depreciation year k charges `(cost − salvage) × (life − k + 1) /
  (life(life + 1)/2)`; with a partial first year each fiscal year takes m/12 of one
  depreciation year and (12 − m)/12 of the previous one.
- **declining_balance**: `book value × rate` a year, rate `factor / life` (factor 2 by default)
  or `rate`; never below salvage. With `switch_to_straight_line` each year takes the larger of
  that and the remaining depreciable amount spread over the remaining life (as Excel's VDB),
  which reaches salvage at the end. Without the switch the book value may stay above salvage
  and `undepreciated_above_salvage` says how much (Excel's DDB behaves the same way).
- **double_declining**: declining balance with factor 2.
- **fixed_declining_balance**: Excel's DB: rate `1 − (salvage/cost)^(1/life)` rounded to three
  decimals, the first year prorated by m/12 and, with a partial first year, a last row of
  (12 − m)/12; it does not stop at salvage.
- **units_of_production**: `(cost − salvage) / total_units` per unit times each year's units,
  stopping at salvage.

The tests reproduce the examples of Excel's help pages: DB (1,000,000, salvage 100,000, 6
years, 7 months: 186,083.33 … 15,845.10, all seven rows), DDB (2,400, salvage 300, 10 years:
480.00 the first year, 22.12 the tenth; 306.00 the second with factor 1.5), SLN (2,250) and SYD
(4,090.91 … 409.09, all ten rows). The switch to straight-line, the partial-year SYD and the
units-of-production cases are checked by hand.

Limits: one asset; tax depreciation tables (MACRS, the Spanish *tablas de amortización*) and
their conventions (half-year, mid-quarter) are not built in; fiscal years are 12 months.
