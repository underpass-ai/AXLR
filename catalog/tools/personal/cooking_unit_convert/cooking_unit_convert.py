#!/usr/bin/env python3
"""Kitchen conversions: cups, tablespoons, teaspoons, fluid ounces, pints and millilitres under a
stated cup definition (US customary, US legal, metric, UK); grams, ounces and pounds; volume to
weight and back through an ingredient's density (bundled USDA table or given); oven temperatures in
degrees Celsius, Fahrenheit and gas marks.

Input (JSON on stdin):
  value       number or string: 2, "1.5", "3/4", "1 1/2", "½"
  from, to    units: ml, l, dl, cl, cup, tbsp, tsp, fl_oz, pint (in `system`), or explicitly
              us_cup, legal_cup, metric_cup, uk_cup, us_tbsp, us_tsp, au_tbsp, us_fl_oz, uk_fl_oz,
              us_pint, uk_pint; g, kg, mg, oz, lb; C, F, gas_mark
  system      "us_customary" (default) | "us_legal" | "metric" | "uk"
  ingredient  name in the bundled table (volume <-> weight)    or
  density     grams per millilitre (volume <-> weight)
  digits      significant digits of the decimal result (default 6)
  list        true: list the ingredients, cup systems and gas marks
"""
import json
import re
import sys
from decimal import Decimal, getcontext
from fractions import Fraction

getcontext().prec = 40
UNICODE = {"½": Fraction(1, 2), "⅓": Fraction(1, 3), "⅔": Fraction(2, 3), "¼": Fraction(1, 4), "¾": Fraction(3, 4),
           "⅛": Fraction(1, 8), "⅜": Fraction(3, 8), "⅝": Fraction(5, 8), "⅞": Fraction(7, 8)}
MASS = {"g": Fraction(1), "kg": Fraction(1000), "mg": Fraction(1, 1000), "oz": Fraction("28.349523125"),
        "lb": Fraction("453.59237")}
MASS_ALIASES = {"gram": "g", "grams": "g", "kilogram": "kg", "kilograms": "kg", "kilo": "kg", "milligram": "mg",
                "ounce": "oz", "ounces": "oz", "pound": "lb", "pounds": "lb", "lbs": "lb"}
METRIC_VOLUME = {"ml": Fraction(1), "cl": Fraction(10), "dl": Fraction(100), "l": Fraction(1000)}
VOLUME_ALIASES = {"millilitre": "ml", "milliliter": "ml", "millilitres": "ml", "milliliters": "ml", "litre": "l", "liter": "l",
                  "litres": "l", "liters": "l", "cups": "cup", "c": "cup", "tablespoon": "tbsp", "tablespoons": "tbsp",
                  "tbs": "tbsp", "teaspoon": "tsp", "teaspoons": "tsp", "fl oz": "fl_oz", "floz": "fl_oz",
                  "fluid ounce": "fl_oz", "fluid ounces": "fl_oz", "pints": "pint", "pt": "pint"}
TEMPS = {"c": "C", "°c": "C", "degc": "C", "celsius": "C", "f": "F", "°f": "F", "degf": "F", "fahrenheit": "F",
         "gas_mark": "gas_mark", "gas mark": "gas_mark", "gas": "gas_mark", "gasmark": "gas_mark", "regulo": "gas_mark"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load():
    try:
        with open("cooking.json", encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, ValueError) as error:
        fail(f"cannot read the bundled cooking.json: {error}")


def number(value, field):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a string such as \"1 1/2\"")
    text = str(value).strip()
    try:
        m = re.fullmatch(r"(-?\d+)\s+(\d+)/(\d+)", text)
        if m:
            return int(m.group(1)) + Fraction(int(m.group(2)), int(m.group(3)))
        m = re.fullmatch(r"(\d*)\s*([½⅓⅔¼¾⅛⅜⅝⅞])", text)
        if m:
            return (int(m.group(1)) if m.group(1) else 0) + UNICODE[m.group(2)]
        return Fraction(text)
    except (ValueError, ZeroDivisionError):
        fail(f"{field}: cannot read {value!r} as a number")


def dec(x, digits):
    d = Decimal(x.numerator) / Decimal(x.denominator)
    if d == 0:
        return "0"
    text = format(d, f".{digits}g")
    if "e" in text:
        text = format(Decimal(text), "f")
    if "." in text:
        text = text.rstrip("0").rstrip(".")
    return text


def frac(x):
    return str(x.numerator) if x.denominator == 1 else f"{x.numerator}/{x.denominator}"


def unit(name, systems, system, field):
    """('volume', mL per unit, label) | ('mass', g per unit, label) | ('temp', kind, label)."""
    if not isinstance(name, str) or not name.strip():
        fail(f"{field} must be a unit name")
    key = name.strip()
    low = key.lower()
    if low in TEMPS or key in ("°C", "°F"):
        return "temp", TEMPS.get(low, key[-1]), TEMPS.get(low, key[-1])
    low = MASS_ALIASES.get(low, low)
    if low in MASS:
        return "mass", MASS[low], low
    low = VOLUME_ALIASES.get(low, low)
    if low in METRIC_VOLUME:
        return "volume", METRIC_VOLUME[low], low
    explicit = {"us_cup": ("us_customary", "cup"), "legal_cup": ("us_legal", "cup"), "metric_cup": ("metric", "cup"),
                "uk_cup": ("uk", "cup"), "us_tbsp": ("us_customary", "tbsp"), "us_tsp": ("us_customary", "tsp"),
                "us_fl_oz": ("us_customary", "fl_oz"), "uk_fl_oz": ("uk", "fl_oz"), "us_pint": ("us_customary", "pint"),
                "uk_pint": ("uk", "pint"), "legal_tbsp": ("us_legal", "tbsp"), "legal_tsp": ("us_legal", "tsp")}
    if low == "au_tbsp":
        return "volume", Fraction(20), "au_tbsp (20 mL)"
    if low in explicit:
        sysname, base = explicit[low]
        return "volume", Fraction(systems[sysname][base]), f"{base} ({sysname})"
    if low in ("cup", "tbsp", "tsp", "fl_oz", "pint"):
        return "volume", Fraction(systems[system][low]), f"{low} ({system})"
    fail(f"{field}: unknown unit {name!r}")


def find_ingredient(table, name):
    if not isinstance(name, str) or not name.strip():
        fail("ingredient must be a name")
    key = name.strip().lower()
    for k, info in table["ingredients"].items():
        if key == k or key in info.get("aliases", []):
            return k, info
    for k, info in table["ingredients"].items():
        if key in k:
            return k, info
    if any(key in k for k in table.get("not_fetched", {})):
        fail(f"ingredient {name!r} is listed for the table but its USDA data were not fetched in this version; give density in g/mL")
    fail(f"ingredient {name!r} is not in the bundled table ({', '.join(sorted(table['ingredients']))}); give density in g/mL instead")


def gas_rows(table):
    return [(Fraction(r["gas_mark"]), r) for r in table["gas_marks"]]


def to_fahrenheit_from_gas(g):
    if g >= 1:
        return 250 + 25 * g
    # below gas mark 1 the scale halves for each 25 degF step: 1/2 = 250 F, 1/4 = 225 F
    steps = 0
    x = g
    while x < 1:
        x *= 2
        steps += 1
    if x != 1:
        fail("gas marks below 1 are 1/2 and 1/4 (and 1/8)")
    return 275 - 25 * steps


def temperature(value, src, dst, table, digits):
    result = {"from": src, "to": dst, "value": dec(value, digits)}
    if src == "gas_mark":
        f = Fraction(to_fahrenheit_from_gas(value))
        row = next((r for g, r in gas_rows(table) if g == value), None)
        c_exact = (f - 32) * Fraction(5, 9)
        if dst == "F":
            result.update({"result": dec(f, digits), "exact": frac(f)})
        elif dst == "C":
            if row:
                result.update({"result": str(row["celsius"]), "convention": "UK cookery table (rounded to 10 degC)",
                               "description": row["description"]})
            else:
                result["result"] = dec(c_exact, 4)
            result["exact_celsius_from_fahrenheit"] = dec(c_exact, digits)
        else:
            result.update({"result": dec(value, digits)})
        result["fahrenheit"] = dec(f, digits)
        result["formula"] = "degF = 250 + 25 x gas mark (gas mark >= 1); 1/2 = 250 degF, 1/4 = 225 degF"
        result["table_source"] = table["gas_mark_source"]
        return result
    c = value if src == "C" else (value - 32) * Fraction(5, 9)
    f = value if src == "F" else value * Fraction(9, 5) + 32
    if dst in ("C", "F"):
        out = c if dst == "C" else f
        result.update({"result": dec(out, digits), "exact": frac(out), "formula": "degF = degC x 9/5 + 32"})
        return result
    # to gas mark
    if f >= 275:
        exact = (f - 250) / 25
    else:  # below gas mark 1 each 25 degF step halves the mark
        steps = (f - 275) / 25
        exact = Fraction(2) ** int(steps) if steps.denominator == 1 else Fraction(2 ** float(steps))
    rows = gas_rows(table)
    if src == "C":
        g, row = min(rows, key=lambda gr: (abs(gr[1]["celsius"] - c), gr[0]))
        basis = "nearest by the UK table's Celsius values"
    else:
        g, row = min(rows, key=lambda gr: (abs(gr[1]["fahrenheit"] - f), gr[0]))
        basis = "nearest by degF"
    result.update({"result": row["gas_mark"], "basis": basis, "description": row["description"],
                   "fahrenheit": dec(f, digits), "celsius": dec(c, digits),
                   "exact_gas_mark": dec(exact, 4) if exact is not None else None,
                   "table_source": table["gas_mark_source"]})
    if f < 200 or f > 525:
        result["warning"] = "outside the gas mark scale (1/4 to 10)"
    return result


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"value", "from", "to", "system", "ingredient", "density", "digits", "list"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    table = load()
    systems = table["volume_systems"]
    if request.get("list"):
        print(json.dumps({"volume_systems": systems, "gas_marks": table["gas_marks"],
                          "ingredients": {k: {"aliases": v.get("aliases", []), "grams_per_us_cup": v["grams_per_us_cup"],
                                              "density_g_per_ml": v["density_g_per_ml"], "fdc_id": v.get("fdc_id"),
                                              "usda_description": v.get("description")} for k, v in table["ingredients"].items()},
                          "ingredients_source": table["ingredients_source"], "generated": table["generated"]}, ensure_ascii=False))
        return
    system = request.get("system", "us_customary")
    if system not in systems:
        fail(f"system must be one of {', '.join(systems)}")
    digits = request.get("digits", 6)
    if isinstance(digits, bool) or not isinstance(digits, int) or not 1 <= digits <= 30:
        fail("digits must be an integer from 1 to 30")
    if "value" not in request or "from" not in request or "to" not in request:
        fail("give value, from and to (or list: true)")
    value = number(request["value"], "value")
    kind_a, fa, la = unit(request["from"], systems, system, "from")
    kind_b, fb, lb = unit(request["to"], systems, system, "to")
    if (kind_a == "temp") != (kind_b == "temp"):
        fail("temperatures convert only to temperatures")
    if kind_a == "temp":
        if fa == "gas_mark" and value <= 0:
            fail("gas mark must be positive")
        print(json.dumps(temperature(value, fa, fb, table, digits), ensure_ascii=False))
        return
    if value < 0:
        fail("value must not be negative")
    result = {"value": dec(value, digits), "from": la, "to": lb, "system": system}
    definitions = {}
    for label, kind, factor in ((la, kind_a, fa), (lb, kind_b, fb)):
        definitions[label] = f"{dec(factor, 15)} {'mL' if kind == 'volume' else 'g'}"
    result["definitions"] = definitions
    if any(k == "volume" and "(" + system + ")" in lab for k, lab in ((kind_a, la), (kind_b, lb))):
        result["system_source"] = systems[system]["source"]
    if kind_a == kind_b:
        out = value * fa / fb
        result.update({"result": dec(out, digits), "exact": frac(out), "factor": frac(fa / fb)})
        print(json.dumps(result, ensure_ascii=False))
        return
    # volume <-> mass through a density in g/mL
    if "density" in request and "ingredient" in request:
        fail("give ingredient or density, not both")
    if "density" in request:
        density = number(request["density"], "density")
        if density <= 0:
            fail("density must be positive")
        result["density_g_per_ml"] = dec(density, digits)
        result["density_source"] = "given in the input"
    elif "ingredient" in request:
        name, info = find_ingredient(table, request["ingredient"])
        density = Fraction(info["grams_per_us_cup"]) / Fraction(systems["us_customary"]["cup"])
        result["ingredient"] = name
        result["density_g_per_ml"] = info["density_g_per_ml"]
        result["grams_per_us_cup"] = info["grams_per_us_cup"]
        source = {"basis": info["density_basis"]}
        if info.get("fdc_id"):
            source.update({"usda": info["description"], "fdc_id": info["fdc_id"], "dataset": table["ingredients_source"]})
        else:
            source["note"] = info["description"]
        result["density_source"] = source
        result["note"] = "household measures vary with how the cup is filled (sifted, spooned, packed): treat as approximate"
    else:
        fail("converting between volume and weight needs ingredient or density (g/mL)")
    ml = value * fa if kind_a == "volume" else value * fa / density
    out = ml * density / fb if kind_b == "mass" else ml / fb
    result.update({"result": dec(out, digits), "exact": None, "millilitres": dec(ml, digits)})
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
