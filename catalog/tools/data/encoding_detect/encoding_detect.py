#!/usr/bin/env python3
"""Detect the text encoding of bytes given as hex and convert them to UTF-8: byte-order marks,
strict UTF-8 validity, UTF-16 and UTF-32 without a BOM from the pattern of NUL bytes, and the
single-byte Western encodings (cp1252, latin-1, ISO-8859-15, cp850, mac_roman) ranked by how
plausible their decoded characters are. Standard library only; charset_normalizer covers many
more encodings (a tier-2 alternative).

Input (JSON on stdin):
  hex         the bytes as hex (or path: a relative file in the working directory)
  encoding    skip detection and decode with this codec name
  candidates  single-byte encodings to consider, default the five above
  max_text    longest decoded text returned, in characters, default 100000
"""
import json
import pathlib
import re
import sys
import unicodedata

BOMS = [("utf-32-le", "fffe0000"), ("utf-32-be", "0000feff"), ("utf-8-sig", "efbbbf"), ("utf-16-le", "fffe"), ("utf-16-be", "feff")]
SINGLE_BYTE = ["cp1252", "latin-1", "iso-8859-15", "cp850", "mac_roman"]
TYPOGRAPHIC = set("‘’‚“”„–—…€•™")
COMMON_SIGNS = set("¡¿ºª«»°£§©®· ´½¼²³µ×")
MAX_BYTES = 20_000_000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load(request):
    if ("hex" in request) == ("path" in request):
        fail("give exactly one of hex (the bytes as hex) or path (relative)")
    if "hex" in request:
        if not isinstance(request["hex"], str):
            fail("hex must be a string")
        try:
            data = bytes.fromhex(re.sub(r"\s", "", request["hex"]))
        except ValueError:
            fail("hex is not valid hexadecimal")
    else:
        path = request["path"]
        if not isinstance(path, str) or not path or path.startswith(("/", "\\", "~")) or re.match(r"^[A-Za-z]:", path) or ".." in re.split(r"[\\/]", path):
            fail("path must be a relative path inside the working directory, without '..'")
        target = pathlib.Path(path)
        if not target.is_file():
            fail(f"no such file: {path}")
        if target.stat().st_size > MAX_BYTES:
            fail("the file is larger than 20 MB")
        data = target.read_bytes()
    if len(data) > MAX_BYTES:
        fail("the input is larger than 20 MB")
    return data


def plausibility(text):
    """Score the non-ASCII characters of a decoding: accented letters in words are likely,
    control characters, box drawing and mathematical symbols are not."""
    score, count, previous = 0.0, 0, " "
    for char in text:
        code = ord(char)
        if code < 128:
            if code < 32 and char not in "\t\n\r\f":
                score -= 5
                count += 1
            previous = char
            continue
        count += 1
        category = unicodedata.category(char)
        if 0x80 <= code <= 0x9F:
            score -= 6
        elif char in TYPOGRAPHIC:
            score += 1.5
        elif char in COMMON_SIGNS:
            score += 0.5
        elif category == "Ll":
            score += 2 if previous.isalpha() else 1.5 if previous in " \t\n'\"(-" else 0.5
            if code > 0x24F:
                score -= 2.5  # Greek and other non-Latin lower-case letters in a single-byte Latin text
        elif category == "Lu":
            if previous.isalpha() and previous.islower():
                score -= 1
            else:
                score += 1
            if code > 0x24F:
                score -= 2.5
        elif 0x2500 <= code <= 0x259F:
            score -= 3
        elif category in ("Sm", "So", "Sk"):
            score -= 2
        elif category in ("Co", "Cn", "Cc", "Cf"):
            score -= 6
        else:
            score -= 0.5
        previous = char
    return score, count


def utf16_guess(data):
    """UTF-16/32 without BOM: ASCII-range text has a NUL in every other (or three in four) byte."""
    if len(data) < 4:
        return None
    even = data[0::2]
    odd = data[1::2]
    nul_even = even.count(0) / len(even)
    nul_odd = odd.count(0) / len(odd) if odd else 0
    if len(data) % 4 == 0:
        quads = [data[i:i + 4] for i in range(0, len(data), 4)]
        if sum(1 for q in quads if q[1:] == b"\0\0\0" or (q[1] == 0 and q[2:] == b"\0\0")) / len(quads) > 0.7 and nul_odd > 0.7:
            return "utf-32-le"
        if sum(1 for q in quads if q[:3] == b"\0\0\0" or q[:2] == b"\0\0") / len(quads) > 0.7 and nul_even > 0.7:
            return "utf-32-be"
    if len(data) % 2 == 0:
        if nul_odd > 0.4 and nul_even < 0.1:
            return "utf-16-le"
        if nul_even > 0.4 and nul_odd < 0.1:
            return "utf-16-be"
        # Non-Latin UTF-16 has few NULs (only in spaces, digits, punctuation): accept NULs on one
        # side only when the decoding is made of letters, spaces and punctuation.
        for name, ok in (("utf-16-le", even.count(0) == 0 and odd.count(0) > 0), ("utf-16-be", odd.count(0) == 0 and even.count(0) > 0)):
            if ok:
                text = decode(data, name)
                if text and all(unicodedata.category(c)[0] in "LZPNM" or c in "\t\n\r" for c in text):
                    return name
    return None


def decode(data, encoding):
    try:
        return data.decode(encoding)
    except UnicodeDecodeError:
        return None


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("stdin must be a JSON object")
    unknown = sorted(set(request) - {"hex", "path", "encoding", "candidates", "max_text"})
    if unknown:
        fail(f"unknown field: {unknown[0]}")
    max_text = request.get("max_text", 100000)
    if isinstance(max_text, bool) or not isinstance(max_text, int) or max_text < 0:
        fail("max_text must be an integer >= 0")
    candidates = request.get("candidates", SINGLE_BYTE)
    if not isinstance(candidates, list) or not candidates or not all(isinstance(c, str) for c in candidates):
        fail("candidates must be a non-empty list of codec names")
    for name in candidates:
        try:
            b"".decode(name)
        except LookupError:
            fail(f"unknown encoding {name!r}")
    data = load(request)
    result = {"bytes": len(data), "bom": None, "method": None, "candidates": []}

    def finish(encoding, text, confidence, method):
        clipped = text[:max_text]
        result.update({"encoding": encoding, "confidence": round(confidence, 3), "method": method, "text": clipped,
                       "text_truncated": len(text) > max_text, "characters": len(text),
                       "utf8_hex": clipped.encode("utf-8").hex() if len(clipped) <= 4096 else None,
                       "line_endings": {"crlf": text.count("\r\n"), "lf": text.count("\n") - text.count("\r\n"), "cr": text.count("\r") - text.count("\r\n")},
                       "note": "Detection is a heuristic for the encodings listed; charset_normalizer (a package) handles many more."})
        print(json.dumps(result, ensure_ascii=False))
        sys.exit(0)

    if "encoding" in request:
        name = request["encoding"]
        if not isinstance(name, str):
            fail("encoding must be a codec name")
        try:
            text = data.decode(name)
        except LookupError:
            fail(f"unknown encoding {name!r}")
        except UnicodeDecodeError as error:
            fail(f"the bytes are not valid {name}: byte {error.start} (0x{data[error.start]:02x}) cannot be decoded")
        finish(name, text, 1.0, "given")
    if not data:
        finish("ascii", "", 1.0, "empty")
    for name, marker in BOMS:
        if data.startswith(bytes.fromhex(marker)):
            result["bom"] = name.replace("-sig", "")
            text = decode(data[len(marker) // 2:], name.replace("-sig", ""))
            if text is not None:
                finish(name.replace("-sig", ""), text, 1.0, "byte-order mark")
            result["bom"] = None
    guess = utf16_guess(data)
    if guess:
        text = decode(data, guess)
        if text is not None:
            finish(guess, text, 0.9, "NUL byte pattern (no BOM)")
    if all(b < 128 for b in data):
        if b"\x1b$B" in data or b"\x1b(J" in data:
            text = decode(data, "iso2022_jp")
            if text is not None:
                finish("iso2022_jp", text, 0.9, "ISO-2022-JP escape sequences")
        finish("ascii", data.decode("ascii"), 1.0, "only 7-bit bytes (also valid UTF-8 and every candidate)")
    text = decode(data, "utf-8")
    if text is not None:
        multibyte = sum(1 for b in data if b >= 0xC2 and b <= 0xF4)
        confidence = 0.99 if multibyte >= 2 else 0.95
        finish("utf-8", text, confidence, "strict UTF-8 validation")
    # Single-byte candidates, ranked by plausibility per non-ASCII character.
    scored = []
    for name in candidates:
        decoded = decode(data, name)
        if decoded is None:
            result["candidates"].append({"encoding": name, "valid": False})
            continue
        score, count = plausibility(decoded)
        scored.append((score / max(count, 1), name, decoded))
    if not scored:
        fail("no candidate encoding can decode these bytes")
    # Prefer the earlier candidate (cp1252 first) when scores tie, as WHATWG treats latin-1 as windows-1252.
    scored.sort(key=lambda item: (-item[0], candidates.index(item[1])))
    best_score, best_name, best_text = scored[0]
    same_text = [name for _, name, decoded in scored[1:] if decoded == best_text]
    rivals = [s for s, name, decoded in scored[1:] if decoded != best_text]
    margin = best_score - (rivals[0] if rivals else best_score - 3)
    confidence = max(0.05, min(0.95, 0.5 + margin / 6)) if best_score > 0 else max(0.05, min(0.5, 0.3 + margin / 10))
    for score, name, decoded in scored:
        result["candidates"].append({"encoding": name, "valid": True, "score": round(score, 3),
                                     "same_text_as_best": decoded == best_text, "sample": decoded[:60]})
    result["equivalent_encodings"] = same_text
    finish(best_name, best_text, confidence, "single-byte plausibility (not valid UTF-8)")


if __name__ == "__main__":
    main()
