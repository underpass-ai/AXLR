#!/usr/bin/env python3
"""Write catalog/tools/health/nutrition_totals/reference_intakes.json: the default reference
intakes the tool bundles, transcribed on 18 Oct 2026 from

  Regulation (EU) No 1169/2011 of the European Parliament and of the Council of 25 October 2011
  on the provision of food information to consumers, Annex XIII "Reference intakes":
    Part A, point 1: daily reference intakes (nutrient reference values, NRV) for vitamins and
                     minerals (adults);
    Part B: reference intakes for energy and selected nutrients other than vitamins and minerals
            (adults).
  Source: EUR-Lex, CELEX 32011R1169, https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:32011R1169
  Licence: (c) European Union, https://eur-lex.europa.eu; reuse is authorised provided the source
  is acknowledged (Commission Decision 2011/833/EU on the reuse of Commission documents).

The values are literal (no network). An amendment of Annex XIII is a new table version and a new
version of the tool.

Usage: python3 tools/catalog/data/nutrition_totals.py
"""
import json
import pathlib

SOURCE_URL = "https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:32011R1169"
LICENCE = "(c) European Union; reuse authorised with acknowledgement of the source (Commission Decision 2011/833/EU)"
TRANSCRIBED = "2026-10-18"

PART_B = [  # key, label, amount, unit
    ("energy_kj", "Energy", "8400", "kJ"),
    ("energy_kcal", "Energy", "2000", "kcal"),
    ("fat", "Total fat", "70", "g"),
    ("saturates", "Saturates", "20", "g"),
    ("carbohydrate", "Carbohydrate", "260", "g"),
    ("sugars", "Sugars", "90", "g"),
    ("protein", "Protein", "50", "g"),
    ("salt", "Salt", "6", "g"),
]
PART_A = [
    ("vitamin_a", "Vitamin A", "800", "ug"),
    ("vitamin_d", "Vitamin D", "5", "ug"),
    ("vitamin_e", "Vitamin E", "12", "mg"),
    ("vitamin_k", "Vitamin K", "75", "ug"),
    ("vitamin_c", "Vitamin C", "80", "mg"),
    ("thiamin", "Thiamin", "1.1", "mg"),
    ("riboflavin", "Riboflavin", "1.4", "mg"),
    ("niacin", "Niacin", "16", "mg"),
    ("vitamin_b6", "Vitamin B6", "1.4", "mg"),
    ("folic_acid", "Folic acid", "200", "ug"),
    ("vitamin_b12", "Vitamin B12", "2.5", "ug"),
    ("biotin", "Biotin", "50", "ug"),
    ("pantothenic_acid", "Pantothenic acid", "6", "mg"),
    ("potassium", "Potassium", "2000", "mg"),
    ("chloride", "Chloride", "800", "mg"),
    ("calcium", "Calcium", "800", "mg"),
    ("phosphorus", "Phosphorus", "700", "mg"),
    ("magnesium", "Magnesium", "375", "mg"),
    ("iron", "Iron", "14", "mg"),
    ("zinc", "Zinc", "10", "mg"),
    ("copper", "Copper", "1", "mg"),
    ("manganese", "Manganese", "2", "mg"),
    ("fluoride", "Fluoride", "3.5", "mg"),
    ("selenium", "Selenium", "55", "ug"),
    ("chromium", "Chromium", "40", "ug"),
    ("molybdenum", "Molybdenum", "50", "ug"),
    ("iodine", "Iodine", "150", "ug"),
]


def main():
    table = {
        "title": "Reference intakes, Regulation (EU) No 1169/2011, Annex XIII (adults)",
        "source_url": SOURCE_URL,
        "licence": LICENCE,
        "transcribed": TRANSCRIBED,
        "generator": "tools/catalog/data/nutrition_totals.py",
        "intakes": {},
    }
    for part, rows in (("Annex XIII Part B", PART_B), ("Annex XIII Part A point 1", PART_A)):
        for key, label, amount, unit in rows:
            table["intakes"][key] = {"label": label, "amount": amount, "unit": unit, "part": part}
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/health/nutrition_totals/reference_intakes.json"
    out.write_text(json.dumps(table, indent=1, ensure_ascii=False) + "\n")
    print(f"wrote {out} ({len(table['intakes'])} reference intakes)")


if __name__ == "__main__":
    main()
