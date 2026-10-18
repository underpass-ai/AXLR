# break_even

Cost-volume-profit analysis in `Decimal`, for one product or a sales mix.

- **Contribution margin** `price − variable_cost` per unit and its ratio to the price.
- **Break-even** `fixed_costs / margin` units (exact to four places and rounded up to whole
  units) and the revenue at that point.
- **At a sales level** (`sales_units` or `sales_revenue`): contribution, operating income,
  margin of safety (sales above break-even, in money, percent of sales and units) and the
  degree of operating leverage, contribution over operating income (not meaningful at or below
  break-even, and the output says so).
- **Target profit**: `(fixed + target) / margin` units, `(fixed + target) / ratio` revenue;
  with `tax_rate` the target is after tax and is grossed up to `target / (1 − t)` first.
- **Sales mix** (`products`): with `mix_basis: "units"` the mix weights the unit margins into a
  weighted margin per unit; with `"revenue"` it weights the margin ratios. The break-even is
  split across products in the mix.
- **Sensitivity**: every combination of the given percent changes to price, variable cost and
  fixed costs, with the break-even of each (one product).

The tests reproduce the Acoustic Concepts example of Garrison, Noreen and Brewer's *Managerial
Accounting* (price $250, variable cost $150, fixed $35,000: 350 units and $87,500 to break
even; at 400 units a 12.5 % margin of safety and an operating leverage of 8; 750 units for a
$40,000 profit). The sales-mix and sensitivity cases are checked by hand, including that a mix
stated by units and the equivalent mix stated by revenue give the same break-even.

Limits: linear costs and revenue within the relevant range; one period; no step costs, no
inventory changes, no capacity limit.
