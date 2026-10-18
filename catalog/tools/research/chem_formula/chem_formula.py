#!/usr/bin/env python3
"""Parse a chemical formula and compute element counts, molar mass and mass percent from the
bundled IUPAC/CIAAW abridged standard atomic weights (elements.json, written by
tools/catalog/data/chem_formula.py).

Input (JSON on stdin):
  formula    "Ca(OH)2", "K4[Fe(CN)6]", "CuSO4·5H2O" (also . * • as the hydrate dot),
             "SO4^2-", "NH4+", "Fe+++", "SO4 2-", "2H2O" (leading coefficient)
  formulas   a list of formulas instead of one (at most 100)
  decimals   digits of molar_mass_rounded and mass_percent, default 2 (0-6)
  masses     {"Tc": 98} atomic weights for elements without a standard atomic weight, or to override one
"""
import json
import pathlib
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation

TABLE = pathlib.Path(__file__).resolve().parent / "elements.json"
DOTS = "·•∙⋅.*"
SUB = str.maketrans("₀₁₂₃₄₅₆₇₈₉", "0123456789")
SUP = str.maketrans("⁰¹²³⁴⁵⁶⁷⁸⁹⁺⁻", "0123456789+-")
SUP_CHARS = "⁰¹²³⁴⁵⁶⁷⁸⁹"
CLOSE = {"(": ")", "[": "]", "{": "}"}


class FormulaError(Exception):
    pass


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def split_charge(text):
    """Return (formula without charge, charge as int)."""
    patterns = [
        (r"\^\{?(\d*)([+-])\}?$", lambda m: (m.group(1), m.group(2))),
        (r"\^\{?([+-])(\d*)\}?$", lambda m: (m.group(2), m.group(1))),
        (r"\s+(\d*)([+-])$", lambda m: (m.group(1), m.group(2))),
    ]
    for pattern, parts in patterns:
        m = re.search(pattern, text)
        if m:
            digits, sign = parts(m)
            size = int(digits) if digits else 1
            return text[:m.start()], size if sign == "+" else -size
    m = re.search(f"([{SUP_CHARS}]*)([⁺⁻])$", text)
    if m:
        digits = m.group(1).translate(SUP)
        size = int(digits) if digits else 1
        return text[:m.start()], size if m.group(2) == "⁺" else -size
    m = re.search(r"(\++|-+)$", text)
    if m:
        size = len(m.group(1))
        return text[:m.start()], size if m.group(1)[0] == "+" else -size
    return text, 0


class Parser:
    def __init__(self, text, known):
        self.text, self.pos, self.known = text, 0, known

    def peek(self):
        return self.text[self.pos] if self.pos < len(self.text) else ""

    def number(self):
        start = self.pos
        while self.peek().isdigit():
            self.pos += 1
        return int(self.text[start:self.pos]) if self.pos > start else None

    def group(self, closing=None):
        counts = {}
        while True:
            ch = self.peek()
            if ch == "":
                if closing:
                    raise FormulaError(f"missing '{closing}'")
                break
            if ch == closing:
                break
            if ch in CLOSE:
                self.pos += 1
                inner = self.group(CLOSE[ch])
                if self.peek() != CLOSE[ch]:
                    raise FormulaError(f"missing '{CLOSE[ch]}'")
                self.pos += 1
                n = self.number()
                if n == 0:
                    raise FormulaError(f"zero count after '{CLOSE[ch]}' at position {self.pos}")
                for symbol, c in inner.items():
                    counts[symbol] = counts.get(symbol, 0) + c * (n or 1)
            elif ch.isupper():
                start = self.pos
                self.pos += 1
                if self.peek().islower():
                    self.pos += 1
                symbol = self.text[start:self.pos]
                if symbol not in self.known:
                    raise FormulaError(f"unknown element '{symbol}' at position {start + 1}")
                n = self.number()
                if n == 0:
                    raise FormulaError(f"zero count after {symbol}")
                counts[symbol] = counts.get(symbol, 0) + (n or 1)
            elif ch in ")]}":
                raise FormulaError(f"unexpected '{ch}' at position {self.pos + 1}")
            elif ch.islower():
                raise FormulaError(f"element symbols start with a capital letter: '{ch}' at position {self.pos + 1}")
            elif ch.isdigit():
                raise FormulaError(f"a count must follow an element or a bracket (position {self.pos + 1})")
            else:
                raise FormulaError(f"unexpected character '{ch}' at position {self.pos + 1}")
        return counts


def parse(formula, known):
    text = formula.strip().translate(SUB)
    if not text:
        raise FormulaError("empty formula")
    text, charge = split_charge(text)
    text = text.replace(" ", "")
    pieces = re.split(f"[{re.escape(DOTS)}]", text)
    total, parts = {}, []
    for piece in pieces:
        if not piece:
            raise FormulaError("empty part around a hydrate dot")
        m = re.match(r"\d+", piece)
        coefficient = int(m.group()) if m else 1
        body = piece[m.end():] if m else piece
        if coefficient == 0 or not body:
            raise FormulaError(f"bad part '{piece}'")
        parser = Parser(body, known)
        counts = parser.group()
        if parser.pos != len(body):
            raise FormulaError(f"unexpected '{body[parser.pos]}'")
        parts.append({"part": piece, "coefficient": coefficient})
        for symbol, c in counts.items():
            total[symbol] = total.get(symbol, 0) + c * coefficient
    return total, charge, parts


def hill(counts, charge):
    symbols = sorted(counts)
    if "C" in counts:
        symbols = ["C"] + (["H"] if "H" in counts else []) + [s for s in symbols if s not in ("C", "H")]
    text = "".join(s + (str(counts[s]) if counts[s] > 1 else "") for s in symbols)
    if charge:
        text += f"^{abs(charge) if abs(charge) > 1 else ''}{'+' if charge > 0 else '-'}"
    return text, symbols


def compute(formula, table, overrides, decimals):
    known = {e["symbol"]: e for e in table}
    counts, charge, parts = parse(formula, known)
    text, order = hill(counts, charge)
    q = Decimal(1).scaleb(-decimals)
    total, unc, rows = Decimal(0), Decimal(0), []
    for symbol in order:
        e = known[symbol]
        if symbol in overrides:
            weight, u, origin = overrides[symbol], None, "input"
        elif e["weight"] is None:
            raise FormulaError(f"{e['name']} ({symbol}) has no standard atomic weight; give it in masses")
        else:
            weight, u, origin = Decimal(e["weight"]), Decimal(e["uncertainty"]), "CIAAW 2024"
        mass = weight * counts[symbol]
        total += mass
        if u is not None:
            unc += u * counts[symbol]
        rows.append({"symbol": symbol, "name": e["name"], "count": counts[symbol], "atomic_weight": str(weight),
                     "weight_source": origin, "mass": str(mass)})
    for row in rows:
        row["mass_percent"] = str((Decimal(row["mass"]) * 100 / total).quantize(q, ROUND_HALF_UP))
    out = {"formula": formula, "hill": text, "charge": charge, "parts": parts,
           "counts": counts, "atoms": sum(counts.values()), "elements": rows,
           "molar_mass": str(total), "molar_mass_rounded": str(total.quantize(q, ROUND_HALF_UP)), "unit": "g/mol",
           "abridged_uncertainty_sum": str(unc)}
    if charge:
        out["note"] = "molar mass of the ion ignores the mass of the electrons gained or lost"
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"formula", "formulas", "decimals", "masses"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    try:
        data = json.loads(TABLE.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        fail(f"cannot read the bundled table: {error}")
    table = data["elements"]
    decimals = request.get("decimals", 2)
    if isinstance(decimals, bool) or not isinstance(decimals, int) or not 0 <= decimals <= 6:
        fail("decimals must be an integer from 0 to 6")
    overrides = {}
    masses = request.get("masses", {})
    if not isinstance(masses, dict):
        fail("masses must be an object of element symbol to atomic weight")
    symbols = {e["symbol"] for e in table}
    for symbol, value in masses.items():
        if symbol not in symbols:
            fail(f"masses: unknown element '{symbol}'")
        if isinstance(value, bool) or not isinstance(value, (int, float, str)):
            fail(f"masses.{symbol} must be a number")
        try:
            weight = Decimal(str(value).strip())
        except InvalidOperation:
            fail(f"masses.{symbol} must be a number")
        if not weight.is_finite() or weight <= 0:
            fail(f"masses.{symbol} must be positive")
        overrides[symbol] = weight
    source = {"table": data["title"], "basis": data["basis"], "url": data["source_url"], "fetched": data["fetched"]}
    if ("formula" in request) == ("formulas" in request):
        fail("give formula or formulas")
    if "formula" in request:
        formula = request["formula"]
        if not isinstance(formula, str) or len(formula) > 500:
            fail("formula must be a string of at most 500 characters")
        try:
            result = compute(formula, table, overrides, decimals)
        except FormulaError as error:
            fail(f"formula {formula!r}: {error}")
        result["source"] = source
        print(json.dumps(result, ensure_ascii=False))
        return
    formulas = request["formulas"]
    if not isinstance(formulas, list) or not 1 <= len(formulas) <= 100:
        fail("formulas must be a list of 1 to 100 strings")
    results = []
    for i, formula in enumerate(formulas):
        if not isinstance(formula, str) or len(formula) > 500:
            fail(f"formulas[{i}] must be a string of at most 500 characters")
        try:
            results.append(compute(formula, table, overrides, decimals))
        except FormulaError as error:
            results.append({"formula": formula, "error": str(error)})
    print(json.dumps({"results": results, "source": source}, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
