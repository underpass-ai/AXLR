#!/usr/bin/env python3
"""Write catalog/tools/mathematics/unit_convert/units.json: the unit table the tool bundles.

Every factor is the exact value of one unit in SI base units (m, kg, s, A, K, mol, cd), written
as a fraction string and derived here from its legal definition, transcribed on 18 Oct 2026 from:
  - BIPM, The International System of Units (SI Brochure), 9th edition 2019, v2.01 (2022):
    Tables 2 (base units), 4 (derived units with special names), 7 (prefixes, including ronna,
    quetta, ronto and quecto from 2022) and 8 (non-SI units accepted for use with the SI)
    https://www.bipm.org/en/publications/si-brochure
  - NIST Special Publication 811 (2008 edition), Guide for the Use of the International System
    of Units, Appendix B.8 and B.9 (conversion factors)  https://www.nist.gov/pml/special-publication-811
  - NIST Handbook 44 (2024), Appendix C (US customary length, mass and capacity)
  - the 1959 international yard and pound agreement (yard = 0.9144 m, pound = 0.45359237 kg)
  - IAU 2012 Resolution B2 (astronomical unit = 149 597 870 700 m)
Angles in degrees are irrational in radians; they carry a 50-digit decimal and are marked inexact.
A new or corrected definition is a new table version and a new version of the tool.

Usage: python3 tools/catalog/data/unit_convert.py
"""
import json
import pathlib
from decimal import Decimal, getcontext
from fractions import Fraction as F

DIMS = ["m", "kg", "s", "A", "K", "mol", "cd"]
SI = "SI Brochure 9th ed. (2019)"
NIST = "NIST SP 811 (2008), App. B"
H44 = "NIST Handbook 44 (2024), App. C"
PI = Decimal("3.14159265358979323846264338327950288419716939937510582097494459")


def dim(**exps):
    return [exps.get(d, 0) for d in DIMS]


LENGTH, MASS, TIME, AREA, VOLUME = dim(m=1), dim(kg=1), dim(s=1), dim(m=2), dim(m=3)
FORCE, PRESSURE, ENERGY, POWER = dim(kg=1, m=1, s=-2), dim(kg=1, m=-1, s=-2), dim(kg=1, m=2, s=-2), dim(kg=1, m=2, s=-3)
SPEED, NONE = dim(m=1, s=-1), dim()

INCH, FOOT, YARD, MILE = F("0.0254"), F("0.3048"), F("0.9144"), F("1609.344")
POUND, GN = F("0.45359237"), F("9.80665")
LBF = POUND * GN
US_GAL = 231 * INCH ** 3
IMP_GAL = F("0.00454609")
CAL_IT = F("4.1868")
BTU_IT = CAL_IT * 1000 * POUND * F(5, 9)  # cal_IT per g per K x 1 lb x (1 degF = 5/9 K)
MMHG = F("13595.1") * GN * F("0.001")     # conventional: 13.5951 g/cm3 mercury column of 1 mm at g_n

# symbol, aliases, factor (in SI base units), dimension, prefixable, definition, source
UNITS = [
    ("m", ["metre", "meter"], F(1), LENGTH, True, "SI base unit of length", SI + ", Table 2"),
    ("g", ["gram"], F(1, 1000), MASS, True, "1/1000 kg (the kilogram is the SI base unit; prefixes attach to the gram)", SI + ", Table 2"),
    ("s", ["second"], F(1), TIME, True, "SI base unit of time", SI + ", Table 2"),
    ("A", ["ampere"], F(1), dim(A=1), True, "SI base unit of electric current", SI + ", Table 2"),
    ("K", ["kelvin"], F(1), dim(K=1), True, "SI base unit of thermodynamic temperature", SI + ", Table 2"),
    ("mol", ["mole"], F(1), dim(mol=1), True, "SI base unit of amount of substance", SI + ", Table 2"),
    ("cd", ["candela"], F(1), dim(cd=1), True, "SI base unit of luminous intensity", SI + ", Table 2"),
    ("rad", ["radian"], F(1), NONE, True, "m/m", SI + ", Table 4"),
    ("sr", ["steradian"], F(1), NONE, True, "m2/m2", SI + ", Table 4"),
    ("Hz", ["hertz"], F(1), dim(s=-1), True, "s-1", SI + ", Table 4"),
    ("N", ["newton"], F(1), FORCE, True, "kg m s-2", SI + ", Table 4"),
    ("Pa", ["pascal"], F(1), PRESSURE, True, "N/m2", SI + ", Table 4"),
    ("J", ["joule"], F(1), ENERGY, True, "N m", SI + ", Table 4"),
    ("W", ["watt"], F(1), POWER, True, "J/s", SI + ", Table 4"),
    ("C", ["coulomb"], F(1), dim(A=1, s=1), True, "A s", SI + ", Table 4"),
    ("V", ["volt"], F(1), dim(kg=1, m=2, s=-3, A=-1), True, "W/A", SI + ", Table 4"),
    ("F", ["farad"], F(1), dim(kg=-1, m=-2, s=4, A=2), True, "C/V", SI + ", Table 4"),
    ("ohm", ["\u03a9", "\u2126"], F(1), dim(kg=1, m=2, s=-3, A=-2), True, "V/A", SI + ", Table 4"),
    ("S", ["siemens"], F(1), dim(kg=-1, m=-2, s=3, A=2), True, "A/V", SI + ", Table 4"),
    ("Wb", ["weber"], F(1), dim(kg=1, m=2, s=-2, A=-1), True, "V s", SI + ", Table 4"),
    ("T", ["tesla"], F(1), dim(kg=1, s=-2, A=-1), True, "Wb/m2", SI + ", Table 4"),
    ("H", ["henry"], F(1), dim(kg=1, m=2, s=-2, A=-2), True, "Wb/A", SI + ", Table 4"),
    ("lm", ["lumen"], F(1), dim(cd=1), True, "cd sr", SI + ", Table 4"),
    ("lx", ["lux"], F(1), dim(cd=1, m=-2), True, "lm/m2", SI + ", Table 4"),
    ("Bq", ["becquerel"], F(1), dim(s=-1), True, "s-1", SI + ", Table 4"),
    ("Gy", ["gray"], F(1), dim(m=2, s=-2), True, "J/kg", SI + ", Table 4"),
    ("Sv", ["sievert"], F(1), dim(m=2, s=-2), True, "J/kg", SI + ", Table 4"),
    ("kat", ["katal"], F(1), dim(mol=1, s=-1), True, "mol/s", SI + ", Table 4"),
    ("degC", ["°C", "celsius"], F(1), dim(K=1), False, "degree Celsius: t/°C = T/K - 273.15; as a difference 1 °C = 1 K", SI + ", Table 4"),
    ("min", ["minute"], F(60), TIME, False, "60 s", SI + ", Table 8"),
    ("h", ["hour"], F(3600), TIME, False, "60 min = 3600 s", SI + ", Table 8"),
    ("d", ["day"], F(86400), TIME, False, "24 h = 86400 s", SI + ", Table 8"),
    ("wk", ["week"], F(604800), TIME, False, "7 d", "7 days of 86400 s"),
    ("yr", ["year", "a"], F(31557600), TIME, False, "Julian year, 365.25 d", "IAU; " + NIST + ".9 (year, 365.25 d)"),
    ("au", ["astronomical_unit"], F(149597870700), LENGTH, False, "149 597 870 700 m", SI + ", Table 8 (IAU 2012 Resolution B2)"),
    ("ly", ["light_year"], F(299792458 * 31557600), LENGTH, False, "c x Julian year = 9 460 730 472 580 800 m", "IAU; c from " + SI),
    ("ha", ["hectare"], F(10000), AREA, False, "1 hm2 = 10^4 m2", SI + ", Table 8"),
    ("L", ["l", "litre", "liter"], F(1, 1000), VOLUME, True, "1 dm3 = 10^-3 m3", SI + ", Table 8"),
    ("t", ["tonne"], F(1000), MASS, True, "10^3 kg", SI + ", Table 8"),
    ("eV", ["electronvolt"], F("1.602176634e-19"), ENERGY, True, "e x 1 V, e = 1.602176634e-19 C exactly", SI + ", Table 8"),
    ("angstrom", ["\u00c5", "\u212b"], F(1, 10 ** 10), LENGTH, False, "10^-10 m", NIST + ".8"),
    ("bar", [], F(100000), PRESSURE, True, "10^5 Pa", NIST + ".8"),
    ("in", ["inch"], INCH, LENGTH, False, "0.0254 m exactly", NIST + ".8; 1959 agreement"),
    ("ft", ["foot", "feet"], FOOT, LENGTH, False, "12 in = 0.3048 m exactly", NIST + ".8"),
    ("yd", ["yard"], YARD, LENGTH, False, "3 ft = 0.9144 m exactly", NIST + ".8; 1959 agreement"),
    ("mi", ["mile"], MILE, LENGTH, False, "5280 ft = 1609.344 m exactly (international mile)", NIST + ".8"),
    ("nmi", ["nautical_mile"], F(1852), LENGTH, False, "1852 m exactly", NIST + ".8; " + SI + ", Table 8 note"),
    ("acre", [], 43560 * FOOT ** 2, AREA, False, "43 560 ft2 = 4046.8564224 m2 (international foot)", NIST + ".8"),
    ("lb", ["pound"], POUND, MASS, False, "avoirdupois pound, 0.45359237 kg exactly", NIST + ".8; 1959 agreement"),
    ("oz", ["ounce"], POUND / 16, MASS, False, "lb/16 = 28.349523125 g", NIST + ".8"),
    ("st", ["stone"], 14 * POUND, MASS, False, "14 lb", H44),
    ("ton", ["short_ton"], 2000 * POUND, MASS, False, "short ton, 2000 lb", NIST + ".8"),
    ("long_ton", [], 2240 * POUND, MASS, False, "long ton, 2240 lb", NIST + ".8"),
    ("gr", ["grain"], POUND / 7000, MASS, False, "lb/7000 = 64.79891 mg", NIST + ".8"),
    ("ozt", ["troy_ounce"], 480 * POUND / 7000, MASS, False, "480 gr = 31.1034768 g", NIST + ".8"),
    ("ct", ["carat"], F(1, 5000), MASS, False, "metric carat, 200 mg", NIST + ".8"),
    ("gal", ["gallon", "us_gal"], US_GAL, VOLUME, False, "US gallon, 231 in3 = 3.785411784 L", NIST + ".8; " + H44),
    ("qt", ["quart"], US_GAL / 4, VOLUME, False, "US liquid quart, gal/4", H44),
    ("pt", ["pint"], US_GAL / 8, VOLUME, False, "US liquid pint, gal/8", H44),
    ("cup", [], US_GAL / 16, VOLUME, False, "US cup, gal/16 = 8 fl oz", H44),
    ("floz", ["fl_oz", "fluid_ounce"], US_GAL / 128, VOLUME, False, "US fluid ounce, gal/128", NIST + ".8; " + H44),
    ("tbsp", ["tablespoon"], US_GAL / 256, VOLUME, False, "US tablespoon, fl oz/2", H44),
    ("tsp", ["teaspoon"], US_GAL / 768, VOLUME, False, "US teaspoon, fl oz/6", H44),
    ("bbl", ["barrel"], 42 * US_GAL, VOLUME, False, "oil barrel, 42 US gal", NIST + ".8"),
    ("imp_gal", ["imperial_gallon"], IMP_GAL, VOLUME, False, "imperial gallon, 4.54609 L exactly", NIST + ".8"),
    ("imp_pt", ["imperial_pint"], IMP_GAL / 8, VOLUME, False, "imperial pint, imp gal/8", NIST + ".8"),
    ("imp_floz", ["imperial_fluid_ounce"], IMP_GAL / 160, VOLUME, False, "imperial fluid ounce, imp gal/160", NIST + ".8"),
    ("gn", ["g0", "standard_gravity"], GN, dim(m=1, s=-2), False, "standard acceleration of gravity, 9.80665 m/s2", NIST + ".8"),
    ("lbf", ["pound_force"], LBF, FORCE, False, "lb x g_n = 4.4482216152605 N", NIST + ".8"),
    ("kgf", ["kilogram_force"], GN, FORCE, False, "kg x g_n = 9.80665 N", NIST + ".8"),
    ("psi", [], LBF / INCH ** 2, PRESSURE, False, "lbf/in2", NIST + ".8"),
    ("atm", ["atmosphere"], F(101325), PRESSURE, False, "standard atmosphere, 101 325 Pa", NIST + ".8"),
    ("torr", ["Torr"], F(101325, 760), PRESSURE, False, "atm/760", NIST + ".8"),
    ("mmHg", [], MMHG, PRESSURE, False, "conventional millimetre of mercury, 133.322387415 Pa", NIST + ".8"),
    ("inHg", [], MMHG * F("25.4"), PRESSURE, False, "conventional inch of mercury, 25.4 mmHg", NIST + ".8"),
    ("cal", ["calorie"], F("4.184"), ENERGY, True, "thermochemical calorie, 4.184 J exactly", NIST + ".8"),
    ("cal_IT", [], CAL_IT, ENERGY, True, "International Table calorie, 4.1868 J exactly", NIST + ".8"),
    ("Btu", ["BTU", "btu"], BTU_IT, ENERGY, False, "International Table Btu, 1055.05585262 J", NIST + ".8"),
    ("Wh", ["watt_hour"], F(3600), ENERGY, True, "W x h = 3600 J", SI + ", Table 8 (hour)"),
    ("Ah", ["amp_hour"], F(3600), dim(A=1, s=1), True, "A x h = 3600 C", SI + ", Table 8 (hour)"),
    ("hp", ["horsepower"], 550 * FOOT * LBF, POWER, False, "mechanical horsepower, 550 ft lbf/s = 745.69987158227022 W", NIST + ".8"),
    ("hp_metric", ["PS"], 75 * GN, POWER, False, "metric horsepower, 75 kgf m/s = 735.49875 W", NIST + ".8"),
    ("mph", [], MILE / 3600, SPEED, False, "mi/h", NIST + ".8"),
    ("kn", ["knot"], F(1852, 3600), SPEED, False, "nmi/h", NIST + ".8"),
    ("percent", ["%"], F(1, 100), NONE, False, "1/100", "ISO 80000-1"),
    ("ppm", [], F(1, 10 ** 6), NONE, False, "10^-6", "ISO 80000-1"),
]

TEMPERATURES = {
    "K": {"scale": "1", "offset": "0"},
    "degC": {"scale": "1", "offset": "273.15"},
    "degF": {"scale": "5/9", "offset": "459.67"},
    "degR": {"scale": "5/9", "offset": "0"},
}
TEMPERATURE_UNITS = [
    ("degF", ["°F", "fahrenheit"], F(5, 9), dim(K=1), False, "degree Fahrenheit: T/K = (t/°F + 459.67) x 5/9; as a difference 5/9 K", NIST + ".8"),
    ("degR", ["°R", "rankine"], F(5, 9), dim(K=1), False, "degree Rankine, 5/9 K", NIST + ".8"),
]

ANGLES = [
    ("deg", ["°", "degree"], 180, "pi/180 rad"),
    ("arcmin", ["′"], 10800, "(1/60) degree"),
    ("arcsec", ["″"], 648000, "(1/60) arcmin"),
]

PREFIXES = {"Q": "1e30", "R": "1e27", "Y": "1e24", "Z": "1e21", "E": "1e18", "P": "1e15", "T": "1e12", "G": "1e9",
            "M": "1e6", "k": "1e3", "h": "1e2", "da": "1e1", "d": "1e-1", "c": "1e-2", "m": "1e-3", "\u00b5": "1e-6",
            "\u03bc": "1e-6", "u": "1e-6", "n": "1e-9", "p": "1e-12", "f": "1e-15", "a": "1e-18", "z": "1e-21",
            "y": "1e-24", "r": "1e-27", "q": "1e-30"}


def main():
    getcontext().prec = 60
    units = {}
    for symbol, aliases, factor, dims, prefixable, definition, source in UNITS + TEMPERATURE_UNITS:
        units[symbol] = {"factor": str(F(factor)), "dimension": dims, "prefixable": prefixable, "aliases": aliases,
                         "definition": definition, "source": source, "exact": True}
    for symbol, aliases, divisor, definition in ANGLES:
        value = PI / divisor
        units[symbol] = {"factor": None, "factor_decimal": str(value.quantize(Decimal("1e-50"))), "dimension": dim(),
                         "prefixable": False, "aliases": aliases, "definition": definition,
                         "source": SI + ", Table 8", "exact": False}
    prefixes = {p: str(F(v)) for p, v in PREFIXES.items()}
    out = pathlib.Path(__file__).resolve().parents[3] / "catalog/tools/mathematics/unit_convert/units.json"
    payload = {"generated": "2026-10-18", "generator": "tools/catalog/data/unit_convert.py",
               "source": "BIPM SI Brochure, 9th ed. (2019, v2.01 2022); NIST SP 811 (2008) App. B; NIST Handbook 44 App. C; IAU 2012 B2",
               "dimensions": DIMS, "prefixes": prefixes, "temperatures": TEMPERATURES, "units": units}
    out.write_text(json.dumps(payload, indent=1, ensure_ascii=False) + "\n")
    print(f"{out}: {len(units)} units, {len(prefixes)} prefixes")


if __name__ == "__main__":
    main()
