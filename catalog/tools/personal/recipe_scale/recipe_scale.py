#!/usr/bin/env python3
"""Scale a recipe's ingredient list by servings, by a factor, or to a target amount of one
ingredient, with exact fractions and kitchen-friendly rounding.

Input (JSON on stdin):
  ingredients   ["1 1/2 cups flour", "2-3 tbsp sugar", "3 eggs", "½ tsp salt", "salt to taste", ...]
                or objects {"quantity": "1 1/2", "unit": "cup", "name": "flour"}
  servings      {"from": 4, "to": 6}                  one of servings, factor, target
  factor        "3/2", 1.5 or "150%"
  target        {"ingredient": "flour", "quantity": "3", "unit": "cups"}  (unit optional: the same as
                the recipe's; volume to volume and mass to mass convert with US customary/SI factors)
  round         true (default): cups to 1/4 or 1/3, tbsp to 1/2, tsp to 1/8, g and ml whole, eggs whole
  convert_units true (default): small cups to tbsp, large tsp to tbsp, many tbsp to cups
"""
import json
import re
import sys
from fractions import Fraction

UNICODE = {"½": Fraction(1, 2), "⅓": Fraction(1, 3), "⅔": Fraction(2, 3), "¼": Fraction(1, 4), "¾": Fraction(3, 4),
           "⅕": Fraction(1, 5), "⅖": Fraction(2, 5), "⅗": Fraction(3, 5), "⅘": Fraction(4, 5), "⅙": Fraction(1, 6),
           "⅚": Fraction(5, 6), "⅛": Fraction(1, 8), "⅜": Fraction(3, 8), "⅝": Fraction(5, 8), "⅞": Fraction(7, 8)}
# unit: (canonical, family, factor to the family base: tsp for US volume, ml for metric volume, g for mass)
UNITS = {
    "cup": ("cup", "volume", Fraction(48)), "cups": ("cup", "volume", Fraction(48)), "c": ("cup", "volume", Fraction(48)),
    "tablespoon": ("tbsp", "volume", Fraction(3)), "tablespoons": ("tbsp", "volume", Fraction(3)), "tbsp": ("tbsp", "volume", Fraction(3)),
    "tbs": ("tbsp", "volume", Fraction(3)), "tbl": ("tbsp", "volume", Fraction(3)), "T": ("tbsp", "volume", Fraction(3)),
    "teaspoon": ("tsp", "volume", Fraction(1)), "teaspoons": ("tsp", "volume", Fraction(1)), "tsp": ("tsp", "volume", Fraction(1)),
    "t": ("tsp", "volume", Fraction(1)),
    "fl oz": ("fl oz", "volume", Fraction(6)), "fl. oz": ("fl oz", "volume", Fraction(6)), "fluid ounce": ("fl oz", "volume", Fraction(6)),
    "fluid ounces": ("fl oz", "volume", Fraction(6)),
    "pint": ("pint", "volume", Fraction(96)), "pints": ("pint", "volume", Fraction(96)), "quart": ("quart", "volume", Fraction(192)),
    "quarts": ("quart", "volume", Fraction(192)),
    "ml": ("ml", "metric_volume", Fraction(1)), "milliliter": ("ml", "metric_volume", Fraction(1)), "milliliters": ("ml", "metric_volume", Fraction(1)),
    "millilitre": ("ml", "metric_volume", Fraction(1)), "millilitres": ("ml", "metric_volume", Fraction(1)),
    "cl": ("cl", "metric_volume", Fraction(10)), "dl": ("dl", "metric_volume", Fraction(100)),
    "l": ("l", "metric_volume", Fraction(1000)), "liter": ("l", "metric_volume", Fraction(1000)), "liters": ("l", "metric_volume", Fraction(1000)),
    "litre": ("l", "metric_volume", Fraction(1000)), "litres": ("l", "metric_volume", Fraction(1000)),
    "g": ("g", "mass", Fraction(1)), "gram": ("g", "mass", Fraction(1)), "grams": ("g", "mass", Fraction(1)),
    "kg": ("kg", "mass", Fraction(1000)), "kilogram": ("kg", "mass", Fraction(1000)), "kilograms": ("kg", "mass", Fraction(1000)),
    "mg": ("mg", "mass", Fraction(1, 1000)),
    "oz": ("oz", "mass", Fraction(28349523125, 10 ** 9)), "ounce": ("oz", "mass", Fraction(28349523125, 10 ** 9)),
    "ounces": ("oz", "mass", Fraction(28349523125, 10 ** 9)),
    "lb": ("lb", "mass", Fraction(45359237, 10 ** 5)), "lbs": ("lb", "mass", Fraction(45359237, 10 ** 5)),
    "pound": ("lb", "mass", Fraction(45359237, 10 ** 5)), "pounds": ("lb", "mass", Fraction(45359237, 10 ** 5)),
    "pinch": ("pinch", "other", None), "pinches": ("pinch", "other", None), "dash": ("dash", "other", None), "dashes": ("dash", "other", None),
    "clove": ("clove", "count", None), "cloves": ("clove", "count", None), "can": ("can", "count", None), "cans": ("can", "count", None),
    "stick": ("stick", "count", None), "sticks": ("stick", "count", None), "slice": ("slice", "count", None), "slices": ("slice", "count", None),
    "piece": ("piece", "count", None), "pieces": ("piece", "count", None), "package": ("package", "count", None),
    "packages": ("package", "count", None), "sprig": ("sprig", "count", None), "sprigs": ("sprig", "count", None),
    "bunch": ("bunch", "count", None), "handful": ("handful", "other", None),
}
PLURAL = {"cup": "cups", "pint": "pints", "quart": "quarts", "pinch": "pinches", "dash": "dashes", "clove": "cloves",
          "can": "cans", "stick": "sticks", "slice": "slices", "piece": "pieces", "package": "packages", "sprig": "sprigs"}
US_ML_PER_TSP = Fraction(473176473, 96 * 10 ** 6)  # US cup 236.5882365 mL / 48 (NIST Handbook 44)
NUM = r"(?:\d+\s+\d+/\d+|\d+\s*[½⅓⅔¼¾⅕⅖⅗⅘⅙⅚⅛⅜⅝⅞]|\d+/\d+|\d+(?:\.\d+)?|\.\d+|[½⅓⅔¼¾⅕⅖⅗⅘⅙⅚⅛⅜⅝⅞])"
LINE = re.compile(rf"^\s*(?P<q1>{NUM})(?:\s*(?:-|–|—|to)\s*(?P<q2>{NUM}))?\s*(?P<rest>.*)$")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def parse_number(text, field):
    text = text.strip()
    try:
        m = re.fullmatch(r"(\d+)\s+(\d+)/(\d+)", text)
        if m:
            return int(m.group(1)) + Fraction(int(m.group(2)), int(m.group(3)))
        m = re.fullmatch(r"(\d+)\s*([½⅓⅔¼¾⅕⅖⅗⅘⅙⅚⅛⅜⅝⅞])", text)
        if m:
            return int(m.group(1)) + UNICODE[m.group(2)]
        if text in UNICODE:
            return UNICODE[text]
        return Fraction(text)
    except (ValueError, ZeroDivisionError):
        fail(f"{field}: cannot read the quantity {text!r}")


def mixed(x):
    if x.denominator == 1:
        return str(x.numerator)
    whole, rest = divmod(x.numerator, x.denominator)
    frac = f"{rest}/{x.denominator}"
    return f"{whole} {frac}" if whole else frac


def split_unit(rest):
    """(unit key or None, name) from the text after the quantity."""
    text = rest.strip()
    lowered = text.lower()
    for key in sorted(UNITS, key=len, reverse=True):
        if key == "T" or key == "t":
            continue
        if lowered.startswith(key) and (len(text) == len(key) or not text[len(key)].isalpha()):
            name = text[len(key):].lstrip(" .")
            if name.lower().startswith("of "):
                name = name[3:]
            return key, name.strip()
    for key in ("T", "t"):
        if text.startswith(key + " "):
            return key, text[2:].strip()
    return None, text


def parse_line(item, i):
    field = f"ingredients[{i}]"
    if isinstance(item, dict):
        extra = set(item) - {"quantity", "unit", "name"}
        if extra:
            fail(f"{field}: unknown keys {sorted(extra)}")
        name = item.get("name", "")
        if not isinstance(name, str):
            fail(f"{field}.name must be a string")
        unit = item.get("unit")
        if unit is not None and (not isinstance(unit, str) or (unit not in UNITS and unit.lower() not in UNITS)):
            fail(f"{field}.unit {unit!r} is not a known unit")
        if unit is not None and unit not in UNITS:
            unit = unit.lower()
        q = item.get("quantity")
        if q is None:
            return {"text": name, "low": None, "high": None, "unit": unit, "name": name}
        if isinstance(q, bool) or not isinstance(q, (int, float, str)):
            fail(f"{field}.quantity must be a number or a string")
        m = LINE.match(str(q))
        if not m or m.group("rest").strip():
            fail(f"{field}: cannot read the quantity {q!r}")
        low = parse_number(m.group("q1"), field)
        high = parse_number(m.group("q2"), field) if m.group("q2") else None
        return {"text": f"{q} {unit or ''} {name}".replace("  ", " ").strip(), "low": low, "high": high, "unit": unit, "name": name}
    if not isinstance(item, str):
        fail(f"{field} must be a string or an object")
    if len(item) > 500:
        fail(f"{field} is longer than 500 characters")
    m = LINE.match(item)
    if not m:
        return {"text": item, "low": None, "high": None, "unit": None, "name": item.strip()}
    low = parse_number(m.group("q1"), field)
    high = parse_number(m.group("q2"), field) if m.group("q2") else None
    unit, name = split_unit(m.group("rest"))
    return {"text": item, "low": low, "high": high, "unit": unit, "name": name}


def nearest(x, step):
    n = x / step
    whole = n.numerator // n.denominator
    if n - whole >= Fraction(1, 2):
        whole += 1
    return whole * step


def kitchen_round(x, canon, is_egg):
    """(rounded value, note or None)."""
    if x == 0:
        return x, None
    if is_egg:
        r = max(Fraction(1), nearest(x, Fraction(1)))
        whole = x.numerator // x.denominator
        return r, (None if r == x else f"{mixed(x)} eggs is not a whole number: rounded to {r} "
                   f"(or use {whole} and {mixed(x - whole)} of a beaten egg)")
    steps = {"cup": [Fraction(1, 4), Fraction(1, 3)], "tbsp": [Fraction(1, 2)], "tsp": [Fraction(1, 8)], "fl oz": [Fraction(1, 2)],
             "g": [Fraction(1)], "ml": [Fraction(1)], "oz": [Fraction(1, 4)], "lb": [Fraction(1, 4)], "kg": [Fraction(1, 100)],
             "l": [Fraction(1, 100)], "pint": [Fraction(1, 4)], "quart": [Fraction(1, 4)], "mg": [Fraction(1)], "cl": [Fraction(1)],
             "dl": [Fraction(1, 10)]}.get(canon, [Fraction(1, 2)])
    if canon == "g" and x < 10:
        steps = [Fraction(1, 10)]
    best = min((nearest(x, s) for s in steps), key=lambda r: (abs(r - x), r.denominator))
    note = None
    if best == 0:
        best = min(steps)
        note = f"below the smallest step: shown as {mixed(best)}"
    return best, note


def to_spoons(x, canon):
    """Re-express US volumes in a handier unit: under 1/4 cup (or a cup fraction off the 1/4 and 1/3 marks
    that is a whole or half tablespoon) in tbsp, 3 tsp or more in tbsp, 4 tbsp or more in whole quarter cups."""
    tsp = x * {"cup": 48, "tbsp": 3, "tsp": 1}[canon]
    if canon == "cup" and x < Fraction(1, 4):
        return tsp / 3, "tbsp"
    if canon == "cup" and x < 1 and (x * 4).denominator != 1 and (x * 3).denominator != 1 and (x * 16).denominator <= 2:
        return tsp / 3, "tbsp"  # 3/8 cup is 6 tbsp: exact in spoons, not on the cup marks
    if canon == "tsp" and tsp >= 3 and (tsp / 3).denominator in (1, 2):
        return tsp / 3, "tbsp"
    if canon == "tbsp" and tsp >= 12 and (tsp / 48 * 4).denominator == 1:
        return tsp / 48, "cup"
    return x, canon


def parse_factor(request):
    given = [k for k in ("servings", "factor", "target") if k in request]
    if len(given) != 1:
        fail("give exactly one of servings, factor, target")
    kind = given[0]
    if kind == "servings":
        s = request["servings"]
        if not isinstance(s, dict) or set(s) != {"from", "to"}:
            fail("servings must be {\"from\": n, \"to\": m}")
        if any(isinstance(s[k], bool) or not isinstance(s[k], (int, float, str)) for k in ("from", "to")):
            fail("servings must be numbers")
        a, b = parse_number(str(s["from"]), "servings.from"), parse_number(str(s["to"]), "servings.to")
        if a <= 0 or b <= 0:
            fail("servings must be positive")
        return b / a, f"servings {mixed(a)} -> {mixed(b)}"
    if kind == "factor":
        f = request["factor"]
        if isinstance(f, bool) or not isinstance(f, (int, float, str)):
            fail("factor must be a number or a string such as \"3/2\" or \"150%\"")
        text = str(f).strip()
        if text.endswith("%"):
            value = parse_number(text[:-1], "factor") / 100
        else:
            value = parse_number(text, "factor")
        if value <= 0:
            fail("factor must be positive")
        return value, f"factor {mixed(value)}"
    return None, "target"


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"ingredients", "servings", "factor", "target", "round", "convert_units"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    items = request.get("ingredients")
    if not isinstance(items, list) or not items or len(items) > 500:
        fail("ingredients must be a list of 1 to 500 lines")
    do_round = request.get("round", True)
    convert = request.get("convert_units", True)
    if not isinstance(do_round, bool) or not isinstance(convert, bool):
        fail("round and convert_units must be booleans")
    lines = [parse_line(item, i) for i, item in enumerate(items)]
    factor, how = parse_factor(request)
    if factor is None:
        target = request["target"]
        if not isinstance(target, dict) or not {"ingredient", "quantity"} <= set(target) or set(target) - {"ingredient", "quantity", "unit"}:
            fail("target must be {\"ingredient\": name, \"quantity\": q, \"unit\"?: u}")
        wanted = str(target["ingredient"]).strip().lower()
        matches = [ln for ln in lines if ln["low"] is not None and wanted and wanted in ln["name"].lower()]
        if not matches:
            fail(f"target: no ingredient with a quantity matches {target['ingredient']!r}")
        line = matches[0]
        if line["high"] is not None:
            fail("target: the matching ingredient has a range; scale by servings or factor instead")
        tq = parse_number(str(target["quantity"]), "target.quantity")
        if tq <= 0 or line["low"] == 0:
            fail("target: quantities must be positive")
        tunit = target.get("unit", line["unit"])
        if tunit is not None and tunit not in UNITS and str(tunit).lower() not in UNITS:
            fail(f"target.unit {tunit!r} is not a known unit")
        if tunit is not None and tunit not in UNITS:
            tunit = tunit.lower()
        src, dst = UNITS.get(line["unit"]) if line["unit"] else None, UNITS.get(tunit) if tunit else None
        if (src is None) != (dst is None):
            fail("target: give the target in the recipe's unit")
        if src is None or src[0] == dst[0]:
            factor = tq / line["low"]
        else:
            fam = {"volume": "v", "metric_volume": "v", "mass": "m"}
            if src[2] is None or dst[2] is None or fam.get(src[1]) != fam.get(dst[1]) or fam.get(src[1]) is None:
                fail(f"target: cannot convert {dst[0]} to the recipe's {src[0]}")
            def base(q, u):
                b = q * u[2]
                return b * US_ML_PER_TSP if u[1] == "volume" else b
            factor = base(tq, dst) / base(line["low"], src)
        how = f"target {mixed(tq)} {tunit or ''} of {line['name']}".replace("  ", " ")
    out, notes_all = [], []
    for ln in lines:
        entry = {"original": ln["text"], "name": ln["name"]}
        if ln["low"] is None:
            entry["scaled"] = ln["text"]
            entry["note"] = "no quantity: adjust to taste"
            out.append(entry)
            continue
        canon, family = (UNITS[ln["unit"]][0], UNITS[ln["unit"]][1]) if ln["unit"] else (None, "count")
        is_egg = canon is None and re.search(r"\beggs?\b", ln["name"].lower()) is not None
        values = [ln["low"] * factor] + ([ln["high"] * factor] if ln["high"] is not None else [])
        entry["exact"] = " - ".join(mixed(v) for v in values) + (f" {canon}" if canon else "")
        unit_out = canon
        shown = values
        notes = []
        if convert and canon in ("cup", "tbsp", "tsp"):
            converted = [to_spoons(v, canon) for v in values]
            if len({u for _, u in converted}) == 1:
                shown, unit_out = [v for v, _ in converted], converted[0][1]
        if do_round:
            rounded = []
            for v in shown:
                r, note = kitchen_round(v, unit_out, is_egg)
                rounded.append(r)
                if note:
                    notes.append(note)
            if rounded != shown and not is_egg:
                notes.append("rounded for measuring")
            shown = rounded
        entry["quantity"] = " - ".join(mixed(v) for v in shown)
        entry["unit"] = unit_out
        label = unit_out
        if unit_out in PLURAL and max(shown) > 1:
            label = PLURAL[unit_out]
        entry["scaled"] = f"{entry['quantity']}{' ' + label if label else ''} {ln['name']}".strip()
        if is_egg and values[0].denominator != 1:
            entry["flag"] = "eggs"
        if family == "other":
            notes.append("pinches and dashes do not scale precisely")
        if notes:
            entry["note"] = "; ".join(notes)
        out.append(entry)
    result = {"factor": mixed(factor), "factor_decimal": format(float(factor), ".6g"), "basis": how, "ingredients": out,
              "rounding": "cups to 1/4 or 1/3, tbsp to 1/2, tsp to 1/8, g and ml to 1 (0.1 g under 10 g), oz and lb to 1/4, eggs whole" if do_round else "none"}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
