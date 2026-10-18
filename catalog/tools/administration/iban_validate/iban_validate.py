#!/usr/bin/env python3
"""IBAN validation (ISO 13616): characters, country length, the mod-97 check, and the BBAN
split for countries whose layout is bundled; Spanish account numbers also get their two
national check digits verified. Can also compute the check digits for a country and BBAN.

Input (JSON on stdin):
  iban     "ES91 2100 0418 4502 0005 1332"   or
  ibans    ["...", "..."]                      or
  country + bban   to build an IBAN with its check digits
"""
import json
import re
import sys

REGISTRY = "SWIFT IBAN Registry, country lengths as published in 2025"
LENGTHS = {
    "AD": 24, "AE": 23, "AL": 28, "AT": 20, "AZ": 28, "BA": 20, "BE": 16, "BG": 22, "BH": 22, "BI": 27, "BR": 29, "BY": 28,
    "CH": 21, "CR": 22, "CY": 28, "CZ": 24, "DE": 22, "DJ": 27, "DK": 18, "DO": 28, "EE": 20, "EG": 29, "ES": 24, "FI": 18,
    "FK": 18, "FO": 18, "FR": 27, "GB": 22, "GE": 22, "GI": 23, "GL": 18, "GR": 27, "GT": 28, "HN": 28, "HR": 21, "HU": 28,
    "IE": 22, "IL": 23, "IQ": 23, "IS": 26, "IT": 27, "JO": 30, "KW": 30, "KZ": 20, "LB": 28, "LC": 32, "LI": 21, "LT": 20,
    "LU": 20, "LV": 21, "LY": 25, "MC": 27, "MD": 24, "ME": 22, "MK": 19, "MN": 20, "MR": 27, "MT": 31, "MU": 30, "NI": 28,
    "NL": 18, "NO": 15, "OM": 23, "PK": 24, "PL": 28, "PS": 29, "PT": 25, "QA": 29, "RO": 24, "RS": 22, "RU": 33, "SA": 24,
    "SC": 31, "SD": 18, "SE": 24, "SI": 19, "SK": 24, "SM": 27, "SO": 23, "ST": 25, "SV": 28, "TL": 23, "TN": 24, "TR": 26,
    "UA": 29, "VA": 22, "VG": 24, "XK": 20, "YE": 30,
}
# BBAN fields (name, length) for common countries, from the same registry.
LAYOUTS = {
    "ES": [("bank", 4), ("branch", 4), ("national_check", 2), ("account", 10)],
    "DE": [("bank", 8), ("account", 10)],
    "FR": [("bank", 5), ("branch", 5), ("account", 11), ("national_check", 2)],
    "IT": [("national_check", 1), ("bank", 5), ("branch", 5), ("account", 12)],
    "PT": [("bank", 4), ("branch", 4), ("account", 11), ("national_check", 2)],
    "GB": [("bank", 4), ("branch", 6), ("account", 8)],
    "IE": [("bank", 4), ("branch", 6), ("account", 8)],
    "NL": [("bank", 4), ("account", 10)],
    "BE": [("bank", 3), ("account", 7), ("national_check", 2)],
    "AT": [("bank", 5), ("account", 11)],
    "CH": [("bank", 5), ("account", 12)],
    "PL": [("bank", 8), ("account", 16)],
    "SE": [("bank", 3), ("account", 17)],
}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def mod97(text):
    digits = "".join(str(int(c, 36)) for c in text)
    return int(digits) % 97


def spanish_check(bank, branch, account):
    weights = [1, 2, 4, 8, 5, 10, 9, 7, 3, 6]
    fix = lambda r: 1 if r == 10 else 0 if r == 11 else r
    first = 11 - sum(int(c) * w for c, w in zip("00" + bank + branch, weights)) % 11
    second = 11 - sum(int(c) * w for c, w in zip(account, weights)) % 11
    return f"{fix(first)}{fix(second)}"


def check(raw):
    if not isinstance(raw, str):
        return {"input": raw, "valid": False, "reason": "not a string"}
    iban = re.sub(r"[\s-]", "", raw).upper()
    if iban.startswith("IBAN"):
        iban = iban[4:]
    result = {"input": raw, "iban": iban, "valid": False}
    if not re.fullmatch(r"[A-Z]{2}[0-9]{2}[A-Z0-9]{1,30}", iban):
        result["reason"] = "an IBAN is two letters, two digits and up to 30 letters or digits"
        return result
    country = iban[:2]
    result["country"] = country
    expected = LENGTHS.get(country)
    if expected is None:
        result["reason"] = f"{country} is not an IBAN country in the bundled registry"
        return result
    if len(iban) != expected:
        result["reason"] = f"{country} IBANs have {expected} characters, this one has {len(iban)}"
        return result
    if mod97(iban[4:] + iban[:4]) != 1:
        result["reason"] = "check digits do not match (mod 97)"
        return result
    bban = iban[4:]
    result.update({"valid": True, "check_digits": iban[2:4], "bban": bban,
                   "formatted": " ".join(iban[i:i + 4] for i in range(0, len(iban), 4))})
    layout = LAYOUTS.get(country)
    if layout:
        fields, at = {}, 0
        for name, size in layout:
            fields[name] = bban[at:at + size]
            at += size
        result["fields"] = fields
        if country == "ES" and bban.isdigit():
            want = spanish_check(fields["bank"], fields["branch"], fields["account"])
            result["national_check_valid"] = want == fields["national_check"]
            if not result["national_check_valid"]:
                result["valid"] = False
                result["reason"] = f"Spanish account check digits should be {want}"
    return result


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if "country" in request or "bban" in request:
        country = str(request.get("country", "")).upper()
        bban = re.sub(r"[\s-]", "", str(request.get("bban", ""))).upper()
        if country not in LENGTHS or not re.fullmatch(r"[A-Z0-9]+", bban):
            fail("country must be an IBAN country code and bban letters and digits")
        digits = 98 - mod97(bban + country + "00")
        built = f"{country}{digits:02d}{bban}"
        print(json.dumps({"registry": REGISTRY, **check(built)}))
        return
    if "iban" in request:
        print(json.dumps({"registry": REGISTRY, **check(request["iban"])}))
        return
    ibans = request.get("ibans")
    if not isinstance(ibans, list) or not ibans:
        fail("give iban, ibans, or country and bban")
    results = [check(i) for i in ibans]
    print(json.dumps({"registry": REGISTRY, "results": results, "valid": sum(r["valid"] for r in results), "invalid": sum(not r["valid"] for r in results)}))


if __name__ == "__main__":
    main()
