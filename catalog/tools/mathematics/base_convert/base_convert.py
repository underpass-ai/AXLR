#!/usr/bin/env python3
"""Number representations: positional bases 2 to 36 (integers and fractions, with the repeating
block of a fraction found exactly), Roman numerals both ways with strict validation, two's
complement of a given bit width both ways, and 8421 binary-coded decimal both ways.

Input (JSON on stdin):
  operation  "convert" | "to_roman" | "from_roman" | "to_twos_complement" |
             "from_twos_complement" | "to_bcd" | "from_bcd"
  value      the number: a string of digits in from_base (convert; "-FF.8", "0x1F" when
             from_base is omitted), an integer (to_roman, to_twos_complement, to_bcd), a Roman
             numeral (from_roman) or a bit string (from_twos_complement, from_bcd)
  from_base  2..36, default 10 (or read from a 0x, 0o, 0b prefix)
  to_base    2..36 or a list of them, default [2, 8, 10, 16]
  fraction_digits  digit limit for a fractional part, default 32 (max 256)
  bits       width for to_twos_complement (1..4096)
"""
import json
import re
import sys
from fractions import Fraction

if hasattr(sys, "set_int_max_str_digits"):
    sys.set_int_max_str_digits(0)

DIGITS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
MAX_LENGTH = 2000
ROMAN = re.compile(r"^M{0,3}(CM|CD|D?C{0,3})(XC|XL|L?X{0,3})(IX|IV|V?I{0,3})$")
ROMAN_VALUES = [(1000, "M"), (900, "CM"), (500, "D"), (400, "CD"), (100, "C"), (90, "XC"), (50, "L"), (40, "XL"),
                (10, "X"), (9, "IX"), (5, "V"), (4, "IV"), (1, "I")]


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def base_arg(value, field):
    if isinstance(value, bool) or not isinstance(value, int) or not 2 <= value <= 36:
        fail(f"{field} must be an integer from 2 to 36")
    return value


def int_value(request, low=None, high=None):
    value = request.get("value")
    if isinstance(value, str) and re.fullmatch(r"\s*[+-]?\d+\s*", value) and len(value) <= MAX_LENGTH:
        value = int(value)
    if isinstance(value, bool) or not isinstance(value, int):
        fail("value must be an integer")
    if (low is not None and value < low) or (high is not None and value > high):
        fail(f"value must be from {low} to {high}")
    return value


def parse(text, base):
    """A Fraction from a digit string in the given base, with optional sign and point."""
    s = text.strip().replace("_", "")
    sign = 1
    if s and s[0] in "+-":
        sign, s = (-1 if s[0] == "-" else 1), s[1:]
    if "/" in s:
        top, _, bottom = s.partition("/")
        if "/" in bottom or "." in top + bottom or not top or not bottom:
            fail("a fraction value must be digits/digits")
        den = parse(bottom, base)
        if den == 0:
            fail("the denominator is zero")
        return sign * parse(top, base) / den
    if not s or s == ".":
        fail("value has no digits")
    if s.count(".") > 1:
        fail("value has more than one point")
    whole, _, frac = s.partition(".")
    total = Fraction(0)
    for ch in whole + frac:
        d = DIGITS.find(ch.upper())
        if d < 0 or d >= base:
            fail(f"'{ch}' is not a digit in base {base}")
        total = total * base + d
    return sign * total / base ** len(frac)


def int_to_base(n, base):
    if n == 0:
        return "0"
    out = []
    while n:
        n, d = divmod(n, base)
        out.append(DIGITS[d])
    return "".join(reversed(out))


def to_base(x, base, limit):
    """Digits of x in base, the fractional part with its repeating block in parentheses."""
    sign = "-" if x < 0 else ""
    x = abs(x)
    whole = x.numerator // x.denominator
    r, d = x.numerator % x.denominator, x.denominator
    digits, seen, repeat_at, truncated = [], {}, None, False
    while r:
        if r in seen:
            repeat_at = seen[r]
            break
        if len(digits) == limit:
            truncated = True
            break
        seen[r] = len(digits)
        r *= base
        digits.append(DIGITS[r // d])
        r %= d
    text = sign + int_to_base(whole, base)
    if digits:
        if repeat_at is None:
            text += "." + "".join(digits) + ("..." if truncated else "")
        else:
            text += "." + "".join(digits[:repeat_at]) + "(" + "".join(digits[repeat_at:]) + ")"
    return {"digits": text, "terminates": not r and not truncated and repeat_at is None,
            "repeating": repeat_at is not None, "truncated": truncated}


def convert(request):
    value = request.get("value")
    if isinstance(value, bool) or not isinstance(value, (str, int)):
        fail("value must be a string of digits (or an integer in base 10)")
    text = str(value)
    if len(text) > MAX_LENGTH:
        fail(f"value may have at most {MAX_LENGTH} characters")
    if "from_base" in request:
        base = base_arg(request["from_base"], "from_base")
    else:
        base = 10
        m = re.match(r"^\s*([+-]?)0([xXoObB])(.*)$", text)
        if m:
            base = {"x": 16, "o": 8, "b": 2}[m.group(2).lower()]
            text = m.group(1) + m.group(3)
    targets = request.get("to_base", [2, 8, 10, 16])
    single = not isinstance(targets, list)
    targets = [targets] if single else targets
    if not targets or len(targets) > 35:
        fail("to_base must be a base or a list of up to 35 bases")
    targets = [base_arg(t, "to_base") for t in targets]
    limit = request.get("fraction_digits", 32)
    if isinstance(limit, bool) or not isinstance(limit, int) or not 0 <= limit <= 256:
        fail("fraction_digits must be an integer from 0 to 256")
    x = parse(text, base)
    results = {str(t): to_base(x, t, limit) for t in targets}
    out = {"operation": "convert", "from_base": base, "value": str(x), "is_integer": x.denominator == 1}
    if single:
        out["result"] = results[str(targets[0])]["digits"]
        out.update({k: v for k, v in results[str(targets[0])].items() if k != "digits"})
        out["to_base"] = targets[0]
    else:
        out["results"] = {k: v["digits"] for k, v in results.items()}
        out["details"] = results
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    op = request.get("operation")
    if op == "convert":
        out = convert(request)
    elif op == "to_roman":
        n = int_value(request, 1, 3999)
        parts, rest = [], n
        for v, sym in ROMAN_VALUES:
            while rest >= v:
                parts.append(sym)
                rest -= v
        out = {"operation": op, "value": n, "roman": "".join(parts)}
    elif op == "from_roman":
        text = request.get("value")
        if not isinstance(text, str) or not text.strip():
            fail("value must be a Roman numeral")
        s = text.strip().upper()
        if not ROMAN.match(s):
            fail(f"'{text}' is not a standard Roman numeral (I V X L C D M, subtractive pairs IV IX XL XC CD CM only, 1 to 3999)")
        values = dict((sym, v) for v, sym in ROMAN_VALUES if len(sym) == 1)
        total = 0
        for i, ch in enumerate(s):
            v = values[ch]
            total += -v if i + 1 < len(s) and values[s[i + 1]] > v else v
        out = {"operation": op, "roman": s, "value": total}
    elif op == "to_twos_complement":
        bits = request.get("bits")
        if isinstance(bits, bool) or not isinstance(bits, int) or not 1 <= bits <= 4096:
            fail("bits must be an integer from 1 to 4096")
        n = int_value(request, -(1 << (bits - 1)), (1 << (bits - 1)) - 1)
        pattern = n & ((1 << bits) - 1)
        binary = format(pattern, f"0{bits}b")
        out = {"operation": op, "value": n, "bits": bits, "binary": binary,
               "hex": format(pattern, f"0{(bits + 3) // 4}X"), "unsigned": pattern,
               "range": [-(1 << (bits - 1)), (1 << (bits - 1)) - 1]}
    elif op in ("from_twos_complement", "from_bcd"):
        text = request.get("value")
        if not isinstance(text, str):
            fail("value must be a string of bits")
        bitstr = re.sub(r"[\s_]", "", text)
        if not bitstr or len(bitstr) > 4096 or set(bitstr) - {"0", "1"}:
            fail("value must be a string of 0 and 1 (spaces and underscores allowed), at most 4096 bits")
        if op == "from_twos_complement":
            raw, bits = int(bitstr, 2), len(bitstr)
            n = raw - (1 << bits) if bitstr[0] == "1" else raw
            out = {"operation": op, "binary": bitstr, "bits": bits, "value": n, "unsigned": raw}
        else:
            if len(bitstr) % 4:
                fail("BCD needs a multiple of 4 bits")
            digits = []
            for i in range(0, len(bitstr), 4):
                nibble = int(bitstr[i:i + 4], 2)
                if nibble > 9:
                    fail(f"nibble {bitstr[i:i + 4]} at bit {i} is not a decimal digit (1010 to 1111 are invalid in BCD)")
                digits.append(str(nibble))
            out = {"operation": op, "binary": bitstr, "value": int("".join(digits)), "digits": "".join(digits)}
    elif op == "to_bcd":
        n = int_value(request, 0)
        if len(str(n)) > 1000:
            fail("value may have at most 1000 digits")
        groups = [format(int(d), "04b") for d in str(n)]
        out = {"operation": op, "value": n, "bcd": " ".join(groups), "bits": 4 * len(groups)}
    else:
        fail("operation must be convert, to_roman, from_roman, to_twos_complement, from_twos_complement, to_bcd or from_bcd")
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except (ArithmeticError, ValueError, RecursionError) as error:
        fail(f"the numbers are out of the range this tool can compute: {type(error).__name__}")
