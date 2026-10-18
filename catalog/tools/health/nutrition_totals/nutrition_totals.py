#!/usr/bin/env python3
"""Totals of energy and nutrients for a recipe or a day's food, from a food table given in the
input (values per 100 g) and the grams of each item, per serving and as a percentage of the
reference intakes (default: Regulation (EU) No 1169/2011 Annex XIII, bundled in
reference_intakes.json).

Input (JSON on stdin):
  foods              {"name": {"nutrient": amount per 100 g, ...}, ...}
  items              [{"food": "name", "grams": 150}, ...]
  servings           integer, default 1
  reference_intakes  optional {"nutrient": amount, ...}: replaces the Annex XIII defaults
  decimals           optional, default 2 (amounts); percentages get 1

Nutrient keys are free; for the Annex XIII keys (energy_kj, energy_kcal, fat, saturates,
carbohydrate, sugars, protein, salt, vitamin_c, iron, ...) the food table must use the
reference intake's unit (g, mg, ug, kJ, kcal).
"""
import json
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, localcontext
from fractions import Fraction
from pathlib import Path

DISCLAIMER = ("Not medical or dietary advice: arithmetic on the food values given. Reference intakes are general "
              "adult labelling values, not personal requirements; ask a qualified clinician or dietitian.")
KEY = re.compile(r"^[a-z][a-z0-9_]{0,47}$")
DECIMAL = re.compile(r"\s*[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d{1,3})?\s*")


def fail(message):
    print(json.dumps({"error": message, "disclaimer": DISCLAIMER}))
    sys.exit(2)


def number(value, field, positive=False):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    if isinstance(value, str) and not DECIMAL.fullmatch(value):
        fail(f"{field} must be a number or a decimal string")
    try:
        result = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{field} must be a number or a decimal string")
    if abs(result) > 10 ** 12:
        fail(f"{field} is too large")
    if result < 0 or (positive and result == 0):
        fail(f"{field} must be {'positive' if positive else 'zero or more'}")
    return result


def text(value, places):
    with localcontext() as ctx:
        ctx.prec = 60
        d = Decimal(value.numerator) / Decimal(value.denominator)
        return str(d.quantize(Decimal(1).scaleb(-places), rounding=ROUND_HALF_UP))


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"foods", "items", "servings", "reference_intakes", "decimals"})
    if extra:
        fail(f"unknown field(s): {', '.join(extra)}")
    try:
        table = json.loads((Path(__file__).resolve().parent / "reference_intakes.json").read_text(encoding="utf-8"))
    except (OSError, ValueError):
        fail("reference_intakes.json is missing or unreadable")
    foods = request.get("foods")
    if not isinstance(foods, dict) or not foods:
        fail("foods must be an object of food names, each an object of nutrient amounts per 100 g")
    per100 = {}
    for name, nutrients in foods.items():
        if not isinstance(nutrients, dict) or not nutrients:
            fail(f"foods[{name!r}] must be an object of nutrient amounts per 100 g")
        row = {}
        for key, value in nutrients.items():
            if not KEY.match(key):
                fail(f"nutrient key {key!r} must be lowercase letters, digits and underscores")
            row[key] = number(value, f"foods[{name!r}].{key}")
        per100[name] = row
    items = request.get("items")
    if not isinstance(items, list) or not items:
        fail("items must be a non-empty list of {food, grams}")
    servings = request.get("servings", 1)
    if isinstance(servings, bool) or not isinstance(servings, int) or not 1 <= servings <= 1000:
        fail("servings must be an integer from 1 to 1000")
    decimals = request.get("decimals", 2)
    if isinstance(decimals, bool) or not isinstance(decimals, int) or not 0 <= decimals <= 6:
        fail("decimals must be an integer from 0 to 6")
    if request.get("reference_intakes") is not None:
        given = request["reference_intakes"]
        if not isinstance(given, dict) or not given:
            fail("reference_intakes must be an object of nutrient amounts")
        intakes = {k: {"amount": number(v, f"reference_intakes.{k}", positive=True), "unit": None} for k, v in given.items()}
        ri_source = "given in the input"
    else:
        intakes = {k: {"amount": Fraction(v["amount"]), "unit": v["unit"]} for k, v in table["intakes"].items()}
        ri_source = f"{table['title']}; {table['source_url']} (transcribed {table['transcribed']})"

    nutrients = sorted({k for row in per100.values() for k in row})
    totals = {k: Fraction(0) for k in nutrients}
    incomplete = {k: [] for k in nutrients}
    breakdown, grams_total = [], Fraction(0)
    for i, item in enumerate(items):
        if not isinstance(item, dict) or set(item) - {"food", "grams"}:
            fail(f"items[{i}] must be an object with food and grams")
        food = item.get("food")
        if food not in per100:
            fail(f"items[{i}].food {food!r} is not in foods")
        grams = number(item.get("grams"), f"items[{i}].grams", positive=True)
        grams_total += grams
        contribution = {}
        for k in nutrients:
            if k in per100[food]:
                amount = per100[food][k] * grams / 100
                totals[k] += amount
                contribution[k] = text(amount, decimals)
            elif food not in incomplete[k]:
                incomplete[k].append(food)
        breakdown.append({"food": food, "grams": text(grams, decimals), "nutrients": contribution})

    def percent(amounts):
        return {k: text(v * 100 / intakes[k]["amount"], 1) for k, v in amounts.items() if k in intakes}

    per_serving = {k: v / servings for k, v in totals.items()}
    units = {k: intakes[k]["unit"] for k in nutrients if k in intakes and intakes[k]["unit"]}
    result = {
        "total_grams": text(grams_total, decimals), "servings": servings,
        "grams_per_serving": text(grams_total / servings, decimals),
        "totals": {k: text(v, decimals) for k, v in totals.items()},
        "per_serving": {k: text(v, decimals) for k, v in per_serving.items()},
        "percent_reference_intake_per_serving": percent(per_serving),
        "percent_reference_intake_total": percent(totals),
        "units": units,
        "missing_values": {k: v for k, v in incomplete.items() if v},
        "items": breakdown,
        "reference_intakes": {k: text(v["amount"], 2).rstrip("0").rstrip(".") + (f" {v['unit']}" if v["unit"] else "")
                              for k, v in intakes.items() if k in nutrients},
        "reference_intakes_source": ri_source,
        "notes": ["A nutrient missing from some foods is totalled over the foods that have it (see missing_values)."],
        "disclaimer": DISCLAIMER,
    }
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # last resort: a usage error, never a traceback
        fail(f"invalid input ({type(error).__name__})")
