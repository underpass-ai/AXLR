#!/usr/bin/env python3
"""Write catalog/tools/personal/cooking_unit_convert/cooking.json: the kitchen measures, oven
temperature table and ingredient weights the tool bundles.

1. Cup and spoon definitions (literal, exact):
   - US customary: NIST Handbook 44 (2024), Appendix C: gallon = 231 in^3 = 3.785411784 L;
     cup = 1/16 gal = 236.5882365 mL; fl oz = 1/128 gal; tablespoon = 1/2 fl oz =
     14.78676478125 mL; teaspoon = 1/3 tbsp; pint = 1/8 gal.
   - US "legal" (nutrition labelling): 21 CFR 101.9(b)(5)(viii): 1 cup = 240 mL, 1 tablespoon =
     15 mL, 1 teaspoon = 5 mL, 1 fl oz = 30 mL.  https://www.ecfr.gov/current/title-21/section-101.9
   - metric (Australia, Canada, New Zealand, South Africa): cup 250 mL, tablespoon 15 mL (20 mL in
     Australia, available as the unit au_tbsp), teaspoon 5 mL.
   - UK: cup = 1/2 imperial pint = 284.130625 mL; imperial fl oz = 1/160 imperial gallon of
     4.54609 L (NIST SP 811 App. B.8; Weights and Measures Act 1985, Sch. 1); spoons 15 and 5 mL.
2. Gas marks: the conventional UK table of oven temperatures (gas mark 1/4 to 10 in degrees F
   and rounded degrees C) as printed by BBC Good Food and The Guardian (2007) and reproduced in the
   English Wikipedia article "Gas mark" (retrieved 2026-10-18); the underlying scale is
   degF = 250 + 25 G for G >= 1, halving below 1 (1/2 = 250 F, 1/4 = 225 F).
3. Ingredient weights per US cup (and per tablespoon/teaspoon where USDA gives them): USDA
   FoodData Central, SR Legacy dataset (April 2018), household-measure portions ("foodPortions")
   of each food, fetched once with the FoodData Central API (POST /fdc/v1/foods, DEMO_KEY).
   FoodData Central data are in the public domain (CC0 1.0): https://fdc.nal.usda.gov/
   Each ingredient records its FDC id, description, portion text and grams. USDA's household cup
   is taken as the US customary cup (236.5882365 mL) when a density in g/mL is derived.

Licences: NIST Handbook 44, NIST SP 811 and the CFR are works of the US Government, public domain
(17 U.S.C. 105); USDA FoodData Central is public domain (CC0 1.0); the gas mark table is a list of
conventional facts (no text of the cited pages is copied).

This generator uses the network; the tool never does.

Usage: python3 tools/catalog/data/cooking_unit_convert.py [--cache DIR]
  --cache DIR  keep the raw API response in DIR and reuse it on a rerun (DEMO_KEY is rate limited)
"""
import datetime as dt
import json
import pathlib
import sys
import urllib.request
from fractions import Fraction

API = "https://api.nal.usda.gov/fdc/v1/foods?api_key=DEMO_KEY"
US_CUP_ML = "236.5882365"
SYSTEMS = {
    "us_customary": {"cup": "236.5882365", "tbsp": "14.78676478125", "tsp": "4.92892159375", "fl_oz": "29.5735295625",
                     "pint": "473.176473", "source": "NIST Handbook 44 (2024), Appendix C (US gallon = 231 in^3)"},
    "us_legal": {"cup": "240", "tbsp": "15", "tsp": "5", "fl_oz": "30", "pint": "480",
                 "source": "21 CFR 101.9(b)(5)(viii) (US nutrition labelling)"},
    "metric": {"cup": "250", "tbsp": "15", "tsp": "5", "fl_oz": "29.5735295625", "pint": "500",
               "source": "metric cup of Australia, Canada, New Zealand and South Africa; fl oz is the US one, pint 500 mL"},
    "uk": {"cup": "284.130625", "tbsp": "15", "tsp": "5", "fl_oz": "28.4130625", "pint": "568.26125",
           "source": "half imperial pint; imperial gallon 4.54609 L (NIST SP 811 App. B.8, Weights and Measures Act 1985); metric spoons"},
}
GAS_MARKS = [  # gas mark, degF, conventional degC, description
    ("1/4", 225, 110, "very slow / very cool"), ("1/2", 250, 120, "very slow / very cool"),
    ("1", 275, 140, "slow / cool"), ("2", 300, 150, "slow / cool"), ("3", 325, 170, "moderately slow / warm"),
    ("4", 350, 180, "moderate"), ("5", 375, 190, "moderately hot"), ("6", 400, 200, "moderately hot"),
    ("7", 425, 220, "hot"), ("8", 450, 230, "very hot"), ("9", 475, 240, "very hot"), ("10", 500, 260, "extremely hot"),
]
# key, aliases, FDC id, expected start of the USDA description
INGREDIENTS = [
    ("all-purpose flour", ["flour", "plain flour", "ap flour"], 168894, "Wheat flour, white, all-purpose, enriched, bleached"),
    ("bread flour", ["strong flour"], 168896, "Wheat flour, white, bread, enriched"),
    ("whole wheat flour", ["wholemeal flour"], 168944, "Wheat flour, whole-grain"),
    ("granulated sugar", ["sugar", "white sugar", "caster sugar"], 169655, "Sugars, granulated"),
    ("brown sugar", ["light brown sugar"], 168833, "Sugars, brown"),
    ("powdered sugar", ["icing sugar", "confectioners sugar"], 169656, "Sugars, powdered"),
    ("butter", ["salted butter"], 173410, "Butter, salted"),
    ("whole milk", ["milk"], 171265, "Milk, whole, 3.25% milkfat"),
    ("heavy cream", ["double cream", "whipping cream"], 170859, "Cream, fluid, heavy whipping"),
    ("honey", [], 169640, "Honey"),
    ("maple syrup", [], 169661, "Syrups, maple"),
    ("olive oil", [], 171413, "Oil, olive, salad or cooking"),
    ("vegetable oil", ["canola oil", "rapeseed oil", "oil"], 172336, "Oil, canola"),
    ("table salt", ["salt"], 173468, "Salt, table"),
    ("baking soda", ["bicarbonate of soda", "bicarb"], 175040, "Leavening agents, baking soda"),
    ("baking powder", [], 172805, "Leavening agents, baking powder"),
    ("cocoa powder", ["cocoa"], 169593, "Cocoa, dry powder, unsweetened"),
    ("rolled oats", ["oats", "oatmeal"], 173904, "Cereals, oats, regular and quick, not fortified, dry"),
    ("white rice", ["rice", "long-grain rice"], 169756, "Rice, white, long-grain, regular, raw, enriched"),
    ("cornstarch", ["corn starch", "cornflour (uk)"], 169698, "Cornstarch"),
]
# water is not taken from USDA: the gram was first defined (1795) as the mass of 1 cm3 of water near
# 4 degC, its maximum density (0.99997 g/mL); at 20 degC water is 0.9982 g/mL. 1 g/mL is used.
WATER = {"aliases": ["tap water"], "fdc_id": None, "description": "water (by definition, about 1 g/mL)",
         "grams_per_us_cup": "236.5882365", "density_g_per_ml": "1.000000", "density_basis": "1 mL = 1 g (water near 4 degC)"}


def fetch(ids, cache):
    if cache:
        path = cache / "fdc_foods.json"
        if path.exists():
            data = json.loads(path.read_text())
            if all(any(f.get("fdcId") == i for f in data) for i in ids):
                return data
    body = json.dumps({"fdcIds": ids, "format": "full"}).encode()
    request = urllib.request.Request(API, data=body, headers={"Content-Type": "application/json",
                                                               "User-Agent": "AXLR-catalog data generator"})
    with urllib.request.urlopen(request, timeout=120) as response:
        data = json.loads(response.read().decode("utf-8"))
    if cache:
        (cache / "fdc_foods.json").write_text(json.dumps(data))
    return data


def portion(food, unit_names):
    for p in food.get("foodPortions", []):
        name = (p.get("measureUnit") or {}).get("name", "") or ""
        modifier = (p.get("modifier") or "").strip().lower()
        text = name.lower() if name and name.lower() != "undetermined" else modifier
        if text in unit_names and float(p.get("amount") or 1) == 1:
            return {"text": f"1 {modifier or name}".strip(), "grams": format(p["gramWeight"], "g")}
    return None


def main():
    cache = None
    if len(sys.argv) == 3 and sys.argv[1] == "--cache":
        cache = pathlib.Path(sys.argv[2])
        cache.mkdir(parents=True, exist_ok=True)
    table = list(INGREDIENTS)
    ids = [row[2] for row in table]
    foods, fetch_error = {}, None
    try:
        for start in range(0, len(ids), 20):
            for food in fetch(ids[start:start + 20], cache if start == 0 else None):
                foods[food["fdcId"]] = food
    except Exception as error:  # ship the literal tables; the file says what was not fetched
        fetch_error = f"{type(error).__name__}: {error}"
    ingredients, skipped = {"water": WATER}, {}
    for key, aliases, fdc_id, expected in table:
        food = foods.get(fdc_id)
        if food is None or not food.get("description", "").startswith(expected):
            skipped[key] = (f"FDC {fdc_id}: API request failed ({fetch_error})" if fetch_error else
                            f"FDC {fdc_id}: {food.get('description') if food else 'not returned'}")
            continue
        cup = portion(food, {"cup", "cup, packed", "cup, unsifted"})
        tbsp = portion(food, {"tbsp", "tablespoon"})
        tsp = portion(food, {"tsp", "teaspoon"})
        if cup is None and tbsp is None and tsp is None:
            skipped[key] = f"FDC {fdc_id}: no cup, tablespoon or teaspoon portion"
            continue
        grams_per_cup = Fraction(cup["grams"]) if cup else Fraction(tbsp["grams"]) * 16 if tbsp else Fraction(tsp["grams"]) * 48
        density = grams_per_cup / Fraction(US_CUP_ML)
        ingredients[key] = {"aliases": aliases, "fdc_id": fdc_id, "description": food["description"],
                            "publication_date": food.get("publicationDate"),
                            "cup": cup, "tbsp": tbsp, "tsp": tsp,
                            "grams_per_us_cup": format(float(grams_per_cup), "g"),
                            "density_g_per_ml": format(float(density), ".6f"),
                            "density_basis": "1 cup" if cup else "1 tbsp x 16" if tbsp else "1 tsp x 48"}
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/personal/cooking_unit_convert/cooking.json"
    out.parent.mkdir(parents=True, exist_ok=True)
    payload = {"generated": dt.date.today().isoformat(), "generator": "tools/catalog/data/cooking_unit_convert.py",
               "volume_systems": SYSTEMS,
               "gas_marks": [{"gas_mark": g, "fahrenheit": f, "celsius": c, "description": d} for g, f, c, d in GAS_MARKS],
               "gas_mark_source": "conventional UK table (BBC Good Food; The Guardian, 2007) as reproduced in Wikipedia, 'Gas mark', retrieved 2026-10-18",
               "ingredients_source": "USDA FoodData Central, SR Legacy (2018), household-measure portions; public domain (CC0 1.0); fetched with the FDC API",
               "licence": "NIST and CFR: US Government works, public domain; USDA FoodData Central: CC0 1.0; gas marks: factual values",
               "ingredients": ingredients, "not_fetched": skipped}
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n")
    print(f"{out}: {len(ingredients)} ingredients")
    for key, why in skipped.items():
        print(f"  skipped {key}: {why}")


if __name__ == "__main__":
    main()
