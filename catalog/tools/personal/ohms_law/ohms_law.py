#!/usr/bin/env python3
"""Ohm's law and simple resistor circuits with exact fractions and SI prefixes.

Operations:
  solve    any two of voltage, current, resistance, power -> the other two
           (V = I R, P = V I = I^2 R = V^2 / R)
  network  a resistor network written as an expression: "+" is series, "||" (or "//") parallel,
           parentheses nest: "100 + (220 || 330) + 1k"  ->  equivalent resistance
  divider  unloaded or loaded voltage divider: vin, r1 (top), r2 (bottom), optional load -> vout
  led      series resistor for LEDs: supply, forward voltage, current, LEDs in series (default 1)

Input (JSON on stdin):
  operation   as above
  voltage, current, resistance, power      solve; numbers or strings with SI prefixes ("4.7k", "20m", "12 V")
  expression                               network (also "voltage" to get current and power)
  vin, r1, r2, load                        divider
  supply, forward_voltage, current, count  led
  digits      significant digits of decimal renderings (default 12)
"""
import json
import re
import sys
from decimal import Decimal, InvalidOperation, getcontext
from fractions import Fraction

getcontext().prec = 60
PREFIX = {"p": Fraction(1, 10 ** 12), "n": Fraction(1, 10 ** 9), "u": Fraction(1, 10 ** 6), "µ": Fraction(1, 10 ** 6),
          "μ": Fraction(1, 10 ** 6), "m": Fraction(1, 1000), "": Fraction(1), "k": Fraction(1000), "K": Fraction(1000),
          "M": Fraction(10 ** 6), "G": Fraction(10 ** 9)}
UNIT_WORDS = {"V": "V", "v": "V", "volt": "V", "volts": "V", "A": "A", "amp": "A", "amps": "A", "ampere": "A",
              "Ω": "Ω", "Ω": "Ω", "ohm": "Ω", "ohms": "Ω", "R": "Ω", "W": "W", "watt": "W", "watts": "W"}
SI_OUT = [(Fraction(10 ** 9), "G"), (Fraction(10 ** 6), "M"), (Fraction(1000), "k"), (Fraction(1), ""),
          (Fraction(1, 1000), "m"), (Fraction(1, 10 ** 6), "µ"), (Fraction(1, 10 ** 9), "n"), (Fraction(1, 10 ** 12), "p")]
NUMBER = re.compile(r"(\d+(?:\.\d*)?|\.\d+)(?:[eE]([-+]?\d+))?")
DIGITS = 12


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def quantity(value, field, unit=None):
    """A number, or a string such as "4.7k", "4k7", "20 mA", "12V", "330 Ω"; returns a Fraction."""
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a string such as \"4.7k\"")
    if isinstance(value, (int, float)):
        try:
            return Fraction(Decimal(str(value)))
        except (InvalidOperation, ValueError):
            fail(f"{field} must be finite")
    text = value.strip().replace(" ", "")
    rkm = re.fullmatch(r"(\d+)([RkKMG])(\d+)(Ω|Ω|ohms?)?", text)
    if rkm and (unit in (None, "Ω")):
        scale = Fraction(1) if rkm.group(2) == "R" else PREFIX[rkm.group(2)]
        return Fraction(Decimal(rkm.group(1) + "." + rkm.group(3))) * scale
    match = NUMBER.match(text)
    if not match:
        fail(f"{field}: cannot read {value!r} as a number with an optional SI prefix")
    number = Fraction(Decimal(match.group(1))) * (Fraction(10) ** int(match.group(2)) if match.group(2) else 1)
    rest = text[match.end():]
    for word in sorted(UNIT_WORDS, key=len, reverse=True):
        if rest.endswith(word):
            prefix = rest[: -len(word)]
            if prefix in PREFIX:
                found = UNIT_WORDS[word]
                if unit and found != unit:
                    fail(f"{field}: {value!r} is in {found}, expected {unit}")
                return number * PREFIX[prefix]
    if rest in PREFIX:
        return number * PREFIX[rest]
    fail(f"{field}: cannot read {value!r} (unit or prefix {rest!r} not understood)")


def dec(x, digits):
    if isinstance(x, Fraction):
        value = Decimal(x.numerator) / Decimal(x.denominator)
    else:
        value = x
    text = format(value, f".{digits}g")
    if "e" in text or "E" in text:
        return text
    if "." in text:
        text = text.rstrip("0").rstrip(".")
    return text


def with_prefix(x, unit, digits):
    if x == 0:
        return f"0 {unit}"
    if not isinstance(x, Fraction):
        x = Fraction(x)
    magnitude = abs(x)
    for scale, prefix in SI_OUT:
        if magnitude >= scale:
            return f"{dec(x / scale, digits)} {prefix}{unit}"
    return f"{dec(x / SI_OUT[-1][0], digits)} p{unit}"


def frac_text(x):
    return str(x.numerator) if x.denominator == 1 else f"{x.numerator}/{x.denominator}"


def exact_sqrt(x):
    """Square root of a non-negative Fraction: (Fraction, True) when rational, else (Decimal, False)."""
    def isqrt_exact(n):
        r = int(Decimal(n).sqrt().to_integral_value())
        for c in (r - 1, r, r + 1):
            if c >= 0 and c * c == n:
                return c
        return None
    num, den = isqrt_exact(x.numerator), isqrt_exact(x.denominator)
    if num is not None and den is not None:
        return Fraction(num, den), True
    return (Decimal(x.numerator) / Decimal(x.denominator)).sqrt(), False


def render(name, x, unit, digits, exact=True):
    out = {"value": dec(x, digits), "unit": unit, "si": with_prefix(x, unit, digits)}
    if exact and isinstance(x, Fraction):
        out["exact"] = frac_text(x)
    else:
        out["exact"] = None
    return out


def solve(request, digits):
    keys = [("voltage", "V"), ("current", "A"), ("resistance", "Ω"), ("power", "W")]
    given = {k: quantity(request[k], k, u) for k, u in keys if k in request and request[k] is not None}
    if len(given) != 2:
        fail("solve needs exactly two of voltage, current, resistance, power")
    for k, v in given.items():
        if v < 0:
            fail(f"{k} must not be negative (magnitudes only)")
    V, I, R, P = (given.get(k) for k, _ in keys)
    exact = True
    if "resistance" in given and R == 0 and ("voltage" in given or "power" in given):
        fail("resistance 0 with a voltage or power is a short circuit")
    if V is not None and I is not None:
        if I == 0:
            fail("current 0: resistance is undefined (open circuit)")
        R, P = V / I, V * I
    elif V is not None and R is not None:
        I = V / R
        P = V * I
    elif V is not None and P is not None:
        if V == 0:
            fail("voltage 0 with a power is impossible")
        I = P / V
        if I == 0:
            fail("power 0: resistance is undefined (open circuit)")
        R = V / I
    elif I is not None and R is not None:
        V, P = I * R, I * I * R
    elif I is not None and P is not None:
        if I == 0:
            fail("current 0 with a power is impossible")
        V = P / I
        R = V / I
    else:  # R and P
        V, ok1 = exact_sqrt(P * R)
        I, ok2 = exact_sqrt(P / R)
        exact = ok1 and ok2
    units = {"voltage": "V", "current": "A", "resistance": "Ω", "power": "W"}
    values = {"voltage": V, "current": I, "resistance": R, "power": P}
    result = {"operation": "solve", "given": sorted(given)}
    for k, v in values.items():
        result[k] = render(k, v, units[k], digits, exact=isinstance(v, Fraction))
    result["exact"] = exact
    result["formulas"] = "V = I·R; P = V·I = I²·R = V²/R"
    return result


class Parser:
    """expr := term ('+' term)* ; term := factor (('||' | '//') factor)* ; factor := value | '(' expr ')'."""

    def __init__(self, text):
        self.lexemes = self.lex(text)
        self.pos = 0
        self.count = 0

    @staticmethod
    def lex(text):
        lexemes, i = [], 0
        while i < len(text):
            c = text[i]
            if c.isspace():
                i += 1
            elif text.startswith("||", i) or text.startswith("//", i):
                lexemes.append("||")
                i += 2
            elif c in "+()":
                lexemes.append(c)
                i += 1
            else:
                match = re.match(r"[0-9.]+(?:[eE][-+]?\d+)?[A-Za-zµμΩΩ0-9]*", text[i:])
                if not match:
                    fail(f"expression: unexpected {c!r} at position {i + 1}")
                lexemes.append(("num", match.group(0)))
                i += match.end()
        return lexemes

    def peek(self):
        return self.lexemes[self.pos] if self.pos < len(self.lexemes) else None

    def take(self):
        lexeme = self.peek()
        self.pos += 1
        return lexeme

    def parse(self):
        if not self.lexemes:
            fail("expression is empty")
        value = self.expr(0)
        if self.peek() is not None:
            fail(f"expression: unexpected {self.peek()!r}")
        return value

    def expr(self, depth):
        value = self.term(depth)
        while self.peek() == "+":
            self.take()
            value += self.term(depth)
        return value

    def term(self, depth):
        values = [self.factor(depth)]
        while self.peek() == "||":
            self.take()
            values.append(self.factor(depth))
        if len(values) == 1:
            return values[0]
        if any(v == 0 for v in values):
            return Fraction(0)
        return 1 / sum(1 / v for v in values)

    def factor(self, depth):
        if depth > 100:
            fail("expression nests too deeply")
        lexeme = self.take()
        if lexeme == "(":
            value = self.expr(depth + 1)
            if self.take() != ")":
                fail("expression: missing )")
            return value
        if isinstance(lexeme, tuple):
            self.count += 1
            value = quantity(lexeme[1], "expression", "Ω")
            if value < 0:
                fail("expression: resistances must not be negative")
            return value
        fail(f"expression: expected a resistance or '(' but found {lexeme!r}")


def network(request, digits):
    text = request.get("expression")
    if not isinstance(text, str) or len(text) > 2000:
        fail("network needs expression, a string of at most 2000 characters")
    parser = Parser(text)
    R = parser.parse()
    result = {"operation": "network", "expression": text, "resistors": parser.count,
              "resistance": render("resistance", R, "Ω", digits)}
    if "voltage" in request:
        V = quantity(request["voltage"], "voltage", "V")
        if R == 0:
            fail("the network is a short circuit (0 Ω)")
        I = V / R
        result["voltage"] = render("voltage", V, "V", digits)
        result["current"] = render("current", I, "A", digits)
        result["power"] = render("power", V * I, "W", digits)
    result["rules"] = "series: R = R1 + R2 + ...; parallel: 1/R = 1/R1 + 1/R2 + ..."
    return result


def divider(request, digits):
    for k in ("vin", "r1", "r2"):
        if k not in request:
            fail("divider needs vin, r1 (top) and r2 (bottom); load is optional")
    vin = quantity(request["vin"], "vin", "V")
    r1 = quantity(request["r1"], "r1", "Ω")
    r2 = quantity(request["r2"], "r2", "Ω")
    if r1 < 0 or r2 < 0 or r1 + r2 == 0:
        fail("r1 and r2 must be non-negative and not both 0")
    bottom = r2
    result = {"operation": "divider"}
    if "load" in request:
        load = quantity(request["load"], "load", "Ω")
        if load <= 0:
            fail("load must be positive")
        bottom = r2 * load / (r2 + load) if r2 + load else Fraction(0)
        unloaded = vin * r2 / (r1 + r2)
        result["vout_unloaded"] = render("vout", unloaded, "V", digits)
        result["load"] = render("load", load, "Ω", digits)
    if r1 + bottom == 0:
        fail("the divider is a short circuit")
    vout = vin * bottom / (r1 + bottom)
    current = vin / (r1 + bottom)
    result.update({"vout": render("vout", vout, "V", digits), "ratio": frac_text(bottom / (r1 + bottom)),
                   "current": render("current", current, "A", digits),
                   "power_r1": render("power", current * current * r1, "W", digits),
                   "formula": "Vout = Vin · R2 / (R1 + R2), R2 in parallel with the load when one is given"})
    if r1 + r2 != 0:
        result["thevenin_resistance"] = render("r", r1 * r2 / (r1 + r2), "Ω", digits)
    return result


def led(request, digits):
    for k in ("supply", "forward_voltage", "current"):
        if k not in request:
            fail("led needs supply, forward_voltage and current; count is optional")
    vs = quantity(request["supply"], "supply", "V")
    vf = quantity(request["forward_voltage"], "forward_voltage", "V")
    i = quantity(request["current"], "current", "A")
    count = request.get("count", 1)
    if isinstance(count, bool) or not isinstance(count, int) or count < 1:
        fail("count must be a positive integer")
    if i <= 0:
        fail("current must be positive")
    drop = vs - vf * count
    if drop <= 0:
        fail(f"the supply ({dec(vs, digits)} V) does not exceed the LEDs' forward voltage ({dec(vf * count, digits)} V)")
    R = drop / i
    P = drop * i
    return {"operation": "led", "count": count, "resistor_voltage": render("v", drop, "V", digits),
            "resistance": render("resistance", R, "Ω", digits), "resistor_power": render("power", P, "W", digits),
            "led_power_each": render("power", vf * i, "W", digits),
            "suggested_power_rating": render("power", P * 2, "W", digits),
            "formula": "R = (Vsupply − n·Vf) / I; P = (Vsupply − n·Vf) · I",
            "note": "use the next standard value above R (lower current); the suggested rating doubles the dissipation"}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"operation", "voltage", "current", "resistance", "power", "expression", "vin", "r1", "r2", "load",
               "supply", "forward_voltage", "count", "digits"}
    extra = set(request) - allowed
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    digits = request.get("digits", DIGITS)
    if isinstance(digits, bool) or not isinstance(digits, int) or not 1 <= digits <= 50:
        fail("digits must be an integer from 1 to 50")
    operation = request.get("operation", "solve")
    handlers = {"solve": solve, "network": network, "divider": divider, "led": led}
    if operation not in handlers:
        fail("operation must be solve, network, divider or led")
    print(json.dumps(handlers[operation](request, digits), ensure_ascii=False))


if __name__ == "__main__":
    main()
