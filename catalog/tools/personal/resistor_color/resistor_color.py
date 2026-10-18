#!/usr/bin/env python3
"""Resistor colour code (IEC 60062) and preferred values (IEC 60063).

Operations:
  decode   bands -> resistance, tolerance, temperature coefficient, E-series membership
  encode   value -> the 4-band and 5-band colour codes (and 6-band with a temperature coefficient)
  nearest  value -> the nearest standard values of an E series (below, above, nearest by ratio)

Input (JSON on stdin):
  operation   "decode" | "encode" | "nearest"  (default: decode when bands is given, else encode)
  bands       ["brown", "black", "red", "gold"]  3 to 6 colour names, head end first      decode
  value       ohms: number, decimal string or RKM/SI text ("4k7", "R47", "2.2M", "10 kΩ")  encode, nearest
  tolerance   percent for the tolerance band (default 5 for 4 bands, 1 for 5 and 6)            encode
  tcr         ppm/K for a sixth band (100, 50, 15, 25, 20, 10, 5, 1, 250)                       encode
  series      "E3" ... "E192" (default E24)                                                    nearest
"""
import json
import math
import re
import sys
from decimal import Decimal, InvalidOperation, getcontext

getcontext().prec = 40

PREFIXES = [(Decimal("1e9"), "G"), (Decimal("1e6"), "M"), (Decimal("1e3"), "k"), (Decimal(1), ""), (Decimal("1e-3"), "m")]
ALIASES = {"gray": "grey", "purple": "violet", "none": "none", "": "none", "gold": "gold", "silver": "silver"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load_table():
    try:
        with open("codes.json", encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, ValueError) as error:
        fail(f"cannot read the bundled codes.json: {error}")


def plain(x):
    text = format(x.normalize(), "f")
    return text


def si(ohms):
    if ohms == 0:
        return "0 Ω"
    for scale, prefix in PREFIXES:
        if abs(ohms) >= scale:
            return f"{plain(ohms / scale)} {prefix}Ω"
    return f"{plain(ohms / Decimal('1e-3'))} mΩ"


def parse_value(value):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail("value must be a number or a string such as \"4k7\" or \"10 kΩ\"")
    text = str(value).strip()
    for suffix in ("ohms", "ohm", "Ω", "Ω"):
        if text.endswith(suffix):
            text = text[: -len(suffix)].strip()
            break
    multipliers = {"R": Decimal(1), "r": Decimal(1), "K": Decimal(1000), "k": Decimal(1000), "M": Decimal(10) ** 6,
                   "G": Decimal(10) ** 9, "T": Decimal(10) ** 12, "m": Decimal("0.001"), "L": Decimal("0.001")}
    match = re.fullmatch(r"(\d*)([RrKkMGTmL])(\d*)", text)
    if match and (match.group(1) or match.group(3)):
        number = Decimal((match.group(1) or "0") + "." + (match.group(3) or "0"))
        result = number * multipliers[match.group(2)]
    else:
        match = re.fullmatch(r"([0-9]*\.?[0-9]+(?:[eE][-+]?\d+)?)\s*([kKMGmT]?)", text)
        if not match:
            fail(f"value {value!r} is not a resistance (use 4700, \"4.7k\", \"4k7\", \"R47\", \"10 kΩ\")")
        try:
            result = Decimal(match.group(1)) * (multipliers[match.group(2)] if match.group(2) else 1)
        except InvalidOperation:
            fail(f"value {value!r} is not a number")
    if result <= 0:
        fail("value must be positive")
    return result


def colour_name(name, field):
    if not isinstance(name, str):
        fail(f"{field} must be a colour name")
    key = name.strip().lower()
    return ALIASES.get(key, key)


def series_of(table, mantissa):
    """E series whose list contains this mantissa (1 <= m < 10)."""
    found = []
    for name, info in table["series"].items():
        if any(Decimal(v) == mantissa for v in info["values"]):
            found.append(name)
    return found


def normalise(ohms):
    exponent = ohms.adjusted()
    return ohms.scaleb(-exponent), exponent


def decode(table, bands):
    colours = table["colours"]
    if not isinstance(bands, list) or not 3 <= len(bands) <= 6:
        fail("bands must be a list of 3 to 6 colour names")
    names = [colour_name(b, f"bands[{i}]") for i, b in enumerate(bands)]
    for i, name in enumerate(names):
        if name not in colours:
            fail(f"bands[{i}]: unknown colour {bands[i]!r}; colours: {', '.join(colours)}")
    n = len(names)
    digits_count = 2 if n <= 4 else 3
    digit_bands = names[:digits_count]
    for i, name in enumerate(digit_bands):
        if colours[name]["digit"] is None:
            fail(f"band {i + 1} ({name}) is not a digit colour; a {n}-band code starts with {digits_count} digit bands")
    mult_name = names[digits_count]
    exp = colours[mult_name]["multiplier_exp"]
    significand = int("".join(str(colours[c]["digit"]) for c in digit_bands))
    ohms = Decimal(significand).scaleb(exp)
    if n == 3:
        tol = Decimal(table["no_band_tolerance_pct"])
        tol_name = "none (3-band code)"
    else:
        tol_name = names[digits_count + 1]
        if colours[tol_name]["tolerance_pct"] is None:
            fail(f"band {digits_count + 2} ({tol_name}) is not a tolerance colour")
        tol = Decimal(colours[tol_name]["tolerance_pct"])
    result = {"operation": "decode", "bands": names, "ohms": plain(ohms), "value": si(ohms),
              "tolerance_pct": plain(tol), "tolerance_band": tol_name,
              "min_ohms": plain(ohms * (100 - tol) / 100), "max_ohms": plain(ohms * (100 + tol) / 100)}
    if n == 6:
        tcr_name = names[5]
        tcr = colours[tcr_name]["tcr_ppm_per_k"]
        if tcr is None:
            fail(f"band 6 ({tcr_name}) is not a temperature-coefficient colour")
        result["tcr_ppm_per_k"] = tcr
    warnings = []
    if colours[names[0]]["digit"] == 0:
        warnings.append("the first band is black (a leading zero): check that the resistor is not read backwards")
    if names[-1] in ("gold", "silver") and n >= 4 and names[0] in ("gold", "silver"):
        warnings.append("gold or silver at both ends: the direction cannot be told")
    if ohms > 0:
        mantissa, _ = normalise(ohms)
        result["e_series"] = series_of(table, mantissa)
    if warnings:
        result["warnings"] = warnings
    result["standard"] = table["source"]
    return result


def digits_for(ohms, count):
    """(digits, exponent) with ohms = digits x 10^exponent and `count` digits, or None."""
    mantissa, exponent = normalise(ohms)
    scaled = mantissa.scaleb(count - 1)
    if scaled != scaled.to_integral_value():
        return None
    digits = int(scaled)
    exp = exponent - (count - 1)
    return digits, exp


def encode(table, request):
    colours = table["colours"]
    ohms = parse_value(request.get("value"))
    by_digit = {info["digit"]: name for name, info in colours.items() if info["digit"] is not None}
    by_exp = {info["multiplier_exp"]: name for name, info in colours.items()}
    by_tol = {Decimal(info["tolerance_pct"]): name for name, info in colours.items() if info["tolerance_pct"] is not None}
    by_tcr = {info["tcr_ppm_per_k"]: name for name, info in colours.items() if info["tcr_ppm_per_k"] is not None}

    def tol_colour(default):
        raw = request.get("tolerance", default)
        try:
            tol = Decimal(str(raw).rstrip("%").strip())
        except InvalidOperation:
            fail("tolerance must be a percentage such as 5 or \"1%\"")
        if tol not in by_tol:
            fail(f"tolerance {raw} has no colour; IEC 60062 tolerances: {', '.join(plain(t) for t in sorted(by_tol))} %")
        return by_tol[tol], tol

    result = {"operation": "encode", "ohms": plain(ohms), "value": si(ohms)}
    for count, key, default in ((2, "bands_4", 5), (3, "bands_5", 1)):
        found = digits_for(ohms, count)
        if found is None:
            result[key] = None
            result[key + "_note"] = f"needs more than {count} significant digits"
            continue
        digits, exp = found
        if exp not in by_exp:
            result[key] = None
            result[key + "_note"] = f"multiplier 10^{exp} has no colour (pink 10^-3 to white 10^9)"
            continue
        tol_name, tol = tol_colour(default)
        bands = [by_digit[int(d)] for d in str(digits).zfill(count)] + [by_exp[exp], tol_name]
        result[key] = bands
        result[key + "_tolerance_pct"] = plain(tol)
    if "tcr" in request:
        tcr = request["tcr"]
        if isinstance(tcr, bool) or not isinstance(tcr, int) or tcr not in by_tcr:
            fail(f"tcr must be one of {sorted(by_tcr)} ppm/K")
        result["bands_6"] = (result["bands_5"] + [by_tcr[tcr]]) if result.get("bands_5") else None
    mantissa, _ = normalise(ohms)
    result["e_series"] = series_of(table, mantissa)
    result["standard"] = table["source"]
    return result


def nearest(table, request):
    ohms = parse_value(request.get("value"))
    name = request.get("series", "E24")
    if not isinstance(name, str) or name.upper() not in table["series"]:
        fail(f"series must be one of {', '.join(table['series'])}")
    name = name.upper()
    values = [Decimal(v) for v in table["series"][name]["values"]]
    mantissa, exponent = normalise(ohms)
    candidates = [values[-1].scaleb(exponent - 1)] + [v.scaleb(exponent) for v in values] + [values[0].scaleb(exponent + 1)]
    below = max(c for c in candidates if c <= ohms)
    above = min(c for c in candidates if c >= ohms)
    ratio_pick = min((below, above), key=lambda c: abs(math.log(float(c) / float(ohms))))
    error = (ratio_pick - ohms) / ohms * 100
    result = {"operation": "nearest", "ohms": plain(ohms), "value": si(ohms), "series": name,
              "series_tolerance_pct": table["series"][name]["tolerance_pct"],
              "nearest": plain(ratio_pick), "nearest_value": si(ratio_pick),
              "error_pct": format(error.quantize(Decimal("0.001")), "f"),
              "below": plain(below), "above": plain(above), "exact_match": below == ohms,
              "rule": "nearest on a logarithmic scale (smaller ratio), as the E series are geometric",
              "standard": table["source"]}
    return result


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"operation", "bands", "value", "tolerance", "tcr", "series"}
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    table = load_table()
    operation = request.get("operation") or ("decode" if "bands" in request else "encode")
    if operation == "decode":
        result = decode(table, request.get("bands"))
    elif operation == "encode":
        if "value" not in request:
            fail("encode needs value")
        result = encode(table, request)
    elif operation == "nearest":
        if "value" not in request:
            fail("nearest needs value")
        result = nearest(table, request)
    else:
        fail("operation must be decode, encode or nearest")
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
