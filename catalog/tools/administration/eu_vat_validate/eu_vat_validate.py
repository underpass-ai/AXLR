#!/usr/bin/env python3
"""EU VAT identification numbers: the syntax and check digits of each member state's format,
plus XI (Northern Ireland). Offline: it says whether a number is well formed, not whether it
is registered (that is what the Commission's VIES service answers).

Input (JSON on stdin):
  vat    "DE136695976"   or   "DE 136 695 976"
  vats   ["...", "..."]
  country  optional when the number has no prefix
"""
import json
import re
import sys

NAMES = {"AT": "Austria", "BE": "Belgium", "BG": "Bulgaria", "CY": "Cyprus", "CZ": "Czechia", "DE": "Germany", "DK": "Denmark",
         "EE": "Estonia", "EL": "Greece", "ES": "Spain", "FI": "Finland", "FR": "France", "HR": "Croatia", "HU": "Hungary",
         "IE": "Ireland", "IT": "Italy", "LT": "Lithuania", "LU": "Luxembourg", "LV": "Latvia", "MT": "Malta", "NL": "Netherlands",
         "PL": "Poland", "PT": "Portugal", "RO": "Romania", "SE": "Sweden", "SI": "Slovenia", "SK": "Slovakia",
         "XI": "United Kingdom (Northern Ireland)"}
DNI_LETTERS = "TRWAGMYFPDXBNJZSQVHLCKE"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def digits(n):
    return [int(c) for c in n]


def weighted(n, weights):
    return sum(int(c) * w for c, w in zip(n, weights))


def luhn_ok(n):
    total = 0
    for i, d in enumerate(reversed(digits(n))):
        total += sum(divmod(d * 2, 10)) if i % 2 else d
    return total % 10 == 0


def iso7064_11_10(n):
    p = 10
    for d in digits(n):
        s = (d + p) % 10 or 10
        p = (2 * s) % 11
    return (11 - p) % 10


def at(n):
    if not re.fullmatch(r"U\d{8}", n):
        return False
    d = digits(n[1:])
    s = d[0] + sum(divmod(2 * d[1], 10)) + d[2] + sum(divmod(2 * d[3], 10)) + d[4] + sum(divmod(2 * d[5], 10)) + d[6]
    return (10 - (s + 4) % 10) % 10 == d[7]


def be(n):
    if len(n) == 9:
        n = "0" + n
    return bool(re.fullmatch(r"[01]\d{9}", n)) and 97 - int(n[:8]) % 97 == int(n[8:])


def bg(n):
    if re.fullmatch(r"\d{9}", n):
        c = weighted(n, range(1, 9)) % 11
        if c == 10:
            c = weighted(n, range(3, 11)) % 11 % 10
        return c == int(n[8])
    if not re.fullmatch(r"\d{10}", n):
        return False
    egn = weighted(n, (2, 4, 8, 5, 10, 9, 7, 3, 6)) % 11 % 10 == int(n[9])
    foreigner = weighted(n, (21, 19, 17, 13, 11, 9, 7, 3, 1)) % 10 == int(n[9])
    c = 11 - weighted(n, (4, 3, 2, 7, 6, 5, 4, 3, 2)) % 11
    other = c != 10 and (c % 11) == int(n[9])
    return egn or foreigner or other


def cy(n):
    if not re.fullmatch(r"[0-59]\d{7}[A-Z]", n) or n.startswith("12"):
        return False
    table = (1, 0, 5, 7, 9, 13, 15, 17, 19, 21)
    d = digits(n[:8])
    s = sum(table[x] for x in d[0::2]) + sum(d[1::2])
    return "ABCDEFGHIJKLMNOPQRSTUVWXYZ"[s % 26] == n[8]


def cz(n):
    if re.fullmatch(r"\d{8}", n):
        if n[0] == "9":
            return False
        c = (11 - weighted(n, range(8, 1, -1))) % 11
        return str((c or 1) % 10) == n[7]
    if re.fullmatch(r"6\d{8}", n):
        c = weighted(n[1:8], range(8, 1, -1)) % 11
        return str(9 - ((11 - c) % 11) % 10) == n[8]
    if re.fullmatch(r"\d{9}", n):
        return int(n[:2]) < 54
    if re.fullmatch(r"\d{10}", n):
        return int(n) % 11 == 0 or (int(n[:9]) % 11 == 10 and n[9] == "0")
    return False


def de(n):
    return bool(re.fullmatch(r"[1-9]\d{8}", n)) and iso7064_11_10(n[:8]) == int(n[8])


def dk(n):
    return bool(re.fullmatch(r"[1-9]\d{7}", n)) and weighted(n, (2, 7, 6, 5, 4, 3, 2, 1)) % 11 == 0


def ee(n):
    return bool(re.fullmatch(r"10\d{7}", n)) and (10 - weighted(n, (3, 7, 1, 3, 7, 1, 3, 7)) % 10) % 10 == int(n[8])


def el(n):
    if len(n) == 8:
        n = "0" + n
    return bool(re.fullmatch(r"\d{9}", n)) and weighted(n, (256, 128, 64, 32, 16, 8, 4, 2)) % 11 % 10 == int(n[8])


def es(n):
    if not re.fullmatch(r"[0-9A-Z]\d{7}[0-9A-Z]", n):
        return False
    if re.fullmatch(r"\d{8}[A-Z]", n):
        return DNI_LETTERS[int(n[:8]) % 23] == n[8]
    if re.fullmatch(r"[XYZ]\d{7}[A-Z]", n):
        return DNI_LETTERS[int(str("XYZ".index(n[0])) + n[1:8]) % 23] == n[8]
    if re.fullmatch(r"[KLM]\d{7}[A-Z]", n):
        return DNI_LETTERS[int(n[1:8]) % 23] == n[8]
    if re.fullmatch(r"[ABCDEFGHJNPQRSUVW]\d{7}[0-9A-J]", n):
        d = digits(n[1:8])
        s = sum(sum(divmod(2 * x, 10)) for x in d[0::2]) + sum(d[1::2])
        c = (10 - s % 10) % 10
        allowed = {"JABCDEFGHI"[c]} if n[0] in "PQRSNW" else {str(c)} if n[0] in "ABEH" else {str(c), "JABCDEFGHI"[c]}
        return n[8] in allowed
    return False


def fi(n):
    if not re.fullmatch(r"\d{8}", n):
        return False
    c = 11 - weighted(n, (7, 9, 10, 5, 8, 4, 2)) % 11
    return c != 10 and c % 11 == int(n[7])


def fr(n):
    if not re.fullmatch(r"[0-9A-HJ-NP-Z]{2}\d{9}", n):
        return False
    siren = n[2:]
    if n[:2].isdigit():
        return int(n[:2]) == (12 + 3 * (int(siren) % 97)) % 97 and (luhn_ok(siren) or siren == "356000000")
    return luhn_ok(siren) or siren == "356000000"


def hr(n):
    return bool(re.fullmatch(r"\d{11}", n)) and iso7064_11_10(n[:10]) == int(n[10])


def hu(n):
    return bool(re.fullmatch(r"[1-9]\d{7}", n)) and (10 - weighted(n, (9, 7, 3, 1, 9, 7, 3)) % 10) % 10 == int(n[7])


def ie(n):
    if re.fullmatch(r"\d[A-Z+*]\d{5}[A-W]", n):
        n = "0" + n[2:7] + n[0] + n[7]
    if not re.fullmatch(r"\d{7}[A-W][A-IW]?", n):
        return False
    s = weighted(n[:7], range(8, 1, -1)) + (9 * ("WABCDEFGHI".index(n[8])) if len(n) == 9 else 0)
    return "WABCDEFGHIJKLMNOPQRSTUV"[s % 23] == n[7]


def it(n):
    if not re.fullmatch(r"\d{11}", n) or n[:7] == "0000000":
        return False
    office = int(n[7:10])
    return (1 <= office <= 100 or office in (120, 121, 888, 999)) and luhn_ok(n)


def lt(n):
    if not (re.fullmatch(r"\d{7}1\d", n) or re.fullmatch(r"\d{10}1\d", n)):
        return False
    body = n[:-1]
    c = sum((i % 9 + 1) * int(x) for i, x in enumerate(body)) % 11
    if c == 10:
        c = sum(((i + 2) % 9 + 1) * int(x) for i, x in enumerate(body)) % 11
    return c % 10 == int(n[-1])


def lu(n):
    return bool(re.fullmatch(r"\d{8}", n)) and int(n[:6]) % 89 == int(n[6:])


def lv(n):
    if not re.fullmatch(r"\d{11}", n):
        return False
    if n[0] > "3":
        return weighted(n, (9, 1, 4, 8, 3, 10, 2, 5, 7, 6, 1)) % 11 == 3
    return (1 + weighted(n, (10, 5, 8, 4, 2, 1, 6, 3, 7, 9))) % 11 % 10 == int(n[10])


def mt(n):
    return bool(re.fullmatch(r"[1-9]\d{7}", n)) and weighted(n, (3, 4, 6, 7, 8, 9, 10, 1)) % 37 == 0


def nl(n):
    if not re.fullmatch(r"\d{9}B\d{2}", n):
        return False
    old = weighted(n[:8], range(9, 1, -1)) % 11 % 10 == int(n[8])
    new = int("".join(str(int(c, 36)) for c in "NL" + n)) % 97 == 1
    return old or new


def pl(n):
    if not re.fullmatch(r"\d{10}", n):
        return False
    c = weighted(n, (6, 5, 7, 2, 3, 4, 5, 6, 7)) % 11
    return c != 10 and c == int(n[9])


def pt(n):
    return bool(re.fullmatch(r"[1-9]\d{8}", n)) and (11 - weighted(n, range(9, 1, -1)) % 11) % 11 % 10 == int(n[8])


def ro(n):
    if not re.fullmatch(r"[1-9]\d{1,9}", n):
        return False
    n = n.zfill(10)
    return weighted(n, (7, 5, 3, 2, 1, 7, 5, 3, 2)) * 10 % 11 % 10 == int(n[9])


def se(n):
    return bool(re.fullmatch(r"\d{10}01", n)) and luhn_ok(n[:10])


def si(n):
    if not re.fullmatch(r"[1-9]\d{7}", n):
        return False
    c = 11 - weighted(n, range(8, 1, -1)) % 11
    return c != 11 and c % 10 == int(n[7])


def sk(n):
    return bool(re.fullmatch(r"[1-9]\d[2-47-9]\d{7}", n)) and int(n) % 11 == 0


def xi(n):
    if re.fullmatch(r"(GD[0-4]\d{2}|HA[5-9]\d{2})", n):
        return True
    if not re.fullmatch(r"\d{9}(\d{3})?", n):
        return False
    s = weighted(n[:7], range(8, 1, -1)) + int(n[7:9])
    return s % 97 == 0 or (s + 55) % 97 == 0


CHECKS = {"AT": at, "BE": be, "BG": bg, "CY": cy, "CZ": cz, "DE": de, "DK": dk, "EE": ee, "EL": el, "ES": es, "FI": fi, "FR": fr,
          "HR": hr, "HU": hu, "IE": ie, "IT": it, "LT": lt, "LU": lu, "LV": lv, "MT": mt, "NL": nl, "PL": pl, "PT": pt, "RO": ro,
          "SE": se, "SI": si, "SK": sk, "XI": xi}


def check(raw, default_country=None):
    if not isinstance(raw, str):
        return {"input": raw, "valid": False, "reason": "not a string"}
    value = re.sub(r"[\s.\-/]", "", raw).upper()
    country = value[:2] if value[:2].isalpha() else (default_country or "").upper()
    number = value[2:] if value[:2].isalpha() else value
    if country == "GR":
        country = "EL"
    out = {"input": raw, "country": country or None, "number": number, "valid": False}
    if country not in CHECKS:
        out["reason"] = "unknown country prefix; give an EU prefix such as DE, or country" if country != "GB" else "GB numbers are not EU VAT numbers since 2021; XI is for Northern Ireland"
        return out
    out["country_name"] = NAMES[country]
    out["vat"] = country + number
    out["valid"] = CHECKS[country](number)
    if not out["valid"]:
        out["reason"] = "wrong format or check digits for " + NAMES[country]
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    country = request.get("country")
    note = "syntax and check digits only; whether the number is registered is answered by the Commission's VIES service"
    if "vat" in request:
        print(json.dumps({**check(request["vat"], country), "note": note}))
        return
    vats = request.get("vats")
    if not isinstance(vats, list) or not vats:
        fail("give vat or a non-empty vats list")
    results = [check(v, country) for v in vats]
    print(json.dumps({"results": results, "valid": sum(r["valid"] for r in results), "invalid": sum(not r["valid"] for r in results), "note": note}))


if __name__ == "__main__":
    main()
