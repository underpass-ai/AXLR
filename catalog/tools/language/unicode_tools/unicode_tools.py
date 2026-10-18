#!/usr/bin/env python3
"""Unicode inspection with the standard library's unicodedata: the four normalisation forms,
accent stripping, code points with names and general categories, UTF-8 / UTF-16 / UTF-32 bytes
in hex (and back), and a homoglyph check: Cyrillic and Greek letters that look like Latin ones
(pairs taken from Unicode TR39 confusables.txt) and words that mix scripts.

Input (JSON on stdin):
  operation  "normalize" | "strip_accents" | "codepoints" | "bytes" | "decode" | "confusables"
  text       the string (every operation except decode)
  form       "NFC" | "NFD" | "NFKC" | "NFKD"          normalize; omitted: all four
  hex        "C3 A9" or "c3a9"                         decode
  encoding   "utf-8" | "utf-16-be" | "utf-16-le" | "utf-32-be" | "utf-32-le"   decode (default utf-8)

Every answer reports unicodedata.unidata_version: names, categories and normalisation follow the
Unicode version of the Python that runs the tool.

The CONFUSABLE pairs below are taken from confusables.txt (Unicode Security Mechanisms for
UTS #39, version 18.0.0, dated 2026-08-06, https://www.unicode.org/Public/security/latest/
confusables.txt), a Unicode Data File used under the Unicode License v3:
  Copyright (c) 1991-2026 Unicode, Inc. Permission is hereby granted, free of charge, to any
  person obtaining a copy of data files and any associated documentation (the "Data Files") or
  software and any associated documentation (the "Software") to deal in the Data Files or
  Software without restriction, including without limitation the rights to use, copy, modify,
  merge, publish, distribute, and/or sell copies of the Data Files or Software, and to permit
  persons to whom the Data Files or Software are furnished to do so, provided that either (a)
  this copyright and permission notice appear with all copies of the Data Files or Software, or
  (b) this copyright and permission notice appear in associated Documentation.
  THE DATA FILES AND SOFTWARE ARE PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A
  PARTICULAR PURPOSE AND NONINFRINGEMENT OF THIRD PARTY RIGHTS. IN NO EVENT SHALL THE COPYRIGHT
  HOLDER OR HOLDERS INCLUDED IN THIS NOTICE BE LIABLE FOR ANY CLAIM, OR ANY SPECIAL INDIRECT OR
  CONSEQUENTIAL DAMAGES, OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS,
  WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN
  CONNECTION WITH THE USE OR PERFORMANCE OF THE DATA FILES OR SOFTWARE. Except as contained in
  this notice, the name of a copyright holder shall not be used in advertising or otherwise to
  promote the sale, use or other dealings in these Data Files or Software without prior written
  authorization of the copyright holder.
"""
import json
import re
import sys
import unicodedata

MAX_CHARS = 20000
MAX_LISTED = 2000
CONFUSABLES_SOURCE = "Unicode TR39 confusables.txt, version 18.0.0 (2026-08-06), https://www.unicode.org/Public/security/latest/confusables.txt"
CATEGORIES = {
    "Lu": "Letter, uppercase", "Ll": "Letter, lowercase", "Lt": "Letter, titlecase", "Lm": "Letter, modifier", "Lo": "Letter, other",
    "Mn": "Mark, nonspacing", "Mc": "Mark, spacing combining", "Me": "Mark, enclosing",
    "Nd": "Number, decimal digit", "Nl": "Number, letter", "No": "Number, other",
    "Pc": "Punctuation, connector", "Pd": "Punctuation, dash", "Ps": "Punctuation, open", "Pe": "Punctuation, close",
    "Pi": "Punctuation, initial quote", "Pf": "Punctuation, final quote", "Po": "Punctuation, other",
    "Sm": "Symbol, math", "Sc": "Symbol, currency", "Sk": "Symbol, modifier", "So": "Symbol, other",
    "Zs": "Separator, space", "Zl": "Separator, line", "Zp": "Separator, paragraph",
    "Cc": "Other, control", "Cf": "Other, format", "Cs": "Other, surrogate", "Co": "Other, private use", "Cn": "Other, not assigned",
}
# Script ranges (unicodedata has no Script property): the blocks of the Unicode Scripts.txt
# Latin, Greek and Cyrillic ranges that matter for homoglyphs; anything else that is a letter
# is "Other", and marks inherit the script of the letter before them.
SCRIPT_RANGES = [
    ("Latin", 0x0041, 0x005A), ("Latin", 0x0061, 0x007A), ("Latin", 0x00AA, 0x00AA), ("Latin", 0x00BA, 0x00BA),
    ("Latin", 0x00C0, 0x00D6), ("Latin", 0x00D8, 0x00F6), ("Latin", 0x00F8, 0x024F), ("Latin", 0x0250, 0x02AF),
    ("Latin", 0x1D00, 0x1D25), ("Latin", 0x1E00, 0x1EFF), ("Latin", 0x2C60, 0x2C7F), ("Latin", 0xA722, 0xA7FF),
    ("Latin", 0xAB30, 0xAB5A), ("Latin", 0xFB00, 0xFB06), ("Latin", 0xFF21, 0xFF3A), ("Latin", 0xFF41, 0xFF5A),
    ("Greek", 0x0370, 0x0373), ("Greek", 0x0375, 0x0377), ("Greek", 0x037A, 0x037D), ("Greek", 0x037F, 0x037F),
    ("Greek", 0x0384, 0x0384), ("Greek", 0x0386, 0x0386), ("Greek", 0x0388, 0x03E1), ("Greek", 0x03F0, 0x03FF),
    ("Greek", 0x1D26, 0x1D2A), ("Greek", 0x1F00, 0x1FFE), ("Greek", 0x2126, 0x2126),
    ("Cyrillic", 0x0400, 0x052F), ("Cyrillic", 0x1C80, 0x1C8F), ("Cyrillic", 0x2DE0, 0x2DFF), ("Cyrillic", 0xA640, 0xA69F),
]
# Cyrillic and Greek letters that confusables.txt maps to a single Latin letter (prototype).
CONFUSABLE = {
    0x03B1: 0x0061,  # GREEK SMALL LETTER ALPHA -> LATIN SMALL LETTER A
    0x0430: 0x0061,  # CYRILLIC SMALL LETTER A -> LATIN SMALL LETTER A
    0x0391: 0x0041,  # GREEK CAPITAL LETTER ALPHA -> LATIN CAPITAL LETTER A
    0x0410: 0x0041,  # CYRILLIC CAPITAL LETTER A -> LATIN CAPITAL LETTER A
    0x042C: 0x0062,  # CYRILLIC CAPITAL LETTER SOFT SIGN -> LATIN SMALL LETTER B
    0x0392: 0x0042,  # GREEK CAPITAL LETTER BETA -> LATIN CAPITAL LETTER B
    0x0412: 0x0042,  # CYRILLIC CAPITAL LETTER VE -> LATIN CAPITAL LETTER B
    0x03F2: 0x0063,  # GREEK LUNATE SIGMA SYMBOL -> LATIN SMALL LETTER C
    0x0441: 0x0063,  # CYRILLIC SMALL LETTER ES -> LATIN SMALL LETTER C
    0x03F9: 0x0043,  # GREEK CAPITAL LUNATE SIGMA SYMBOL -> LATIN CAPITAL LETTER C
    0x0421: 0x0043,  # CYRILLIC CAPITAL LETTER ES -> LATIN CAPITAL LETTER C
    0x0435: 0x0065,  # CYRILLIC SMALL LETTER IE -> LATIN SMALL LETTER E
    0x04BD: 0x0065,  # CYRILLIC SMALL LETTER ABKHASIAN CHE -> LATIN SMALL LETTER E
    0x0395: 0x0045,  # GREEK CAPITAL LETTER EPSILON -> LATIN CAPITAL LETTER E
    0x0415: 0x0045,  # CYRILLIC CAPITAL LETTER IE -> LATIN CAPITAL LETTER E
    0x03DC: 0x0046,  # GREEK LETTER DIGAMMA -> LATIN CAPITAL LETTER F
    0x04BB: 0x0068,  # CYRILLIC SMALL LETTER SHHA -> LATIN SMALL LETTER H
    0x04BA: 0x0068,  # CYRILLIC CAPITAL LETTER SHHA -> LATIN SMALL LETTER H
    0x0397: 0x0048,  # GREEK CAPITAL LETTER ETA -> LATIN CAPITAL LETTER H
    0x041D: 0x0048,  # CYRILLIC CAPITAL LETTER EN -> LATIN CAPITAL LETTER H
    0x03B9: 0x0069,  # GREEK SMALL LETTER IOTA -> LATIN SMALL LETTER I
    0x0456: 0x0069,  # CYRILLIC SMALL LETTER BYELORUSSIAN-UKRAINIAN I -> LATIN SMALL LETTER I
    0x03F3: 0x006A,  # GREEK LETTER YOT -> LATIN SMALL LETTER J
    0x0458: 0x006A,  # CYRILLIC SMALL LETTER JE -> LATIN SMALL LETTER J
    0x037F: 0x004A,  # GREEK CAPITAL LETTER YOT -> LATIN CAPITAL LETTER J
    0x0408: 0x004A,  # CYRILLIC CAPITAL LETTER JE -> LATIN CAPITAL LETTER J
    0x039A: 0x004B,  # GREEK CAPITAL LETTER KAPPA -> LATIN CAPITAL LETTER K
    0x041A: 0x004B,  # CYRILLIC CAPITAL LETTER KA -> LATIN CAPITAL LETTER K
    0x0399: 0x006C,  # GREEK CAPITAL LETTER IOTA -> LATIN SMALL LETTER L
    0x0406: 0x006C,  # CYRILLIC CAPITAL LETTER BYELORUSSIAN-UKRAINIAN I -> LATIN SMALL LETTER L
    0x04CF: 0x006C,  # CYRILLIC SMALL LETTER PALOCHKA -> LATIN SMALL LETTER L
    0x04C0: 0x006C,  # CYRILLIC LETTER PALOCHKA -> LATIN SMALL LETTER L
    0x039C: 0x004D,  # GREEK CAPITAL LETTER MU -> LATIN CAPITAL LETTER M
    0x03FA: 0x004D,  # GREEK CAPITAL LETTER SAN -> LATIN CAPITAL LETTER M
    0x041C: 0x004D,  # CYRILLIC CAPITAL LETTER EM -> LATIN CAPITAL LETTER M
    0x039D: 0x004E,  # GREEK CAPITAL LETTER NU -> LATIN CAPITAL LETTER N
    0x03BF: 0x006F,  # GREEK SMALL LETTER OMICRON -> LATIN SMALL LETTER O
    0x03C3: 0x006F,  # GREEK SMALL LETTER SIGMA -> LATIN SMALL LETTER O
    0x043E: 0x006F,  # CYRILLIC SMALL LETTER O -> LATIN SMALL LETTER O
    0x039F: 0x004F,  # GREEK CAPITAL LETTER OMICRON -> LATIN CAPITAL LETTER O
    0x041E: 0x004F,  # CYRILLIC CAPITAL LETTER O -> LATIN CAPITAL LETTER O
    0x03C1: 0x0070,  # GREEK SMALL LETTER RHO -> LATIN SMALL LETTER P
    0x03F1: 0x0070,  # GREEK RHO SYMBOL -> LATIN SMALL LETTER P
    0x03F8: 0x0070,  # GREEK SMALL LETTER SHO -> LATIN SMALL LETTER P
    0x0440: 0x0070,  # CYRILLIC SMALL LETTER ER -> LATIN SMALL LETTER P
    0x03A1: 0x0050,  # GREEK CAPITAL LETTER RHO -> LATIN CAPITAL LETTER P
    0x0420: 0x0050,  # CYRILLIC CAPITAL LETTER ER -> LATIN CAPITAL LETTER P
    0x0433: 0x0072,  # CYRILLIC SMALL LETTER GHE -> LATIN SMALL LETTER R
    0x0455: 0x0073,  # CYRILLIC SMALL LETTER DZE -> LATIN SMALL LETTER S
    0x0405: 0x0053,  # CYRILLIC CAPITAL LETTER DZE -> LATIN CAPITAL LETTER S
    0x03A4: 0x0054,  # GREEK CAPITAL LETTER TAU -> LATIN CAPITAL LETTER T
    0x0422: 0x0054,  # CYRILLIC CAPITAL LETTER TE -> LATIN CAPITAL LETTER T
    0x03C5: 0x0075,  # GREEK SMALL LETTER UPSILON -> LATIN SMALL LETTER U
    0x03BD: 0x0076,  # GREEK SMALL LETTER NU -> LATIN SMALL LETTER V
    0x0475: 0x0076,  # CYRILLIC SMALL LETTER IZHITSA -> LATIN SMALL LETTER V
    0x0474: 0x0056,  # CYRILLIC CAPITAL LETTER IZHITSA -> LATIN CAPITAL LETTER V
    0x0461: 0x0077,  # CYRILLIC SMALL LETTER OMEGA -> LATIN SMALL LETTER W
    0x0448: 0x0077,  # CYRILLIC SMALL LETTER SHA -> LATIN SMALL LETTER W
    0x0445: 0x0078,  # CYRILLIC SMALL LETTER HA -> LATIN SMALL LETTER X
    0x03A7: 0x0058,  # GREEK CAPITAL LETTER CHI -> LATIN CAPITAL LETTER X
    0x0425: 0x0058,  # CYRILLIC CAPITAL LETTER HA -> LATIN CAPITAL LETTER X
    0x03B3: 0x0079,  # GREEK SMALL LETTER GAMMA -> LATIN SMALL LETTER Y
    0x0443: 0x0079,  # CYRILLIC SMALL LETTER U -> LATIN SMALL LETTER Y
    0x04AF: 0x0079,  # CYRILLIC SMALL LETTER STRAIGHT U -> LATIN SMALL LETTER Y
    0x03A5: 0x0059,  # GREEK CAPITAL LETTER UPSILON -> LATIN CAPITAL LETTER Y
    0x03D2: 0x0059,  # GREEK UPSILON WITH HOOK SYMBOL -> LATIN CAPITAL LETTER Y
    0x0423: 0x0059,  # CYRILLIC CAPITAL LETTER U -> LATIN CAPITAL LETTER Y
    0x04AE: 0x0059,  # CYRILLIC CAPITAL LETTER STRAIGHT U -> LATIN CAPITAL LETTER Y
    0x0396: 0x005A,  # GREEK CAPITAL LETTER ZETA -> LATIN CAPITAL LETTER Z
}
FORMS = ("NFC", "NFD", "NFKC", "NFKD")
ENCODINGS = {"utf-8": "utf-8", "utf-16-be": "utf-16-be", "utf-16-le": "utf-16-le", "utf-32-be": "utf-32-be", "utf-32-le": "utf-32-le"}


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def cp(char):
    return f"U+{ord(char):04X}"


def hexbytes(data):
    return " ".join(f"{b:02X}" for b in data)


def script_of(char):
    code = ord(char)
    for name, low, high in SCRIPT_RANGES:
        if low <= code <= high:
            return name
    category = unicodedata.category(char)
    if category.startswith("M"):
        return "Inherited"
    if category.startswith("L"):
        return "Other"
    return "Common"


def describe(char):
    category = unicodedata.category(char)
    item = {"char": char, "codepoint": cp(char), "name": unicodedata.name(char, None), "category": category,
            "category_name": CATEGORIES.get(category, category), "script": script_of(char)}
    if unicodedata.combining(char):
        item["combining_class"] = unicodedata.combining(char)
    if unicodedata.decomposition(char):
        item["decomposition"] = unicodedata.decomposition(char)
    item["utf_8_hex"] = hexbytes(char.encode("utf-8"))
    return item


def normal_form(text, form):
    result = unicodedata.normalize(form, text)
    return {"text": result, "codepoints": [cp(c) for c in result][:MAX_LISTED], "length": len(result),
            "input_is_normalized": unicodedata.is_normalized(form, text)}


def strip_accents(text):
    decomposed = unicodedata.normalize("NFD", text)
    kept = "".join(c for c in decomposed if not unicodedata.category(c).startswith("M"))
    result = unicodedata.normalize("NFC", kept)
    removed = sum(1 for c in decomposed if unicodedata.category(c).startswith("M"))
    untouched = sorted({c for c in result if ord(c) > 127 and unicodedata.category(c).startswith("L")})
    return {"text": result, "marks_removed": removed, "non_ascii_letters_left": untouched[:MAX_LISTED]}


def words_of(text):
    """Runs of letters and the marks attached to them, with their start offsets."""
    start = None
    for index, char in enumerate(text + " "):
        category = unicodedata.category(char)
        if category.startswith("L") or (category.startswith("M") and start is not None):
            if start is None:
                start = index
        elif start is not None:
            yield start, text[start:index]
            start = None


def confusables(text):
    words = []
    for start, word in words_of(text):
        scripts, last = [], None
        for char in word:
            script = script_of(char)
            if script == "Inherited":
                script = last
            if script and script not in ("Common", "Inherited") and script not in scripts:
                scripts.append(script)
            last = script
        lookalikes = [{"char": c, "codepoint": cp(c), "name": unicodedata.name(c, None), "looks_like": chr(CONFUSABLE[ord(c)]),
                       "looks_like_codepoint": f"U+{CONFUSABLE[ord(c)]:04X}"} for c in word if ord(c) in CONFUSABLE]
        letters = [c for c in word if not unicodedata.category(c).startswith("M")]
        whole = bool(letters) and all(ord(c) in CONFUSABLE for c in letters)
        mixed = len(scripts) > 1
        if mixed or whole:
            words.append({
                "word": word, "start": start, "scripts": scripts, "mixed_script": mixed,
                "lookalikes": lookalikes[:MAX_LISTED],
                "latin_skeleton": "".join(chr(CONFUSABLE.get(ord(c), ord(c))) for c in word),
                "all_letters_look_latin": whole,
            })
    return {"suspicious_words": words[:MAX_LISTED], "suspicious_count": len(words),
            "mixed_script_words": sum(1 for w in words if w["mixed_script"]),
            "whole_script_lookalike_words": sum(1 for w in words if w["all_letters_look_latin"] and not w["mixed_script"]),
            "verdict": "suspicious" if words else "no mixed-script words and no words made only of Latin lookalikes",
            "table": f"{len(CONFUSABLE)} Cyrillic and Greek to Latin pairs from {CONFUSABLES_SOURCE}"}


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = sorted(set(request) - {"operation", "text", "form", "hex", "encoding"})
    if extra:
        fail(f"unknown field: {extra[0]}")
    operation = request.get("operation")
    operations = ("normalize", "strip_accents", "codepoints", "bytes", "decode", "confusables")
    if operation not in operations:
        fail("operation must be one of " + ", ".join(operations))
    result = {"operation": operation}
    if operation == "decode":
        raw = request.get("hex")
        if not isinstance(raw, str):
            fail("hex must be a string of hexadecimal bytes, e.g. \"C3 A9\"")
        try:
            data = bytes.fromhex(re.sub(r"[\s:,-]|0x", "", raw))
        except ValueError:
            fail("hex must be hexadecimal bytes, e.g. \"C3 A9\" or \"c3a9\"")
        encoding = request.get("encoding", "utf-8")
        if encoding not in ENCODINGS:
            fail("encoding must be one of " + ", ".join(ENCODINGS))
        try:
            text = data.decode(ENCODINGS[encoding])
        except UnicodeDecodeError as error:
            fail(f"not valid {encoding}: byte {error.start} ({error.reason})")
        if len(text) > MAX_CHARS:
            fail(f"the decoded text is longer than {MAX_CHARS} characters")
        result.update({"encoding": encoding, "text": text, "codepoints": [describe(c) for c in text][:MAX_LISTED]})
    else:
        text = request.get("text")
        if not isinstance(text, str):
            fail("text must be a string")
        if len(text) > MAX_CHARS:
            fail(f"text is longer than {MAX_CHARS} characters")
        if any(0xD800 <= ord(c) <= 0xDFFF for c in text):
            fail("text contains a lone surrogate (an unpaired \\ud800-\\udfff escape), which is not a character")
        if "form" in request and operation != "normalize":
            fail("form applies only to normalize")
        if operation == "normalize":
            form = request.get("form")
            if form is not None and form not in FORMS:
                fail("form must be NFC, NFD, NFKC or NFKD")
            result["input_codepoints"] = [cp(c) for c in text][:MAX_LISTED]
            result["forms"] = {f: normal_form(text, f) for f in ([form] if form else FORMS)}
        elif operation == "strip_accents":
            result.update(strip_accents(text))
        elif operation == "codepoints":
            result["length"] = len(text)
            result["codepoints"] = [describe(c) for c in text][:MAX_LISTED]
            result["truncated"] = len(text) > MAX_LISTED
        elif operation == "bytes":
            result["codepoint_count"] = len(text)
            for name in ENCODINGS:
                data = text.encode(ENCODINGS[name])
                result[name.replace("-", "_") + "_hex"] = hexbytes(data)
                result[name.replace("-", "_") + "_length"] = len(data)
            result["utf_16_code_units"] = result["utf_16_be_length"] // 2
        else:
            result.update(confusables(text))
    result["unidata_version"] = unicodedata.unidata_version
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
