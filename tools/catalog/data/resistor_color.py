#!/usr/bin/env python3
"""Write catalog/tools/personal/resistor_color/codes.json: the colour code and the E series the
tool bundles, transcribed on 18 Oct 2026 (literal values, no network) from

  - IEC 60062:2016 (6th ed.), Marking codes for resistors and capacitors, the colour code table:
    significant numerals (black 0 ... white 9), multipliers (pink 10^-3, silver 10^-2, gold 10^-1,
    black 10^0 ... white 10^9), resistance tolerances (silver 10 %, gold 5 %, brown 1 %, red 2 %,
    orange 0.05 %, yellow 0.02 %, green 0.5 %, blue 0.25 %, violet 0.1 %, grey 0.01 %, no band
    20 %) and temperature coefficients in ppm/K (black 250, brown 100, red 50, orange 15,
    yellow 25, green 20, blue 10, violet 5, grey 1);
  - IEC 60063:2015, Preferred number series for resistors and capacitors: the E3, E6, E12, E24,
    E48, E96 and E192 series (one decade, 1.0 to 9.88).
  Both as reproduced in the English Wikipedia articles "Electronic color code" (Template:Electronic
  colour code) and "E series of preferred numbers" (section "Lists"), retrieved 2026-10-18, which
  cite the standards. The standards themselves are sold by the IEC; the values are facts.
Licence: the colour assignments and the preferred numbers are facts, not copyrightable
expression; no text of the standards or of Wikipedia (CC BY-SA 4.0) is copied, only the values.

The script checks the official lists against the generating formula round(10^(n/m)) and records
the documented exceptions (eight E24 values; 9.20 for n = 185 in E192), and that E3 to E12 are
subsets of E24 and E48, E96 subsets of E192. A revision of either standard is a new table version
and a new version of the tool.

Usage: python3 tools/catalog/data/resistor_color.py
"""
import json
import pathlib
from decimal import ROUND_HALF_UP, Decimal

E24 = "1.0 1.1 1.2 1.3 1.5 1.6 1.8 2.0 2.2 2.4 2.7 3.0 3.3 3.6 3.9 4.3 4.7 5.1 5.6 6.2 6.8 7.5 8.2 9.1"
E12 = "1.0 1.2 1.5 1.8 2.2 2.7 3.3 3.9 4.7 5.6 6.8 8.2"
E6 = "1.0 1.5 2.2 3.3 4.7 6.8"
E3 = "1.0 2.2 4.7"
E48 = ("1.00 1.05 1.10 1.15 1.21 1.27 1.33 1.40 1.47 1.54 1.62 1.69 1.78 1.87 1.96 2.05 2.15 2.26 2.37 2.49 "
       "2.61 2.74 2.87 3.01 3.16 3.32 3.48 3.65 3.83 4.02 4.22 4.42 4.64 4.87 5.11 5.36 5.62 5.90 6.19 6.49 "
       "6.81 7.15 7.50 7.87 8.25 8.66 9.09 9.53")
E96 = ("1.00 1.02 1.05 1.07 1.10 1.13 1.15 1.18 1.21 1.24 1.27 1.30 1.33 1.37 1.40 1.43 1.47 1.50 1.54 1.58 "
       "1.62 1.65 1.69 1.74 1.78 1.82 1.87 1.91 1.96 2.00 2.05 2.10 2.15 2.21 2.26 2.32 2.37 2.43 2.49 2.55 "
       "2.61 2.67 2.74 2.80 2.87 2.94 3.01 3.09 3.16 3.24 3.32 3.40 3.48 3.57 3.65 3.74 3.83 3.92 4.02 4.12 "
       "4.22 4.32 4.42 4.53 4.64 4.75 4.87 4.99 5.11 5.23 5.36 5.49 5.62 5.76 5.90 6.04 6.19 6.34 6.49 6.65 "
       "6.81 6.98 7.15 7.32 7.50 7.68 7.87 8.06 8.25 8.45 8.66 8.87 9.09 9.31 9.53 9.76")
E192 = ("1.00 1.01 1.02 1.04 1.05 1.06 1.07 1.09 1.10 1.11 1.13 1.14 1.15 1.17 1.18 1.20 1.21 1.23 1.24 1.26 "
        "1.27 1.29 1.30 1.32 1.33 1.35 1.37 1.38 1.40 1.42 1.43 1.45 1.47 1.49 1.50 1.52 1.54 1.56 1.58 1.60 "
        "1.62 1.64 1.65 1.67 1.69 1.72 1.74 1.76 1.78 1.80 1.82 1.84 1.87 1.89 1.91 1.93 1.96 1.98 2.00 2.03 "
        "2.05 2.08 2.10 2.13 2.15 2.18 2.21 2.23 2.26 2.29 2.32 2.34 2.37 2.40 2.43 2.46 2.49 2.52 2.55 2.58 "
        "2.61 2.64 2.67 2.71 2.74 2.77 2.80 2.84 2.87 2.91 2.94 2.98 3.01 3.05 3.09 3.12 3.16 3.20 3.24 3.28 "
        "3.32 3.36 3.40 3.44 3.48 3.52 3.57 3.61 3.65 3.70 3.74 3.79 3.83 3.88 3.92 3.97 4.02 4.07 4.12 4.17 "
        "4.22 4.27 4.32 4.37 4.42 4.48 4.53 4.59 4.64 4.70 4.75 4.81 4.87 4.93 4.99 5.05 5.11 5.17 5.23 5.30 "
        "5.36 5.42 5.49 5.56 5.62 5.69 5.76 5.83 5.90 5.97 6.04 6.12 6.19 6.26 6.34 6.42 6.49 6.57 6.65 6.73 "
        "6.81 6.90 6.98 7.06 7.15 7.23 7.32 7.41 7.50 7.59 7.68 7.77 7.87 7.96 8.06 8.16 8.25 8.35 8.45 8.56 "
        "8.66 8.76 8.87 8.98 9.09 9.20 9.31 9.42 9.53 9.65 9.76 9.88")
SERIES = {"E3": (E3, "40"), "E6": (E6, "20"), "E12": (E12, "10"), "E24": (E24, "5"), "E48": (E48, "2"),
          "E96": (E96, "1"), "E192": (E192, "0.5")}

# colour, digit, multiplier exponent, tolerance %, TCR ppm/K, IEC 60757 letter code
COLOURS = [
    ("pink", None, -3, None, None, "PK"),
    ("silver", None, -2, "10", None, "SR"),
    ("gold", None, -1, "5", None, "GD"),
    ("black", 0, 0, None, 250, "BK"),
    ("brown", 1, 1, "1", 100, "BN"),
    ("red", 2, 2, "2", 50, "RD"),
    ("orange", 3, 3, "0.05", 15, "OG"),
    ("yellow", 4, 4, "0.02", 25, "YE"),
    ("green", 5, 5, "0.5", 20, "GN"),
    ("blue", 6, 6, "0.25", 10, "BU"),
    ("violet", 7, 7, "0.1", 5, "VT"),
    ("grey", 8, 8, "0.01", 1, "GY"),
    ("white", 9, 9, None, None, "WH"),
]


def check(name, values):
    m = len(values)
    places = 1 if m <= 24 else 2
    exceptions = []
    for n, official in enumerate(values):
        calc = Decimal(10 ** (n / m)).quantize(Decimal(1).scaleb(-places), ROUND_HALF_UP)
        if calc != Decimal(official):
            exceptions.append({"n": n, "official": official, "formula": str(calc)})
    return exceptions


def main():
    series = {}
    for name, (text, tol) in SERIES.items():
        values = text.split()
        if len(values) != int(name[1:]):
            raise RuntimeError(f"{name} has {len(values)} values")
        series[name] = {"tolerance_pct": tol, "values": values, "formula_exceptions": check(name, values)}
    e24, e192 = set(map(Decimal, E24.split())), set(map(Decimal, E192.split()))
    for small in ("E3", "E6", "E12"):
        assert set(map(Decimal, series[small]["values"])) <= e24, small
    for small in ("E48", "E96"):
        assert set(map(Decimal, series[small]["values"])) <= e192, small
        assert not series[small]["formula_exceptions"], small
    assert len(series["E24"]["formula_exceptions"]) == 8
    assert [e["n"] for e in series["E192"]["formula_exceptions"]] == [185]
    colours = {c: {"digit": d, "multiplier_exp": m, "tolerance_pct": t, "tcr_ppm_per_k": tc, "code": code}
               for c, d, m, t, tc, code in COLOURS}
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/personal/resistor_color/codes.json"
    payload = {"generated": "2026-10-18", "generator": "tools/catalog/data/resistor_color.py",
               "source": "IEC 60062:2016 colour code; IEC 60063:2015 E series (as reproduced in Wikipedia, "
                         "'Electronic color code' and 'E series of preferred numbers', retrieved 2026-10-18)",
               "licence": "factual values only (no text of IEC 60062/60063 or of Wikipedia, CC BY-SA 4.0, is copied)",
               "no_band_tolerance_pct": "20", "colours": colours, "series": series}
    out.write_text(json.dumps(payload, indent=1) + "\n")
    print(f"{out}: {len(colours)} colours; " + ", ".join(f"{k} ({len(v['formula_exceptions'])} exceptions)" for k, v in series.items()))


if __name__ == "__main__":
    main()
