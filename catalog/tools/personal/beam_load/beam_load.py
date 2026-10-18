#!/usr/bin/env python3
"""Statically determinate beams: reactions, shear, bending moment and deflection.

Supports: "simply_supported" (pin and roller, at 0 and L by default, or at `supports` [a, b] for
overhangs) or "cantilever" (fixed at x = 0, free at x = L). Loads act downwards:
  {"type": "point", "magnitude": P, "position": a}
  {"type": "udl", "magnitude": w, "start"?: a, "end"?: b}                       (default whole span)
  {"type": "linear", "start_magnitude": w1, "end_magnitude": w2, "start"?: a, "end"?: b}
Reactions and the shear and moment polynomials are exact fractions (the loads are piecewise
linear, so V is piecewise quadratic and M cubic). With E and I the elastic curve EI y'' = M is
integrated exactly; extremes inside a segment are found by bisection to 50 digits.

Input (JSON on stdin):
  support     "simply_supported" | "cantilever"
  length      L
  supports    [a, b]   simply supported only (default [0, L])
  loads       list as above
  E, I        modulus of elasticity and second moment of area (optional, for deflections)
  units       "SI" (N, m, N/m, Pa, m^4) | "imperial" (lbf, in, lbf/in, psi, in^4) | "consistent"
  points      positions to report (default 11 equally spaced points)
  digits      significant digits (default 10)
Sign convention: x from the left end; downward loads positive; upward reactions positive;
sagging moment positive; shear positive when the part left of the section is pushed up;
deflection reported downward positive.
"""
import json
import sys
from decimal import Decimal, InvalidOperation, getcontext
from fractions import Fraction

getcontext().prec = 60
DISCLAIMER = ("Elastic small-deflection beam theory for a prismatic, statically determinate beam. Not a structural "
              "design check: no strength, stability, shear deformation, load factors, code compliance or "
              "connection design. Have structural work checked by a qualified engineer.")
UNITS = {
    "SI": {"force": "N", "length": "m", "distributed": "N/m", "moment": "N·m", "E": "Pa", "I": "m^4", "deflection": "m"},
    "imperial": {"force": "lbf", "length": "in", "distributed": "lbf/in", "moment": "lbf·in", "E": "psi", "I": "in^4", "deflection": "in"},
    "consistent": {"force": "F", "length": "L", "distributed": "F/L", "moment": "F·L", "E": "F/L^2", "I": "L^4", "deflection": "L"},
}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def num(value, field, positive=False, nonneg=False):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail(f"{field} must be a number or a decimal string")
    try:
        d = Decimal(str(value).strip())
    except InvalidOperation:
        fail(f"{field} must be a number or a decimal string")
    if not d.is_finite():
        fail(f"{field} must be finite")
    x = Fraction(d)
    if positive and x <= 0:
        fail(f"{field} must be positive")
    if nonneg and x < 0:
        fail(f"{field} must not be negative")
    return x


# polynomials: lists of Fraction coefficients, p[k] multiplies x^k
def padd(p, q):
    n = max(len(p), len(q))
    return [(p[k] if k < len(p) else 0) + (q[k] if k < len(q) else 0) for k in range(n)]


def pscale(p, s):
    return [c * s for c in p]


def peval(p, x):
    total = 0
    for c in reversed(p):
        total = total * x + c
    return total


def pint(p, x0, value0):
    """The antiderivative of p that equals value0 at x0."""
    q = [Fraction(0)] + [Fraction(c) / (k + 1) for k, c in enumerate(p)]
    q[0] = value0 - peval(q, x0)
    return q


def pderiv(p):
    return [c * k for k, c in enumerate(p)][1:] or [Fraction(0)]


def fmt(x, digits):
    if isinstance(x, Fraction):
        x = Decimal(x.numerator) / Decimal(x.denominator)
    if x == 0:
        return "0"
    text = format(x, f".{digits}g")
    if "e" in text:
        mantissa, exp = text.split("e")
        if "." in mantissa:
            mantissa = mantissa.rstrip("0").rstrip(".")
        return f"{mantissa}e{int(exp)}"
    if "." in text:
        text = text.rstrip("0").rstrip(".")
    return text


def roots_in(p, a, b):
    """Real roots of polynomial p inside (a, b), by sampling and bisection in Decimal."""
    if all(c == 0 for c in p):
        return []
    dp = [Decimal(c.numerator) / Decimal(c.denominator) for c in p]
    da, db = Decimal(a.numerator) / Decimal(a.denominator), Decimal(b.numerator) / Decimal(b.denominator)

    def f(x):
        total = Decimal(0)
        for c in reversed(dp):
            total = total * x + c
        return total
    n = 96
    xs = [da + (db - da) * i / n for i in range(n + 1)]
    found = []
    for lo, hi in zip(xs, xs[1:]):
        flo, fhi = f(lo), f(hi)
        if flo == 0:
            if lo != da:
                found.append(lo)
            continue
        if flo * fhi < 0:
            for _ in range(190):
                mid = (lo + hi) / 2
                fm = f(mid)
                if fm == 0:
                    lo = hi = mid
                    break
                if (fm < 0) == (flo < 0):
                    lo, flo = mid, fm
                else:
                    hi = mid
            found.append((lo + hi) / 2)
    return found


class Beam:
    def __init__(self, request):
        self.support = request.get("support")
        if self.support not in ("simply_supported", "cantilever"):
            fail("support must be simply_supported or cantilever")
        self.L = num(request.get("length"), "length", positive=True)
        L = self.L
        if self.support == "simply_supported":
            sup = request.get("supports", [0, request.get("length")])
            if not isinstance(sup, list) or len(sup) != 2:
                fail("supports must be [a, b]")
            a, b = num(sup[0], "supports[0]"), num(sup[1], "supports[1]")
            if not 0 <= a < b <= L:
                fail("supports must satisfy 0 <= a < b <= length")
            self.supports = (a, b)
        elif "supports" in request:
            fail("supports applies to simply_supported beams only")
        loads = request.get("loads")
        if not isinstance(loads, list) or not loads:
            fail("loads must be a non-empty list")
        if len(loads) > 100:
            fail("at most 100 loads")
        self.points, self.dists = [], []
        for i, load in enumerate(loads):
            f = f"loads[{i}]"
            if not isinstance(load, dict):
                fail(f"{f} must be an object")
            kind = load.get("type")
            if kind == "point":
                extra = set(load) - {"type", "magnitude", "position"}
                if extra:
                    fail(f"{f}: unknown keys {sorted(extra)}")
                P = num(load.get("magnitude"), f + ".magnitude")
                x = num(load.get("position"), f + ".position")
                if not 0 <= x <= L:
                    fail(f"{f}.position must be within the beam")
                self.points.append((x, P))
            elif kind in ("udl", "linear"):
                allowed = {"type", "magnitude", "start", "end"} if kind == "udl" else {"type", "start_magnitude", "end_magnitude", "start", "end"}
                extra = set(load) - allowed
                if extra:
                    fail(f"{f}: unknown keys {sorted(extra)}")
                s = num(load.get("start", 0), f + ".start")
                e = num(load["end"], f + ".end") if "end" in load else L
                if not 0 <= s < e <= L:
                    fail(f"{f}: need 0 <= start < end <= length")
                if kind == "udl":
                    w1 = w2 = num(load.get("magnitude"), f + ".magnitude")
                else:
                    w1 = num(load.get("start_magnitude"), f + ".start_magnitude")
                    w2 = num(load.get("end_magnitude"), f + ".end_magnitude")
                slope = (w2 - w1) / (e - s)
                self.dists.append((s, e, [w1 - slope * s, slope], w1, w2))
            else:
                fail(f"{f}.type must be point, udl or linear")
        self.reactions()
        self.build()

    def resultants(self):
        """Total downward force and its moment about x = 0."""
        W = sum(P for _, P in self.points)
        Mx = sum(P * x for x, P in self.points)
        for s, e, q, _, _ in self.dists:
            F = pint(q, s, 0)
            W += peval(F, e)
            xq = pint(padd([0], [0] + q), s, 0)  # integral of x q(x)
            Mx += peval(xq, e)
        return W, Mx

    def reactions(self):
        W, Mx = self.resultants()
        self.W = W
        if self.support == "simply_supported":
            a, b = self.supports
            Rb = (Mx - W * a) / (b - a)
            Ra = W - Rb
            self.forces = [(a, Ra), (b, Rb)] if a != b else [(a, W)]
            self.m0 = Fraction(0)
            self.fixed_moment = None
        else:
            self.forces = [(Fraction(0), W)]
            self.m0 = -Mx               # internal moment just right of the fixed end (hogging)
            self.fixed_moment = Mx      # reaction couple, counter-clockwise positive

    def build(self):
        L = self.L
        cuts = {Fraction(0), L}
        cuts.update(x for x, _ in self.points)
        cuts.update(x for x, _ in self.forces)
        for s, e, *_ in self.dists:
            cuts.update((s, e))
        self.cuts = sorted(cuts)
        self.segments = []
        V, M = Fraction(0), self.m0
        for x0, x1 in zip(self.cuts, self.cuts[1:]):
            jump = sum(R for x, R in self.forces if x == x0) - sum(P for x, P in self.points if x == x0)
            V += jump
            q = [Fraction(0)]
            for s, e, poly, _, _ in self.dists:
                if s <= x0 and x1 <= e:
                    q = padd(q, poly)
            Vp = padd(pint(pscale(q, -1), x0, 0), [V])
            Mp = pint(Vp, x0, M)
            self.segments.append({"x0": x0, "x1": x1, "V": Vp, "M": Mp})
            V, M = peval(Vp, x1), peval(Mp, x1)
        self.end_shear = V + sum(R for x, R in self.forces if x == L) - sum(P for x, P in self.points if x == L)
        self.end_moment = M

    def segment(self, x, side="right"):
        for seg in self.segments:
            if (seg["x0"] <= x < seg["x1"]) if side == "right" else (seg["x0"] < x <= seg["x1"]):
                return seg
        return self.segments[-1] if side == "right" else self.segments[0]

    def shear(self, x, side):
        if side == "right" and x == self.L:
            return Fraction(0)
        if side == "left" and x == 0:
            return Fraction(0)
        return peval(self.segment(x, side)["V"], x)

    def moment(self, x):
        return peval(self.segment(x, "right" if x < self.L else "left")["M"], x)

    def elastic(self, EI):
        """Slope and deflection polynomials per segment (EI y'' = M), constants from the supports."""
        T, Y = Fraction(0), Fraction(0)
        base = []
        for seg in self.segments:
            Tp = pint(seg["M"], seg["x0"], T)
            Yp = pint(Tp, seg["x0"], Y)
            base.append((Tp, Yp))
            T, Y = peval(Tp, seg["x1"]), peval(Yp, seg["x1"])
        self.base = base
        if self.support == "cantilever":
            c1 = c2 = Fraction(0)
        else:
            a, b = self.supports
            ya, yb = self.base_y(a), self.base_y(b)
            c1 = -(yb - ya) / (b - a)
            c2 = -ya - c1 * a
        self.c1, self.c2, self.EI = c1, c2, EI
        self.curve = [{"x0": seg["x0"], "x1": seg["x1"],
                       "theta": pscale(padd(Tp, [c1]), 1 / EI),
                       "y": pscale(padd(Yp, [c2, c1]), 1 / EI)} for seg, (Tp, Yp) in zip(self.segments, base)]

    def base_y(self, x):
        for seg, (_, Yp) in zip(self.segments, self.base):
            if seg["x0"] <= x <= seg["x1"]:
                return peval(Yp, x)
        return peval(self.base[-1][1], x)

    def curve_at(self, x):
        for c in self.curve:
            if c["x0"] <= x <= c["x1"]:
                return c
        return self.curve[-1]


def to_dec(x):
    return Decimal(x.numerator) / Decimal(x.denominator) if isinstance(x, Fraction) else x


def deval(p, x):
    total = Decimal(0)
    for c in reversed(p):
        total = total * x + to_dec(c)
    return total


def standard_case(beam):
    """Closed forms of the standard beam tables (AISC Manual Table 3-23; Roark Table 8.1) for one load."""
    L = beam.L
    if beam.points and beam.dists or len(beam.points) + len(beam.dists) != 1:
        return None
    if beam.support == "simply_supported" and beam.supports != (0, L):
        return None
    if beam.dists:
        s, e, _, w1, w2 = beam.dists[0]
        if (s, e) != (0, L):
            return None
        if w1 == w2:
            w = w1
            if beam.support == "simply_supported":
                return {"case": "simple beam, uniformly distributed load", "max_moment": w * L * L / 8,
                        "max_moment_formula": "wL²/8 at L/2", "deflection_coeff": Fraction(5, 384) * w * L ** 4,
                        "deflection_formula": "5wL⁴/(384EI) at L/2"}
            return {"case": "cantilever, uniformly distributed load", "max_moment": -w * L * L / 2,
                    "max_moment_formula": "-wL²/2 at the fixed end", "deflection_coeff": w * L ** 4 / 8,
                    "deflection_formula": "wL⁴/(8EI) at the free end"}
        if beam.support == "simply_supported" and (w1 == 0 or w2 == 0):
            w = w1 or w2
            Wt = w * L / 2
            return {"case": "simple beam, load increasing uniformly to one end", "reactions": [Wt / 3, 2 * Wt / 3] if w1 == 0 else [2 * Wt / 3, Wt / 3],
                    "max_moment_formula": "2WL/(9√3) = wL²/(9√3) at L/√3 from the zero end (W = wL/2)",
                    "max_moment_decimal": Decimal(2) * to_dec(Wt * L) / (9 * Decimal(3).sqrt()),
                    "deflection_formula": "0.01304 WL³/(EI) at 0.5193 L from the zero end",
                    "deflection_coeff_decimal": Decimal("0.01304") * to_dec(Wt * L ** 3)}
        return None
    x, P = beam.points[0]
    if beam.support == "simply_supported":
        a, b = x, L - x
        out = {"case": "simple beam, concentrated load at any point", "max_moment": P * a * b / L,
               "max_moment_formula": "Pab/L under the load"}
        if a == b:
            out.update({"case": "simple beam, concentrated load at centre", "max_moment_formula": "PL/4 at L/2",
                        "deflection_coeff": P * L ** 3 / 48, "deflection_formula": "PL³/(48EI) at L/2"})
        return out
    if x == L:
        return {"case": "cantilever, concentrated load at free end", "max_moment": -P * L,
                "max_moment_formula": "-PL at the fixed end", "deflection_coeff": P * L ** 3 / 3,
                "deflection_formula": "PL³/(3EI) at the free end"}
    return {"case": "cantilever, concentrated load at any point", "max_moment": -P * x,
            "max_moment_formula": "-Pa at the fixed end", "deflection_coeff": P * x * x * (3 * L - x) / 6,
            "deflection_formula": "Pa²(3L - a)/(6EI) at the free end"}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"support", "length", "supports", "loads", "E", "I", "units", "points", "digits"}
    extra = set(request) - allowed
    if extra:
        fail(f"unknown fields: {sorted(extra)}")
    units_name = request.get("units", "SI")
    if units_name not in UNITS:
        fail("units must be SI, imperial or consistent")
    u = UNITS[units_name]
    digits = request.get("digits", 10)
    if isinstance(digits, bool) or not isinstance(digits, int) or not 3 <= digits <= 40:
        fail("digits must be an integer from 3 to 40")
    beam = Beam(request)
    L = beam.L
    EI = None
    if ("E" in request) != ("I" in request):
        fail("give both E and I for deflections, or neither")
    if "E" in request:
        EI = num(request["E"], "E", positive=True) * num(request["I"], "I", positive=True)
        beam.elastic(EI)
    pts = request.get("points")
    if pts is None:
        pts_f = [L * k / 10 for k in range(11)]
    else:
        if not isinstance(pts, list) or not pts or len(pts) > 1000:
            fail("points must be a list of 1 to 1000 positions")
        pts_f = [num(p, f"points[{i}]") for i, p in enumerate(pts)]
        if any(not 0 <= p <= L for p in pts_f):
            fail("points must lie on the beam (0 to length)")
    F = lambda x: fmt(x, digits)

    table = []
    for x in pts_f:
        vl, vr = beam.shear(x, "left"), beam.shear(x, "right")
        row = {"x": F(x), "moment": F(beam.moment(x))}
        if vl == vr:
            row["shear"] = F(vr)
        else:
            row["shear_left"], row["shear_right"] = F(vl), F(vr)
        if EI is not None:
            c = beam.curve_at(x)
            row["deflection"] = F(-peval(c["y"], x))
            row["slope"] = F(peval(c["theta"], x))
        table.append(row)

    # extremes of moment: segment ends and zeros of the shear
    cands = []
    for seg in beam.segments:
        for x in (seg["x0"], seg["x1"]):
            cands.append((to_dec(x), to_dec(peval(seg["M"], x))))
        for r in roots_in(seg["V"], seg["x0"], seg["x1"]):
            cands.append((r, deval(seg["M"], r)))
    mmax = max(cands, key=lambda c: c[1])
    mmin = min(cands, key=lambda c: c[1])
    vcands = []
    for seg in beam.segments:
        for x in (seg["x0"], seg["x1"]):
            vcands.append((to_dec(x), to_dec(peval(seg["V"], x))))
        for r in roots_in(pderiv(seg["V"]), seg["x0"], seg["x1"]):
            vcands.append((r, deval(seg["V"], r)))
    vext = max(vcands, key=lambda c: abs(c[1]))
    result = {
        "support": beam.support, "length": F(L), "units": u,
        "total_load": F(beam.W),
        "reactions": [{"position": F(x), "force": F(R)} for x, R in beam.forces],
        "max_moment": {"value": F(mmax[1]), "position": F(mmax[0])},
        "min_moment": {"value": F(mmin[1]), "position": F(mmin[0])},
        "max_abs_shear": {"value": F(abs(vext[1])), "position": F(vext[0])},
        "points": table,
    }
    if beam.fixed_moment is not None:
        result["reactions"][0]["moment"] = F(beam.fixed_moment)
        result["reactions"][0]["moment_note"] = "reaction couple at the fixed end, counter-clockwise positive (resists the loads)"
    if EI is not None:
        dcands = []
        for c in beam.curve:
            for x in (c["x0"], c["x1"]):
                dcands.append((to_dec(x), -to_dec(peval(c["y"], x))))
            for r in roots_in(c["theta"], c["x0"], c["x1"]):
                dcands.append((r, -deval(c["y"], r)))
        dmax = max(dcands, key=lambda c: abs(c[1]))
        result["EI"] = F(EI)
        result["max_deflection"] = {"value": F(dmax[1]), "position": F(dmax[0]), "direction": "down" if dmax[1] > 0 else "up"}
    case = standard_case(beam)
    if case:
        check = {"case": case["case"], "max_moment_formula": case["max_moment_formula"]}
        if "max_moment" in case:
            check["max_moment"] = F(case["max_moment"])
        if "max_moment_decimal" in case:
            check["max_moment"] = F(case["max_moment_decimal"])
        if "reactions" in case:
            check["reactions"] = [F(r) for r in case["reactions"]]
        if "deflection_formula" in case:
            check["deflection_formula"] = case["deflection_formula"]
            if EI is not None:
                coeff = case.get("deflection_coeff")
                check["max_deflection"] = F(coeff / EI) if coeff is not None else F(case["deflection_coeff_decimal"] / to_dec(EI))
        result["table_check"] = check
    result["sign_convention"] = ("x from the left end; loads down positive; reactions up positive; sagging moment positive; "
                                 "deflection down positive")
    result["disclaimer"] = DISCLAIMER
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
