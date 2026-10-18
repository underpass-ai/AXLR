#!/usr/bin/env python3
"""Write catalog/tools/personal/unit_price_compare/units.json: the package-size units the tool
bundles, each as an exact decimal multiple of its family's base unit (gram, millilitre, item),
transcribed on 18 Oct 2026 (literal values, no network) from

  - NIST Handbook 133 / NIST SP 811 (2008), Appendix B.8: the avoirdupois pound is 0.45359237 kg
    and the ounce 1/16 lb = 28.349523125 g exactly (1959 international yard and pound agreement);
  - NIST Handbook 44 (2024), Appendix C: the US gallon is 231 in^3 = 3.785411784 L, the US fluid
    ounce 1/128 gal = 29.5735295625 mL, the US pint 1/8 gal and quart 1/4 gal;
  - NIST SP 811 Appendix B.8: the imperial gallon is 4.54609 L, the imperial fluid ounce 1/160
    imperial gallon = 28.4130625 mL, the imperial pint 1/8 imperial gallon;
  - BIPM SI Brochure, 9th ed. (2019), Tables 7 and 8: SI prefixes and the litre (1 dm^3).
    https://www.nist.gov/pml/special-publication-811

Licence: NIST publications are works of the US Government, in the public domain in the United
States (17 U.S.C. 105); the BIPM SI Brochure is (c) BIPM, CC BY 3.0 IGO. Only numerical
definitions are copied.

A corrected definition is a new table version and a new version of the tool.

Usage: python3 tools/catalog/data/unit_price_compare.py
"""
import json
import pathlib
from decimal import Decimal as D

LB = D("453.59237")          # g
US_GAL = D("3785.411784")    # mL (231 in3 x 16.387064 mL/in3)
IMP_GAL = D("4546.09")       # mL

# symbol, aliases, family, factor to the base unit, definition, source
UNITS = [
    ("mg", ["milligram", "milligrams"], "mass", D("0.001"), "10^-3 g", "SI Brochure Table 7"),
    ("g", ["gram", "grams"], "mass", D(1), "base unit of this table", "SI Brochure Table 2"),
    ("kg", ["kilogram", "kilograms", "kilo", "kilos"], "mass", D(1000), "1000 g", "SI Brochure Table 2"),
    ("oz", ["ounce", "ounces", "oz_av"], "mass", LB / 16, "1/16 lb = 28.349523125 g", "NIST SP 811 App. B.8"),
    ("lb", ["lbs", "pound", "pounds"], "mass", LB, "0.45359237 kg exactly", "NIST SP 811 App. B.8 (1959 agreement)"),
    ("ml", ["mL", "millilitre", "milliliter", "millilitres", "milliliters"], "volume", D(1), "base unit of this table", "SI Brochure Table 8"),
    ("cl", ["cL", "centilitre", "centiliter"], "volume", D(10), "10 mL", "SI Brochure Tables 7, 8"),
    ("dl", ["dL", "decilitre", "deciliter"], "volume", D(100), "100 mL", "SI Brochure Tables 7, 8"),
    ("l", ["L", "litre", "liter", "litres", "liters"], "volume", D(1000), "1 dm^3 = 1000 mL", "SI Brochure Table 8"),
    ("fl_oz", ["floz", "fl oz", "us_fl_oz", "fluid ounce", "fluid ounces"], "volume", US_GAL / 128, "US fluid ounce, 1/128 US gal = 29.5735295625 mL", "NIST Handbook 44 App. C"),
    ("pt", ["pint", "us_pint"], "volume", US_GAL / 8, "US liquid pint, 1/8 US gal", "NIST Handbook 44 App. C"),
    ("qt", ["quart", "us_quart"], "volume", US_GAL / 4, "US liquid quart, 1/4 US gal", "NIST Handbook 44 App. C"),
    ("gal", ["gallon", "us_gal"], "volume", US_GAL, "US gallon, 231 in^3 = 3.785411784 L", "NIST Handbook 44 App. C"),
    ("uk_fl_oz", ["imp_fl_oz", "imperial fluid ounce"], "volume", IMP_GAL / 160, "imperial fluid ounce, 1/160 imp gal = 28.4130625 mL", "NIST SP 811 App. B.8"),
    ("uk_pint", ["imp_pint", "imperial pint"], "volume", IMP_GAL / 8, "imperial pint, 1/8 imp gal = 568.26125 mL", "NIST SP 811 App. B.8"),
    ("uk_gal", ["imp_gal", "imperial gallon"], "volume", IMP_GAL, "imperial gallon, 4.54609 L exactly", "NIST SP 811 App. B.8"),
    ("count", ["each", "ea", "item", "items", "unit", "units", "pc", "pcs", "piece", "pieces", "ct"], "count", D(1), "one item", "counting"),
    ("dozen", ["doz"], "count", D(12), "12 items", "counting"),
]

FAMILIES = {"mass": "g", "volume": "ml", "count": "count"}


def main():
    units = {}
    for symbol, aliases, family, factor, definition, source in UNITS:
        units[symbol] = {"family": family, "factor": format(factor.normalize(), "f"), "aliases": aliases,
                         "definition": definition, "source": source}
    assert units["oz"]["factor"] == "28.349523125" and units["fl_oz"]["factor"] == "29.5735295625"
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/personal/unit_price_compare/units.json"
    payload = {"generated": "2026-10-18", "generator": "tools/catalog/data/unit_price_compare.py",
               "source": "NIST SP 811 (2008) App. B.8; NIST Handbook 44 (2024) App. C; BIPM SI Brochure 9th ed.",
               "licence": "NIST: US Government work, public domain (17 U.S.C. 105); SI Brochure: CC BY 3.0 IGO; numerical definitions only",
               "base_units": FAMILIES, "units": units}
    out.write_text(json.dumps(payload, indent=1) + "\n")
    print(f"{out}: {len(units)} units")


if __name__ == "__main__":
    main()
