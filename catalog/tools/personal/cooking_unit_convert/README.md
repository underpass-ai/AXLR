# cooking_unit_convert

Kitchen conversions with the definitions stated.

**Volumes.** `cup`, `tbsp`, `tsp`, `fl_oz` and `pint` mean what `system` says:

| system | cup | tbsp | tsp | fl oz | source |
|:--|:--|:--|:--|:--|:--|
| `us_customary` (default) | 236.5882365 mL | 14.78676478125 mL | 4.92892159375 mL | 29.5735295625 mL | NIST Handbook 44, App. C (gallon = 231 in³) |
| `us_legal` | 240 mL | 15 mL | 5 mL | 30 mL | 21 CFR 101.9(b)(5)(viii), nutrition labelling |
| `metric` | 250 mL | 15 mL | 5 mL | (US) | Australia, Canada, New Zealand, South Africa |
| `uk` | 284.130625 mL (½ imperial pint) | 15 mL | 5 mL | 28.4130625 mL | imperial gallon 4.54609 L |

Explicit units ignore `system`: `us_cup`, `legal_cup`, `metric_cup`, `uk_cup`, `us_tbsp`,
`us_tsp`, `au_tbsp` (20 mL), `us_fl_oz`, `uk_fl_oz`, `us_pint`, `uk_pint`; plus `ml`, `cl`,
`dl`, `l`. Masses: `g`, `kg`, `mg`, `oz` (avoirdupois, 28.349523125 g) and `lb` (453.59237 g).
Conversions within volume or within mass are exact fractions (`exact`), with a decimal of
`digits` significant digits; each answer shows the definition of both units in mL or g.

**Volume ↔ weight** needs a density: `density` in g/mL, or `ingredient` from the bundled table
(`list: true` lists it). The table is written by `tools/catalog/data/cooking_unit_convert.py`.
It is meant to hold the household-measure weights (grams per cup, tablespoon or teaspoon) of 20
common baking ingredients from USDA FoodData Central, SR Legacy (public domain, CC0), fetched by
FDC id with the public API. **In this version the fetch failed**: the API's shared DEMO_KEY
answered 429 (rate limit) on 2026-10-18, so the file lists those 20 ingredients under
`not_fetched` and holds only water (1 g/mL, the gram's original definition; 0.9982 g/mL at
20 °C). Asking for one of them says so and asks for a density. Rerunning the generator when the
API answers fills the table (a new version of the tool). USDA's household cup is taken as the US
customary cup when weights are turned into densities.

**Oven temperatures.** °C and °F exactly (°F = °C × 9/5 + 32). Gas marks follow the UK scale
°F = 250 + 25 × mark from mark 1 (275 °F), halving below (½ = 250 °F, ¼ = 225 °F). From a gas
mark to °C the answer is the conventional UK cookery table (rounded to 10 °C, as printed by BBC
Good Food and reproduced in Wikipedia's "Gas mark": 4 = 180 °C) with the exact value from °F
beside it (176.667 °C). To a gas mark the answer is the nearest table entry (by °C or °F) with
the exact scale value.

The tests check NIST Handbook 44 (1 US tbsp = 14.7868 mL), the CFR legal cup (240 mL, 15 mL
tablespoon), the UK cup, gas mark 4 = 180 °C (UK table) and 350 °F, °C/°F, mass units and a
given density.

Limits: densities depend on how a cup is filled (sifted, spooned, packed), so weights from
volumes are approximate; no altitude or fan-oven adjustments.
