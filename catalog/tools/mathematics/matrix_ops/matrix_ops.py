#!/usr/bin/env python3
"""Exact matrix arithmetic over Fractions: determinant, inverse, rank, reduced row echelon form,
transpose, product, and the solution of A x = b classified as unique, none or infinitely many
(with the general solution in terms of the free variables). Gaussian elimination throughout,
so 0.1 is 1/10 and a singular matrix is detected exactly, never by a tolerance.

Input (JSON on stdin):
  operation  "determinant" | "inverse" | "rank" | "rref" | "solve" | "transpose" | "multiply"
  matrix     [[...], ...] rows of numbers, decimal strings or fraction strings ("1/3")
  b          right-hand side vector (solve)
  other      second matrix (multiply: matrix x other)
Entries in the output are fraction strings.
"""
import json
import sys
from fractions import Fraction

MAX_DIM = 30


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def entry(v, where):
    if isinstance(v, bool) or not isinstance(v, (int, float, str)):
        fail(f"{where} is not a number")
    try:
        return Fraction(str(v).strip())
    except (ValueError, ZeroDivisionError):
        fail(f"{where} is not a number")


def read_matrix(value, field):
    if not isinstance(value, list) or not value or not all(isinstance(r, list) for r in value):
        fail(f"{field} must be a non-empty list of rows")
    width = len(value[0])
    if width == 0 or any(len(r) != width for r in value):
        fail(f"{field}: every row must have the same, non-zero length")
    if len(value) > MAX_DIM or width > MAX_DIM:
        fail(f"{field} may be at most {MAX_DIM} x {MAX_DIM}")
    return [[entry(v, f"{field}[{i}][{j}]") for j, v in enumerate(r)] for i, r in enumerate(value)]


def show(m):
    return [[str(v) for v in row] for row in m]


def rref(m):
    """Reduced row echelon form, pivot columns, and the determinant factor of the row operations."""
    a = [row[:] for row in m]
    rows, cols = len(a), len(a[0])
    pivots, r, det = [], 0, Fraction(1)
    for c in range(cols):
        if r == rows:
            break
        p = next((i for i in range(r, rows) if a[i][c] != 0), None)
        if p is None:
            continue
        if p != r:
            a[r], a[p] = a[p], a[r]
            det = -det
        pv = a[r][c]
        det *= pv
        a[r] = [v / pv for v in a[r]]
        for i in range(rows):
            if i != r and a[i][c] != 0:
                f = a[i][c]
                a[i] = [x - f * y for x, y in zip(a[i], a[r])]
        pivots.append(c)
        r += 1
    return a, pivots, det


def determinant(m):
    if len(m) != len(m[0]):
        fail("the determinant needs a square matrix")
    _, pivots, det = rref(m)
    return det if len(pivots) == len(m) else Fraction(0)


def term(coef, name):
    if coef == 1:
        return name
    if coef == -1:
        return f"-{name}"
    return f"{coef}*{name}"


def solve(m, b):
    rows, cols = len(m), len(m[0])
    if len(b) != rows:
        fail(f"b must have {rows} entries, one per row")
    aug, pivots, _ = rref([row + [v] for row, v in zip(m, b)])
    names = [f"x{j + 1}" for j in range(cols)]
    rank_a = len([p for p in pivots if p < cols])
    if cols in pivots:
        return {"solution_type": "none", "rank": rank_a, "rank_augmented": rank_a + 1,
                "reason": "the augmented matrix has a pivot in the right-hand column: the equations are inconsistent", "rref_augmented": show(aug)}
    if rank_a == cols:
        x = [Fraction(0)] * cols
        for i, p in enumerate(pivots):
            x[p] = aug[i][cols]
        return {"solution_type": "unique", "rank": rank_a, "solution": [str(v) for v in x],
                "solution_decimal": [float(v) for v in x]}
    free = [j for j in range(cols) if j not in pivots]
    particular = [Fraction(0)] * cols
    for i, p in enumerate(pivots):
        particular[p] = aug[i][cols]
    basis, equations = [], []
    for f in free:
        v = [Fraction(0)] * cols
        v[f] = Fraction(1)
        for i, p in enumerate(pivots):
            v[p] = -aug[i][f]
        basis.append([str(t) for t in v])
    for i, p in enumerate(pivots):
        parts = [str(aug[i][cols])] if aug[i][cols] != 0 else []
        for f in free:
            c = -aug[i][f]
            if c != 0:
                t = term(abs(c), names[f])
                parts.append(("- " if c < 0 else "+ ") + t if parts else (t if c > 0 else f"-{t}"))
        equations.append(f"{names[p]} = " + (" ".join(parts) if parts else "0"))
    return {"solution_type": "infinite", "rank": rank_a, "free_variables": [names[f] for f in free],
            "general_solution": equations + [f"{names[f]} free" for f in free],
            "particular": [str(v) for v in particular], "null_space_basis": basis,
            "parametric": "x = particular + sum of t_i * null_space_basis[i], each t_i any number"}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    op = request.get("operation")
    ops = ("determinant", "inverse", "rank", "rref", "solve", "transpose", "multiply")
    if op not in ops:
        fail("operation must be " + ", ".join(ops))
    m = read_matrix(request.get("matrix"), "matrix")
    rows, cols = len(m), len(m[0])
    out = {"operation": op, "shape": [rows, cols]}
    if op == "determinant":
        out["determinant"] = str(determinant(m))
    elif op == "inverse":
        if rows != cols:
            fail("only a square matrix has an inverse")
        det = determinant(m)
        out["determinant"] = str(det)
        if det == 0:
            out["invertible"] = False
            out["inverse"] = None
        else:
            identity = [[Fraction(int(i == j)) for j in range(rows)] for i in range(rows)]
            reduced, _, _ = rref([r + e for r, e in zip(m, identity)])
            out["invertible"] = True
            out["inverse"] = show([r[cols:] for r in reduced])
    elif op in ("rank", "rref"):
        reduced, pivots, _ = rref(m)
        out["rank"] = len(pivots)
        out["nullity"] = cols - len(pivots)
        if op == "rref":
            out["rref"] = show(reduced)
            out["pivot_columns"] = [p + 1 for p in pivots]
    elif op == "solve":
        b = request.get("b")
        if not isinstance(b, list) or not b:
            fail("b must be a list of numbers")
        out.update(solve(m, [entry(v, f"b[{i}]") for i, v in enumerate(b)]))
    elif op == "transpose":
        out["result"] = show([list(c) for c in zip(*m)])
        out["result_shape"] = [cols, rows]
    else:
        other = read_matrix(request.get("other"), "other")
        if len(other) != cols:
            fail(f"cannot multiply {rows}x{cols} by {len(other)}x{len(other[0])}: inner sizes differ")
        out["result"] = show([[sum((a * b for a, b in zip(row, col)), Fraction(0)) for col in zip(*other)] for row in m])
        out["result_shape"] = [rows, len(other[0])]
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
