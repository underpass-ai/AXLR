# recipe_scale

Scales an ingredient list and keeps the result measurable.

**Reading the lines.** A line starts with a quantity: a whole number, a decimal, a fraction
(`1/2`), a mixed number (`1 1/2`), a Unicode fraction (`½`, `1½`) or a range (`2-3`, `2–3`,
`2 to 3`). Then an optional unit (cup/cups/c, tablespoon/tbsp/T, teaspoon/tsp/t, fl oz, pint,
quart, ml, cl, dl, l, g, kg, mg, oz, lb, and countable or vague units such as clove, can, stick,
slice, pinch, dash) and the ingredient name. A line without a leading quantity ("salt to taste")
is kept as it is. Lines may also be objects `{quantity, unit, name}`.

**The factor.** `servings: {from, to}` gives to/from; `factor` takes `1.5`, `"3/2"` or `"150%"`;
`target: {ingredient, quantity, unit?}` scales so that the first ingredient whose name contains
`ingredient` comes to `quantity`. A target in another unit of the same kind converts exactly
(US cup = 16 tbsp = 48 tsp, 236.5882365 mL; lb = 453.59237 g; oz = 28.349523125 g, NIST
Handbook 44).

**The result.** Every quantity is multiplied in fractions and shown exactly (`exact`) and then, by
default, as something you can measure (`quantity`, `unit`, `scaled`):

- US volumes are moved to a handier unit: under 1/4 cup, or a cup fraction off the 1/4 and 1/3
  marks that is a whole or half tablespoon (3/8 cup = 6 tbsp), goes to tablespoons; 3 tsp or
  more that is a whole or half tablespoon goes to tablespoons; tablespoons that make whole
  quarter cups go to cups;
- rounding: cups to the nearest 1/4 or 1/3 mark (a tie goes to the simpler fraction), tbsp to
  1/2, tsp to 1/8, fl oz to 1/2, g and ml to whole units (0.1 g under 10 g), oz, lb, pints and
  quarts to 1/4, counts to 1/2; anything that would round to 0 is shown as the smallest step;
- eggs (a count whose name mentions egg) are rounded to a whole egg, at least one, and flagged
  with the alternative of a part of a beaten egg;
- pinches and dashes scale with a note that they are not precise.

`round: false` and `convert_units: false` turn these off.

The tests check hand-worked scalings (4 to 6 servings: 1 1/2 cups → 2 1/4 cups, 2/3 cup → 1 cup,
½ tsp → 3/4 tsp, a 2-3 tbsp range → 3 - 4 1/2 tbsp, 3 eggs → 4 1/2 → 5 flagged), the cup and
spoon relations of NIST Handbook 44 (3/8 cup = 6 tbsp, 3 tsp = 1 tbsp) and a target given in
pounds against a recipe in grams.

Limits: one quantity per line ("1 cup plus 2 tbsp" keeps only the first); no conversion between
volume and weight (see `cooking_unit_convert`); British and metric cups are not distinguished
from the US cup when moving between cups and spoons; cooking times, pan sizes and leavening do
not scale linearly and are not adjusted.
