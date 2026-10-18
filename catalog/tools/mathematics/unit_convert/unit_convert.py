#!/usr/bin/env python3
"""Convert a quantity between units with a dimensional check, using the bundled table
units.json (exact definitions from the SI Brochure, 9th ed., and NIST SP 811). Compound units
are parsed ("km/h", "kg*m/s^2", "J/(kg*K)", "kWh", "m²"), SI prefixes attach to SI and
SI-accepted units, and the conversion factor is an exact fraction. Temperatures in degC, degF,
degR and K are converted with their offsets when the whole unit is one temperature; inside a
compound unit (or with temperature "difference") they are temperature differences.

Input (JSON on stdin):
  value        number or decimal string, default 1
  from         unit expression
  to           unit expression; omitted: SI base units
  temperature  "absolute" (default) | "difference"
  precision    significant digits of the decimal rendering of a non-terminating result, default 15
  list_units   true: list the table instead
"""
import json
import pathlib
import re
import sys
from decimal import Decimal, localcontext
from fractions import Fraction

SUPERSCRIPTS = {"⁰": "0", "¹": "1", "²": "2", "³": "3", "⁴": "4", "⁵": "5", "⁶": "6", "⁷": "7", "⁸": "8", "⁹": "9", "⁻": "-"}
TOKEN = re.compile(r"\s*(?:(?P<name>(?:[^\W\d]|[°%′″])+)(?P<attached>-?\d+)?|(?P<number>\d+)|(?P<op>\*\*|[*/^()·×.+-]))")


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def load():
    return json.loads((pathlib.Path(__file__).parent / "units.json").read_text(encoding="utf-8"))


TABLE = load()
DIMS = TABLE["dimensions"]
UNITS = TABLE["units"]
PREFIXES = {p: Fraction(v) for p, v in TABLE["prefixes"].items()}
NAMES = {}
for _symbol, _unit in UNITS.items():
    NAMES[_symbol] = _symbol
    for _alias in _unit["aliases"]:
        NAMES.setdefault(_alias, _symbol)
TEMPERATURE_SYMBOLS = {"K", "degC", "degF", "degR"}


class Quantity:
    def __init__(self, factor=Fraction(1), dims=None, exact=True):
        self.factor, self.dims, self.exact = factor, dims or [0] * len(DIMS), exact

    def mul(self, other, sign=1):
        factor = self.factor * other.factor if sign == 1 else self.factor / other.factor
        return Quantity(factor, [a + sign * b for a, b in zip(self.dims, other.dims)], self.exact and other.exact)

    def power(self, n):
        return Quantity(self.factor ** n, [a * n for a in self.dims], self.exact)


def unit_quantity(symbol, prefix=None):
    unit = UNITS[symbol]
    if unit["exact"]:
        factor, exact = Fraction(unit["factor"]), True
    else:
        factor, exact = Fraction(Decimal(unit["factor_decimal"])), False
    if prefix:
        factor *= PREFIXES[prefix]
    return Quantity(factor, list(unit["dimension"]), exact)


def resolve(name):
    """(symbol, prefix) for a unit name: an exact symbol or alias first, then prefix + unit."""
    if name in NAMES:
        return NAMES[name], None
    for prefix in sorted(PREFIXES, key=len, reverse=True):
        rest = name[len(prefix):]
        if name.startswith(prefix) and rest in NAMES and UNITS[NAMES[rest]]["prefixable"]:
            return NAMES[rest], prefix
    return None, None


def normalise(text):
    out, previous_super = [], False
    for ch in text:
        if ch in SUPERSCRIPTS:
            if not previous_super:
                out.append("^")
            out.append(SUPERSCRIPTS[ch])
            previous_super = True
        else:
            out.append(ch)
            previous_super = False
    return "".join(out)


def tokenize(text):
    text = normalise(text)
    tokens, pos = [], 0
    while pos < len(text):
        if text[pos:].strip() == "":
            break
        m = TOKEN.match(text, pos)
        if not m or m.end() == pos:
            fail(f"cannot read the unit at '{text[pos:].strip()}'")
        if m.group("name"):
            tokens.append(("name", m.group("name")))
            if m.group("attached"):
                tokens.append(("op", "^"))
                tokens.append(("int", int(m.group("attached"))))
        elif m.group("number"):
            tokens.append(("int", int(m.group("number"))))
        else:
            tokens.append(("op", m.group("op")))
        pos = m.end()
    return tokens


class Parser:
    def __init__(self, text):
        self.text = text
        self.tokens = tokenize(text)
        self.i = 0
        self.names = []

    def peek(self):
        return self.tokens[self.i] if self.i < len(self.tokens) else (None, None)

    def take(self):
        tok = self.peek()
        self.i += 1
        return tok

    def parse(self):
        if not self.tokens:
            fail("empty unit")
        q = self.expr()
        if self.i != len(self.tokens):
            fail(f"unexpected '{self.peek()[1]}' in unit '{self.text}'")
        return q

    def expr(self):
        q = self.term()
        while True:
            kind, val = self.peek()
            if kind == "op" and val in ("*", "·", "×", "."):
                self.take()
                q = q.mul(self.term())
            elif kind == "op" and val == "/":
                self.take()
                q = q.mul(self.term(), -1)
            elif kind == "name" or (kind == "op" and val == "("):
                q = q.mul(self.term())  # juxtaposition: "N m"
            else:
                return q

    def term(self):
        q = self.atom()
        kind, val = self.peek()
        if kind == "op" and val in ("^", "**"):
            self.take()
            sign = 1
            kind, val = self.peek()
            if kind == "op" and val in ("-", "+"):
                self.take()
                sign = -1 if val == "-" else 1
            kind, val = self.take()
            if kind != "int":
                fail(f"an exponent must be an integer in '{self.text}'")
            if val > 12:
                fail("exponents above 12 are not supported")
            q = q.power(sign * val)
        return q

    def atom(self):
        kind, val = self.take()
        if kind == "name":
            symbol, prefix = resolve(val)
            if symbol is None:
                fail(f"unknown unit '{val}' (list_units shows the table)")
            self.names.append((symbol, prefix))
            return unit_quantity(symbol, prefix)
        if kind == "int":
            if val == 0:
                fail(f"a numeric factor in '{self.text}' must not be zero")
            return Quantity(Fraction(val))
        if kind == "op" and val == "(":
            q = self.expr()
            kind, val = self.take()
            if (kind, val) != ("op", ")"):
                fail(f"missing ')' in '{self.text}'")
            return q
        fail(f"unexpected '{val}' in unit '{self.text}'")


def dim_string(dims):
    parts = [d if e == 1 else f"{d}^{e}" for d, e in zip(DIMS, dims) if e]
    return " ".join(parts) if parts else "1 (dimensionless)"


def show(d):
    d = d.normalize()
    return format(d, "f") if -7 < d.adjusted() < 21 else str(d)


def decimal_string(fr, precision):
    """(text, rounded?) exact when the decimal expansion terminates within 60 digits."""
    den = fr.denominator
    twos = fives = 0
    while den % 2 == 0:
        den //= 2
        twos += 1
    while den % 5 == 0:
        den //= 5
        fives += 1
    with localcontext() as ctx:
        if den == 1 and max(twos, fives) <= 60 and len(str(abs(fr.numerator))) <= 60:
            ctx.prec = 200
            d = Decimal(fr.numerator) / Decimal(fr.denominator)
            if len(d.normalize().as_tuple().digits) <= 60:
                return show(d), False
        ctx.prec = precision
        d = Decimal(fr.numerator) / Decimal(fr.denominator)
        return show(d), True


def bare_temperature(parser):
    return len(parser.tokens) == 1 and parser.names and parser.names[0][0] in TEMPERATURE_SYMBOLS and parser.names[0][1] is None


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if request.get("list_units"):
        print(json.dumps({"units": {s: {"definition": u["definition"], "aliases": u["aliases"], "prefixable": u["prefixable"],
                                         "dimension": dim_string(u["dimension"]), "source": u["source"]} for s, u in UNITS.items()},
                          "prefixes": TABLE["prefixes"], "generated": TABLE["generated"], "source": TABLE["source"]}, ensure_ascii=False))
        return
    value = request.get("value", 1)
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail("value must be a number or a decimal string")
    try:
        x = Fraction(str(value).strip())
    except (ValueError, ZeroDivisionError):
        fail("value must be a number or a decimal string")
    source, target = request.get("from"), request.get("to")
    if not isinstance(source, str) or not source.strip():
        fail("from must be a unit expression")
    if target is not None and (not isinstance(target, str) or not target.strip()):
        fail("to must be a unit expression")
    mode = request.get("temperature", "absolute")
    if mode not in ("absolute", "difference"):
        fail("temperature must be absolute or difference")
    precision = request.get("precision", 15)
    if isinstance(precision, bool) or not isinstance(precision, int) or not 1 <= precision <= 50:
        fail("precision must be an integer from 1 to 50")
    p_from = Parser(source)
    q_from = p_from.parse()
    if target is None:
        q_to, p_to = Quantity(Fraction(1), list(q_from.dims)), None
        target_text = dim_string(q_from.dims) if any(q_from.dims) else "1"
    else:
        p_to = Parser(target)
        q_to = p_to.parse()
        target_text = target
    if q_from.dims != q_to.dims:
        fail(f"incompatible units: {source} is {dim_string(q_from.dims)}, {target_text} is {dim_string(q_to.dims)}")
    out = {"value": str(value), "from": source, "to": target_text, "dimension": dim_string(q_from.dims)}
    affine = mode == "absolute" and p_to is not None and bare_temperature(p_from) and bare_temperature(p_to)
    if affine:
        t_from, t_to = TABLE["temperatures"][p_from.names[0][0]], TABLE["temperatures"][p_to.names[0][0]]
        kelvin = (x + Fraction(t_from["offset"])) * Fraction(t_from["scale"])
        result = kelvin / Fraction(t_to["scale"]) - Fraction(t_to["offset"])
        out["temperature"] = "absolute: T/K = (t + offset) x scale on each side"
        if kelvin < 0:
            out["warning"] = "below absolute zero"
    else:
        factor = q_from.factor / q_to.factor
        result = x * factor
        text, rounded = decimal_string(factor, max(precision, 15))
        out["factor"] = str(factor) if q_from.exact and q_to.exact else None
        out["factor_decimal"] = text
        if any(s in TEMPERATURE_SYMBOLS - {"K"} for s, _ in p_from.names + (p_to.names if p_to else [])):
            out["temperature"] = "difference: degree units are read as temperature intervals here"
    exact = q_from.exact and q_to.exact
    text, rounded = decimal_string(result, precision)
    out["result"] = text
    out["result_exact"] = str(result) if exact else None
    out["result_is_rounded"] = rounded or not exact
    if not exact:
        out["note"] = "an angle unit in degrees is irrational in radians: the result is a 50-digit approximation"
    out["table"] = {"generated": TABLE["generated"], "source": TABLE["source"]}
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
