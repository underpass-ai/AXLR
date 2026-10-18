#!/usr/bin/env python3
"""Look up the CODATA 2022 recommended values of the fundamental physical constants from the
bundled table constants.json (written from NIST's allascii.txt by
tools/catalog/data/physics_constants.py).

Input (JSON on stdin), one of:
  query    free text: a symbol ("h", "m_e"), an exact name, or keywords ("electron mass")
  name     exact quantity name as NIST prints it (case-insensitive)
  symbol   exact symbol from the bundled symbol list
  list     true: list every name (with symbols), no values
  limit    maximum keyword matches, default 10 (1-400)
"""
import json
import pathlib
import re
import sys
from decimal import Decimal

TABLE = pathlib.Path(__file__).resolve().parent / "constants.json"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def split(text):
    mantissa, _, exponent = text.partition("e")
    return mantissa, exponent


def concise(value, uncertainty):
    """9.1093837139e-31 with 0.0000000028e-31 -> 9.1093837139(28)e-31, when the digits line up."""
    vm, ve = split(value)
    um, ue = split(uncertainty)
    if ve != ue or "." not in vm or "." not in um:
        return None
    if len(vm.split(".")[1]) != len(um.split(".")[1]):
        return None
    digits = um.replace(".", "").lstrip("0")
    if not digits:
        return None
    return f"{vm}({digits})" + (f"e{ve}" if ve else "")


def describe(c):
    out = {"name": c["name"], "symbols": c["symbols"], "value": c["value"], "value_float": float(c["value"]),
           "unit": c["unit"], "exact": c["exact"]}
    if c["exact"]:
        out["uncertainty"] = None
        out["relative_uncertainty"] = 0
        if c["truncated"]:
            out["note"] = "exact, but NIST prints it truncated (more digits follow)"
    else:
        u = Decimal(c["uncertainty"])
        out["uncertainty"] = format(u.normalize(), "e")
        out["uncertainty_float"] = float(u)
        out["relative_uncertainty"] = float(f"{float(u / abs(Decimal(c['value']))):.2e}")
        out["concise"] = concise(c["value"], c["uncertainty"])
    return out


def tokens(text):
    return [t for t in re.split(r"[^a-z0-9]+", text.lower()) if t]


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"query", "name", "symbol", "list", "limit"}
    extra = set(request) - allowed
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    try:
        table = json.loads(TABLE.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        fail(f"cannot read the bundled table: {error}")
    constants = table["constants"]
    source = {"title": table["title"], "source": table["source"], "url": table["source_url"],
              "licence": table["licence"], "fetched": table["fetched"]}
    limit = request.get("limit", 10)
    if isinstance(limit, bool) or not isinstance(limit, int) or not 1 <= limit <= 400:
        fail("limit must be an integer from 1 to 400")
    given = [k for k in ("query", "name", "symbol", "list") if k in request]
    if len(given) != 1:
        fail("give exactly one of query, name, symbol or list")
    key = given[0]
    if key == "list":
        if request["list"] is not True:
            fail("list must be true")
        print(json.dumps({"count": len(constants), "constants": [{"name": c["name"], "symbols": c["symbols"]} for c in constants],
                          "source": source}, ensure_ascii=False))
        return
    text = request[key]
    if not isinstance(text, str) or not text.strip() or len(text) > 200:
        fail(f"{key} must be a non-empty string of at most 200 characters")
    text = text.strip()
    by_symbol = [c for c in constants if text in c["symbols"]] or \
                [c for c in constants if text.lower() in (s.lower() for s in c["symbols"])]
    by_name = [c for c in constants if c["name"].lower() == text.lower()]
    if key == "symbol":
        found, how = by_symbol, "symbol"
    elif key == "name":
        found, how = by_name, "name"
    elif by_name:
        found, how = by_name, "name"
    elif by_symbol:
        found, how = by_symbol, "symbol"
    else:
        words = tokens(text)
        if not words:
            fail("query has no letters or digits")
        hits = []
        for c in constants:
            name_words = tokens(c["name"])
            if all(any(n.startswith(w) for n in name_words) for w in words):
                starts = c["name"].lower().startswith(text.lower())
                hits.append((0 if starts else 1, len(c["name"]), c["name"], c))
        hits.sort(key=lambda h: h[:3])
        found, how = [h[3] for h in hits], "keywords"
    if not found:
        print(json.dumps({"query": text, "match": how, "count": 0, "results": [], "source": source,
                          "hint": "try fewer keywords, or list: true for every name"}, ensure_ascii=False))
        return
    total = len(found)
    print(json.dumps({"query": text, "match": how, "count": total, "results": [describe(c) for c in found[:limit]],
                      "truncated": total > limit, "source": source}, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
