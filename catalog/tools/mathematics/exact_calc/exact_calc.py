#!/usr/bin/env python3
"""Exact arithmetic on an expression: the expression is parsed into a syntax tree and only
whitelisted nodes are evaluated (no eval), over Fractions, so 0.1 + 0.2 is exactly 3/10.
Decimal literals are read from their source text. Irrational functions (sqrt of a non-square,
pi, e, ln, exp, trig) fall back to Decimal at the requested precision and mark the result
inexact.

Input (JSON on stdin):
  expression  "1/3 + 1/6", "2**100", "sqrt(2)", "binomial(52, 5)", "x * (1 + r)**n"
  variables   {"x": "1000", "r": "0.05", "n": 3}   optional, numbers or decimal strings
  precision   significant digits of the decimal rendering and of inexact results (default 30)
"""
import ast
import json
import math
import sys
from decimal import Decimal, InvalidOperation, getcontext
from fractions import Fraction

MAX_EXPONENT = 10000
MAX_DIGITS = 100000


class Inexact(Exception):
    pass


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


class Evaluator:
    def __init__(self, source, variables, precision):
        self.source = source
        self.variables = variables
        self.precision = precision
        self.exact = True

    def decimal(self, value):
        if isinstance(value, Fraction):
            return Decimal(value.numerator) / Decimal(value.denominator)
        return value

    def inexact(self, value):
        self.exact = False
        return value

    def integer(self, value, name):
        if isinstance(value, Fraction) and value.denominator == 1:
            return value.numerator
        raise ValueError(f"{name} needs integers")

    def call(self, name, args):
        d = self.decimal
        if name == "sqrt" and len(args) == 1:
            x = args[0]
            if isinstance(x, Fraction) and x >= 0:
                n, m = math.isqrt(x.numerator), math.isqrt(x.denominator)
                if n * n == x.numerator and m * m == x.denominator:
                    return Fraction(n, m)
            if d(x) < 0:
                raise ValueError("sqrt of a negative number")
            return self.inexact(d(x).sqrt())
        if name == "abs" and len(args) == 1:
            return abs(args[0])
        if name in ("floor", "ceil", "trunc") and len(args) == 1:
            x = args[0]
            if isinstance(x, Fraction):
                return Fraction({"floor": math.floor, "ceil": math.ceil, "trunc": math.trunc}[name](x))
            return Fraction(int(x.to_integral_value(rounding={"floor": "ROUND_FLOOR", "ceil": "ROUND_CEILING", "trunc": "ROUND_DOWN"}[name])))
        if name == "round" and len(args) in (1, 2):
            places = self.integer(args[1], "round") if len(args) == 2 else 0
            x = args[0] if isinstance(args[0], Fraction) else Fraction(args[0])
            return Fraction(round(x, places))
        if name in ("min", "max") and args:
            return (min if name == "min" else max)(args, key=lambda v: d(v) if not isinstance(v, Fraction) else v)
        if name in ("gcd", "lcm") and len(args) >= 2:
            ints = [self.integer(a, name) for a in args]
            out = ints[0]
            for i in ints[1:]:
                out = math.gcd(out, i) if name == "gcd" else abs(out * i) // math.gcd(out, i) if out and i else 0
            return Fraction(out)
        if name == "factorial" and len(args) == 1:
            n = self.integer(args[0], name)
            if n < 0 or n > 5000:
                raise ValueError("factorial needs 0 ≤ n ≤ 5000")
            return Fraction(math.factorial(n))
        if name in ("binomial", "comb") and len(args) == 2:
            return Fraction(math.comb(self.integer(args[0], name), self.integer(args[1], name)))
        if name == "perm" and len(args) == 2:
            return Fraction(math.perm(self.integer(args[0], name), self.integer(args[1], name)))
        if name in ("ln", "log", "exp", "log10") and len(args) in (1, 2):
            x = d(args[0])
            if name == "exp":
                return self.inexact(x.exp())
            if x <= 0:
                raise ValueError(f"{name} of a non-positive number")
            if name == "log" and len(args) == 2:
                return self.inexact(x.ln() / d(args[1]).ln())
            return self.inexact(x.log10() if name == "log10" else x.ln())
        if name in ("sin", "cos", "tan", "atan") and len(args) == 1:
            x = float(d(args[0]))
            return self.inexact(Decimal(repr({"sin": math.sin, "cos": math.cos, "tan": math.tan, "atan": math.atan}[name](x))))
        raise ValueError(f"unknown function {name} with {len(args)} arguments")

    def visit(self, node):
        if isinstance(node, ast.Expression):
            return self.visit(node.body)
        if isinstance(node, ast.Constant) and isinstance(node.value, (int, float)) and not isinstance(node.value, bool):
            text = ast.get_source_segment(self.source, node) or repr(node.value)
            return Fraction(text.replace("_", ""))
        if isinstance(node, ast.Name):
            if node.id in self.variables:
                return self.variables[node.id]
            if node.id == "pi":
                return self.inexact(Decimal("3.14159265358979323846264338327950288419716939937510582097494459"))
            if node.id == "e":
                return self.inexact(Decimal(1).exp())
            raise ValueError(f"unknown name {node.id}")
        if isinstance(node, ast.UnaryOp) and isinstance(node.op, (ast.UAdd, ast.USub)):
            value = self.visit(node.operand)
            return value if isinstance(node.op, ast.UAdd) else -value
        if isinstance(node, ast.BinOp):
            a, b = self.visit(node.left), self.visit(node.right)
            if not (isinstance(a, Fraction) and isinstance(b, Fraction)):
                a, b = self.decimal(a), self.decimal(b)
            op = node.op
            if isinstance(op, ast.Add):
                return a + b
            if isinstance(op, ast.Sub):
                return a - b
            if isinstance(op, ast.Mult):
                return a * b
            if isinstance(op, (ast.Div, ast.FloorDiv, ast.Mod)) and b == 0:
                raise ZeroDivisionError("division by zero")
            if isinstance(op, ast.Div):
                return a / b
            if isinstance(op, ast.FloorDiv):
                return Fraction(a // b)
            if isinstance(op, ast.Mod):
                return a % b
            if isinstance(op, ast.Pow):
                return self.power(a, b)
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and not node.keywords:
            return self.call(node.func.id, [self.visit(a) for a in node.args])
        raise ValueError(f"{type(node).__name__} is not allowed in an expression")

    def power(self, a, b):
        if isinstance(b, Fraction) and b.denominator == 1:
            if abs(b.numerator) > MAX_EXPONENT:
                raise ValueError(f"exponents are limited to ±{MAX_EXPONENT}")
            if isinstance(a, Fraction):
                if a == 0 and b < 0:
                    raise ZeroDivisionError("zero to a negative power")
                size = max(abs(a.numerator), a.denominator).bit_length() * abs(b.numerator)
                if size > MAX_DIGITS * 4:
                    raise ValueError("the result would be too large")
                return a ** b.numerator
            return a ** int(b.numerator)
        if isinstance(a, Fraction) and isinstance(b, Fraction) and a > 0:
            root = b.denominator
            n, m = round(a.numerator ** (1 / root)), round(a.denominator ** (1 / root))
            if n ** root == a.numerator and m ** root == a.denominator:
                return self.power(Fraction(n, m), Fraction(b.numerator))
        base = self.decimal(a)
        if base < 0:
            raise ValueError("a negative number to a fractional power")
        if base == 0:
            return Fraction(0)
        return self.inexact((base.ln() * self.decimal(b)).exp())


def render(value, precision):
    if isinstance(value, Fraction):
        out = {"result": str(value), "is_integer": value.denominator == 1}
        decimal = Decimal(value.numerator) / Decimal(value.denominator)
    else:
        out = {"result": None, "is_integer": False}
        decimal = +value
    out["decimal"] = format(decimal, "f") if abs(decimal.adjusted()) < precision else str(decimal)
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    expression = request.get("expression")
    if not isinstance(expression, str) or not expression.strip() or len(expression) > 10000:
        fail("expression must be a non-empty string of at most 10000 characters")
    precision = request.get("precision", 30)
    if not isinstance(precision, int) or isinstance(precision, bool) or not 1 <= precision <= 1000:
        fail("precision must be an integer from 1 to 1000")
    getcontext().prec = precision + 10
    variables = {}
    for name, value in (request.get("variables") or {}).items():
        if not name.isidentifier() or isinstance(value, bool) or not isinstance(value, (int, float, str)):
            fail(f"variable {name} must be a number or a decimal string")
        try:
            variables[name] = Fraction(str(value))
        except (ValueError, ZeroDivisionError):
            fail(f"variable {name} must be a number or a decimal string")
    try:
        tree = ast.parse(expression.replace("^", "**"), mode="eval")
    except SyntaxError as error:
        fail(f"expression does not parse: {error.msg}")
    evaluator = Evaluator(expression.replace("^", "**"), variables, precision)
    try:
        value = evaluator.visit(tree)
    except ZeroDivisionError as error:
        fail(str(error))
    except (ValueError, OverflowError, InvalidOperation) as error:
        fail(str(error) or type(error).__name__)
    getcontext().prec = precision
    out = render(value, precision)
    out.update({"expression": expression, "exact": evaluator.exact and isinstance(value, Fraction)})
    if not out["exact"]:
        out["result"] = None
    print(json.dumps(out))


if __name__ == "__main__":
    main()
