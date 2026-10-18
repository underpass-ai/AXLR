# nutrition_totals

Adds up energy and nutrients for a recipe or a day's food. The food table comes from the input
(`foods`: each food's amounts per 100 g, as on a label or from a food-composition database), and
`items` gives the grams of each food used. For each nutrient the tool sums `amount_per_100g ×
grams / 100` exactly (fractions), divides by `servings`, and gives the percentage of the
reference intake per serving and in total (amounts to `decimals` places, default 2, percentages
to one decimal, half up).

The default reference intakes are the adult values of **Regulation (EU) No 1169/2011, Annex
XIII**, bundled in `reference_intakes.json` (written by `tools/catalog/data/nutrition_totals.py`
from the EUR-Lex text, transcribed 18 Oct 2026; © European Union, reuse authorised with
acknowledgement): Part B energy 8,400 kJ / 2,000 kcal, total fat 70 g, saturates 20 g,
carbohydrate 260 g, sugars 90 g, protein 50 g, salt 6 g; Part A the 27 vitamin and mineral NRVs
(vitamin A 800 µg, vitamin C 80 mg, calcium 800 mg, iron 14 mg, iodine 150 µg, …). Keys:
`energy_kj`, `energy_kcal`, `fat`, `saturates`, `carbohydrate`, `sugars`, `protein`, `salt`,
`vitamin_a` … `iodine` (the output's `units` names each unit; µg is written `ug`). The food
table must use those units for those keys. `reference_intakes` in the input replaces the whole
default table (for another jurisdiction, a child, or a nutrient Annex XIII lacks, such as fibre).

A nutrient that some foods lack is totalled over the foods that have it, and `missing_values`
lists the foods without it, so an incomplete total is never silent.

The tests check that the Annex XIII Part B amounts each give 100 % and three Part A NRVs give
50 %, plus a porridge worked by hand (80 g oats and 250 g milk in two servings: 9.325 g protein
per serving, 18.7 % of 50 g).

Every output, errors included, carries the disclaimer: *Not medical or dietary advice:
arithmetic on the food values given. Reference intakes are general adult labelling values, not
personal requirements; ask a qualified clinician or dietitian.*

Limits: no food database; no energy computed from macronutrients (Annex XIV factors); no
cooking losses or yield; no label rounding rules (the EU guidance tolerances).
