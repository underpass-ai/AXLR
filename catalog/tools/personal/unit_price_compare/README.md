# unit_price_compare

Which offer is cheapest per gram, per litre or per item, when the packs come in different sizes,
units, multipacks and promotions.

For each offer the tool works out what you pay and what you get, in `Decimal`:

- the pack price, less `discount_pct` and then `discount_amount` (both per pack);
- the deal: `"3 for 2"` (and `{"buy": 3, "pay_for": 2}`) means you take 3 packs and pay for 2;
  `"buy 2 get 1 free"` is the same thing; `"bogo"` is 2 for 1; `"2 for 5.00"` (and
  `{"buy": 2, "total_price": "5.00"}`) means 2 packs for a total of 5.00. In the string form
  `"N for M"`, M is a number of packs only when it is a whole number smaller than N with no
  decimal point or currency sign; otherwise it is a total price;
- the quantity: `size × count × packs bought`, converted to grams, millilitres or items with the
  exact factors of the bundled `units.json`;
- the unit price `spend / quantity`, expressed `per` a unit you choose (`"kg"`, `"100 g"`,
  `"l"`, `"fl_oz"`; by default kg, l or one item).

Offers are ranked by unit price. Each one shows how much dearer it is than the best, in percent
and per unit, and `savings_choosing_best_for_same_quantity`: what the best offer's quantity
would cost at this offer's unit price, minus what the best offer costs. Ties are listed.

The units (`list_units: true` lists them) are written by `tools/catalog/data/unit_price_compare.py`
from NIST SP 811 Appendix B.8 and NIST Handbook 44 Appendix C: the avoirdupois pound is
453.59237 g and the ounce 28.349523125 g exactly; the US gallon is 231 in³ = 3.785411784 L, so
the US fluid ounce is 29.5735295625 mL; the imperial gallon is 4.54609 L and the imperial fluid
ounce 28.4130625 mL. `oz` is always the mass ounce, `fl_oz` the US fluid ounce and `uk_fl_oz`
the imperial one.

The tests check hand-worked comparisons (500 g at 2.50 against 1 kg at 4.50 and a 3 × 200 g
multipack; 3 for 2; 2 for 5.00; buy 2 get 1 free) and the NIST factors (2.00 a pound is
4.409245 a kilogram; 16 US fl oz is 473.176473 mL).

Limits: mass, volume and counts cannot be compared with each other (no densities); a deal is
assumed to be taken in full (you buy all N packs); taxes, deposits and loyalty prices are not
modelled; money in `pay` is rounded to two decimals, unit prices to `places`.
