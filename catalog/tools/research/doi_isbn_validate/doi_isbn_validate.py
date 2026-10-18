#!/usr/bin/env python3
"""Validate and normalise DOIs, ISBN-10, ISBN-13, ISSN and ISMN identifiers; convert ISBN-10 <->
ISBN-13, ISSN -> EAN-13 and legacy ISMN M-... <-> 979-0-...

Input (JSON on stdin):
  value        one identifier, or
  identifiers  a list of up to 1000
  type         "auto" (default) | "doi" | "isbn" | "issn" | "ismn"
  issue        two-digit issue (sequence variant) for the ISSN's EAN-13, default "00"

Hyphens and spaces in ISBN/ISSN/ISMN are accepted anywhere; labels such as "ISBN-13:" are
stripped. DOIs may be given bare, with "doi:" or as a doi.org / dx.doi.org URL.
"""
import json
import re
import sys

MAX_ITEMS = 1000
TYPES = ("auto", "doi", "isbn", "issn", "ismn")
DOI_URL = re.compile(r"^(?:https?://)?(?:dx\.)?doi\.org/", re.I)
DOI_SYNTAX = re.compile(r"^10\.(\d+(?:\.\d+)*)/(.+)$", re.S)
CROSSREF = re.compile(r"^10\.\d{4,9}/[-._;()/:A-Z0-9]+$", re.I)
LABEL = re.compile(r"^\s*(?:ISBN|ISSN|ISMN|EAN)(?:-1[03])?\s*:?\s*", re.I)
# characters percent-encoded when a DOI is written as a URL (DOI Handbook, encoding rules for URLs)
URL_ENCODE = set('%"#? <>{}^[]`|\\')


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def percent_decode(text):
    """Decode %XX sequences as UTF-8 (a hand-rolled urllib.parse.unquote)."""
    raw, i = bytearray(), 0
    while i < len(text):
        if text[i] == "%" and re.fullmatch(r"[0-9A-Fa-f]{2}", text[i + 1:i + 3] or ""):
            raw.append(int(text[i + 1:i + 3], 16))
            i += 3
        else:
            raw.extend(text[i].encode("utf-8"))
            i += 1
    return raw.decode("utf-8", errors="replace")


def percent_encode(text):
    out = []
    for ch in text:
        if ch in URL_ENCODE or ord(ch) < 33 or ord(ch) > 126:
            out.extend(f"%{b:02X}" for b in ch.encode("utf-8"))
        else:
            out.append(ch)
    return "".join(out)


def ean_check(digits12):
    total = sum(int(d) * (1 if i % 2 == 0 else 3) for i, d in enumerate(digits12))
    return str((10 - total % 10) % 10)


def isbn10_check(digits9):
    total = sum(int(d) * (10 - i) for i, d in enumerate(digits9))
    c = (11 - total % 11) % 11
    return "X" if c == 10 else str(c)


def issn_check(digits7):
    total = sum(int(d) * (8 - i) for i, d in enumerate(digits7))
    c = (11 - total % 11) % 11
    return "X" if c == 10 else str(c)


def compact(value):
    return re.sub(r"[\s\-‐‑‒–]", "", LABEL.sub("", value)).upper()


def check_doi(value):
    text = value.strip()
    out = {"type": "doi", "input": value}
    form = "bare"
    if DOI_URL.match(text):
        text, form = percent_decode(DOI_URL.sub("", text)), "url"
    elif text.lower().startswith("doi:"):
        text, form = text[4:].strip(), "doi:"
    out["form"] = form
    m = DOI_SYNTAX.match(text)
    errors = []
    if not text.startswith("10."):
        errors.append("a DOI starts with the directory indicator 10.")
    elif not m:
        errors.append("a DOI is 10.<registrant>/<suffix>: the registrant code is digits (optionally dot-separated) and a non-empty suffix follows the first '/'")
    elif re.search(r"\s", text):
        errors.append("a DOI contains no whitespace")
    if errors:
        out.update(valid=False, errors=errors)
        return out
    out.update(valid=True, doi=text, doi_lower=text.lower(), prefix=f"10.{m.group(1)}", registrant=m.group(1),
               suffix=m.group(2), url="https://doi.org/" + percent_encode(text),
               crossref_pattern=bool(CROSSREF.match(text)))
    if not out["crossref_pattern"]:
        out["warnings"] = ["valid syntax, but outside Crossref's recommended pattern 10.\\d{4,9}/[-._;()/:A-Z0-9]+ (check for a stray character)"]
    return out


def check_isbn(value, digits):
    out = {"type": "isbn", "input": value, "compact": digits}
    if re.fullmatch(r"\d{9}[\dX]", digits):
        expected = isbn10_check(digits[:9])
        out.update(format="ISBN-10", check_digit=digits[9], expected_check_digit=expected, valid=digits[9] == expected)
        if out["valid"]:
            body = "978" + digits[:9]
            out.update(isbn10=digits, isbn13=body + ean_check(body))
    elif re.fullmatch(r"\d{13}", digits):
        expected = ean_check(digits[:12])
        out.update(format="ISBN-13", check_digit=digits[12], expected_check_digit=expected, valid=digits[12] == expected)
        if not digits.startswith(("978", "979")):
            out["valid"] = False
            out["errors"] = ["an ISBN-13 starts with the GS1 prefix 978 or 979"]
            return out
        if out["valid"]:
            out["isbn13"] = digits
            if digits.startswith("978"):
                out["isbn10"] = digits[3:12] + isbn10_check(digits[3:12])
            else:
                out["isbn10"] = None
                out["note"] = "979 ISBNs have no ISBN-10 form"
    else:
        out.update(valid=False, errors=["an ISBN has 10 characters (digits, last may be X) or 13 digits"])
        return out
    if not out["valid"] and "errors" not in out:
        out["errors"] = [f"check digit is {out['check_digit']}, expected {out['expected_check_digit']}"]
    return out


def check_issn(value, digits, issue):
    out = {"type": "issn", "input": value, "compact": digits}
    if not re.fullmatch(r"\d{7}[\dX]", digits):
        out.update(valid=False, errors=["an ISSN has 8 characters: 7 digits and a check digit (0-9 or X)"])
        return out
    expected = issn_check(digits[:7])
    out.update(check_digit=digits[7], expected_check_digit=expected, valid=digits[7] == expected)
    if out["valid"]:
        body = "977" + digits[:7] + issue
        out.update(issn=f"{digits[:4]}-{digits[4:]}", ean13=body + ean_check(body))
    else:
        out["errors"] = [f"check digit is {digits[7]}, expected {expected}"]
    return out


def check_ismn(value, digits):
    out = {"type": "ismn", "input": value, "compact": digits}
    if re.fullmatch(r"M\d{9}", digits):
        out["format"] = "ISMN-10 (legacy M-)"
        body = "9790" + digits[1:9]
        given = digits[9]
    elif re.fullmatch(r"9790\d{9}", digits):
        out["format"] = "ISMN-13"
        body, given = digits[:12], digits[12]
    else:
        out.update(valid=False, errors=["an ISMN is 979-0 followed by 9 digits, or the legacy M followed by 9 digits"])
        return out
    expected = ean_check(body)
    out.update(check_digit=given, expected_check_digit=expected, valid=given == expected)
    if out["valid"]:
        out.update(ismn13=body + expected, ismn10="M" + body[4:] + expected)
    else:
        out["errors"] = [f"check digit is {given}, expected {expected}"]
    return out


def detect(value):
    text = value.strip()
    if DOI_URL.match(text) or text.lower().startswith("doi:") or text.startswith("10.") or "/" in text:
        return "doi"
    label = LABEL.match(text)
    if label:
        word = label.group().strip().upper()[:4]
        if word in ("ISBN", "ISSN", "ISMN"):
            return word.lower()
    digits = compact(text)
    if digits.startswith("M") or digits.startswith("9790"):
        return "ismn"
    if len(digits) == 8:
        return "issn"
    return "isbn"


def validate(value, kind, issue):
    if kind == "auto":
        kind = detect(value)
    if kind == "doi":
        return check_doi(value)
    digits = compact(value)
    if kind == "isbn":
        return check_isbn(value, digits)
    if kind == "issn":
        return check_issn(value, digits, issue)
    return check_ismn(value, digits)


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"value", "identifiers", "type", "issue"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    kind = request.get("type", "auto")
    if kind not in TYPES:
        fail("type must be auto, doi, isbn, issn or ismn")
    issue = request.get("issue", "00")
    if not isinstance(issue, str) or not re.fullmatch(r"\d{2}", issue):
        fail("issue must be two digits, such as \"00\"")
    if ("value" in request) == ("identifiers" in request):
        fail("give value or identifiers")
    if "value" in request:
        value = request["value"]
        if not isinstance(value, str) or not value.strip() or len(value) > 2000:
            fail("value must be a non-empty string of at most 2000 characters")
        print(json.dumps(validate(value, kind, issue), ensure_ascii=False))
        return
    items = request["identifiers"]
    if not isinstance(items, list) or not 1 <= len(items) <= MAX_ITEMS:
        fail(f"identifiers must be a list of 1 to {MAX_ITEMS} strings")
    results = []
    for i, value in enumerate(items):
        if not isinstance(value, str) or not value.strip() or len(value) > 2000:
            fail(f"identifiers[{i}] must be a non-empty string of at most 2000 characters")
        results.append(validate(value, kind, issue))
    print(json.dumps({"count": len(results), "valid": sum(r["valid"] for r in results), "results": results},
                     ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
