#!/usr/bin/env python3
"""Symbolic mathematics with sympy: solve, simplify, expand, factor, differentiate,
integrate, limit and series, on one expression at a time.

Input (one JSON object on stdin):
  operation   "solve" | "simplify" | "expand" | "factor" | "differentiate" | "integrate"
              | "limit" | "series"
  expression  the expression (or an equation "lhs = rhs" for solve), in sympy syntax:
              x**2, sin(x), exp(x), pi, E, oo; ^ is accepted as a power
  symbol      the variable to solve for, differentiate or integrate by, take the limit or
              series in (default: the single free symbol, or "x")
  order       derivative order (default 1) or series order (default 6)
  point       the limit point or the series expansion point (default 0); "oo" is infinity
  direction   "+" or "-" for a one-sided limit (default two-sided, "+-")
  lower, upper  bounds for a definite integral (both or neither)
Output (JSON on stdout): the result as sympy text, as LaTeX, and when it is a number,
its decimal value to 15 significant digits. Exact arithmetic throughout.
"""
import json
import re
import sys

try:
    import sympy
    from sympy.parsing.sympy_parser import (
        convert_xor,
        implicit_multiplication_application,
        parse_expr,
        standard_transformations,
    )
except ImportError:  # pragma: no cover - the registry's sandbox carries sympy
    print(json.dumps({"error": "sympy is not installed; this tool requires the sympy package"}))
    sys.exit(2)

OPERATIONS = ("solve", "simplify", "expand", "factor", "differentiate", "integrate", "limit", "series")
ALLOWED = re.compile(r"^[0-9A-Za-z_+\-*/^().,= \t]*$")
TRANSFORMATIONS = standard_transformations + (convert_xor, implicit_multiplication_application)


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def parse(text, what):
    """An expression in sympy syntax, with the characters a formula needs and nothing else."""
    if not isinstance(text, str) or not text.strip():
        fail(f"{what} must be a non-empty string")
    if len(text) > 2000:
        fail(f"{what} is longer than 2000 characters")
    if not ALLOWED.match(text) or "__" in text:
        fail(f"{what} may use letters, digits, + - * / ^ ( ) , . = and spaces only")
    try:
        return parse_expr(text, transformations=TRANSFORMATIONS, evaluate=True)
    except Exception as error:  # sympy raises many kinds; the message is what matters
        fail(f"{what} could not be parsed: {error}")


def pick_symbol(request, expr):
    name = request.get("symbol")
    if name is not None:
        if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", name):
            fail("symbol must be an identifier")
        return sympy.Symbol(name)
    free = sorted(expr.free_symbols, key=lambda s: s.name)
    if len(free) == 1:
        return free[0]
    if not free:
        return sympy.Symbol("x")
    fail("symbol is required when the expression has several free symbols: " + ", ".join(s.name for s in free))


def point_of(request):
    raw = request.get("point", 0)
    if isinstance(raw, bool):
        fail("point must be a number or an expression")
    if isinstance(raw, (int, float)):
        return sympy.nsimplify(raw)
    return parse(str(raw), "point")


def describe(value):
    out = {"result": str(value), "latex": sympy.latex(value)}
    try:
        if value.is_number:
            out["decimal"] = str(sympy.N(value, 15))
    except AttributeError:
        pass
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("the input must be a JSON object")
    operation = request.get("operation")
    if operation not in OPERATIONS:
        fail("operation must be one of " + ", ".join(OPERATIONS))
    text = request.get("expression")
    if operation == "solve" and isinstance(text, str) and "=" in text:
        left, _, right = text.partition("=")
        expr = sympy.Eq(parse(left, "expression"), parse(right, "expression"))
    else:
        if isinstance(text, str) and "=" in text:
            fail("only solve accepts an equation; give an expression")
        expr = parse(text, "expression")
    base = expr.lhs - expr.rhs if isinstance(expr, sympy.Eq) else expr
    symbol = pick_symbol(request, base)
    order = request.get("order")

    if operation == "solve":
        solutions = sympy.solve(expr, symbol, dict=False)
        if not isinstance(solutions, list):
            solutions = [solutions]
        result = {
            "operation": operation,
            "symbol": symbol.name,
            "solutions": [describe(s) for s in solutions],
            "count": len(solutions),
        }
    elif operation in ("simplify", "expand", "factor"):
        value = {"simplify": sympy.simplify, "expand": sympy.expand, "factor": sympy.factor}[operation](expr)
        result = {"operation": operation, **describe(value)}
    elif operation == "differentiate":
        n = 1 if order is None else order
        if not isinstance(n, int) or isinstance(n, bool) or n < 1 or n > 10:
            fail("order must be an integer from 1 to 10")
        result = {"operation": operation, "symbol": symbol.name, "order": n, **describe(sympy.diff(expr, symbol, n))}
    elif operation == "integrate":
        lower, upper = request.get("lower"), request.get("upper")
        if (lower is None) != (upper is None):
            fail("give both lower and upper for a definite integral, or neither")
        if lower is None:
            value = sympy.integrate(expr, symbol)
            result = {"operation": operation, "symbol": symbol.name, "definite": False, **describe(value)}
        else:
            a = parse(str(lower), "lower") if not isinstance(lower, (int, float)) else sympy.nsimplify(lower)
            b = parse(str(upper), "upper") if not isinstance(upper, (int, float)) else sympy.nsimplify(upper)
            value = sympy.integrate(expr, (symbol, a, b))
            result = {"operation": operation, "symbol": symbol.name, "definite": True, "lower": str(a), "upper": str(b), **describe(value)}
    elif operation == "limit":
        direction = request.get("direction", "+-")
        if direction not in ("+", "-", "+-"):
            fail("direction must be +, - or +-")
        value = sympy.limit(expr, symbol, point_of(request), dir=direction)
        result = {"operation": operation, "symbol": symbol.name, "point": str(point_of(request)), "direction": direction, **describe(value)}
    else:  # series
        n = 6 if order is None else order
        if not isinstance(n, int) or isinstance(n, bool) or n < 1 or n > 30:
            fail("order must be an integer from 1 to 30")
        value = sympy.series(expr, symbol, point_of(request), n)
        result = {"operation": operation, "symbol": symbol.name, "point": str(point_of(request)), "order": n, **describe(value)}
    result["sympy_version"] = sympy.__version__
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
