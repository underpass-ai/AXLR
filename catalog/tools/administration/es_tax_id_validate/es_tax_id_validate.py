#!/usr/bin/env python3
"""Spanish tax identifiers: DNI/NIF of a citizen (8 digits and a letter), NIE of a foreigner
(X, Y or Z, 7 digits and a letter), special NIFs (K, L, M), and the NIF of a legal entity
(the former CIF: an entity letter, a province code, a serial and a check digit or letter).
Each is checked and its type explained; a wrong check character comes with the right one.

Input (JSON on stdin):
  id    "12345678Z"     or
  ids   ["X1234567L", "B12345674", ...]
"""
import json
import re
import sys

LETTERS = "TRWAGMYFPDXBNJZSQVHLCKE"
ENTITY_LETTER = "JABCDEFGHI"
ENTITY_TYPES = {
    "A": "sociedad anónima", "B": "sociedad de responsabilidad limitada", "C": "sociedad colectiva",
    "D": "sociedad comanditaria", "E": "comunidad de bienes o herencia yacente", "F": "sociedad cooperativa",
    "G": "asociación o fundación", "H": "comunidad de propietarios en régimen de propiedad horizontal",
    "J": "sociedad civil", "N": "entidad extranjera", "P": "corporación local", "Q": "organismo público",
    "R": "congregación o institución religiosa", "S": "órgano de la Administración del Estado o de las comunidades autónomas",
    "U": "unión temporal de empresas", "V": "otro tipo de entidad", "W": "establecimiento permanente de entidad no residente",
}
LETTER_ONLY, DIGIT_ONLY = set("PQRSNW"), set("ABEH")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def entity_control(digits):
    odd = sum(sum(divmod(2 * int(d), 10)) for d in digits[0::2])
    even = sum(int(d) for d in digits[1::2])
    control = (10 - (odd + even) % 10) % 10
    return str(control), ENTITY_LETTER[control]


def check(raw):
    if not isinstance(raw, str):
        return {"input": raw, "valid": False, "reason": "not a string"}
    value = re.sub(r"[\s.\-/]", "", raw).upper()
    if value.startswith("ES") and len(value) == 11:
        value = value[2:]
    out = {"input": raw, "normalized": value, "valid": False}
    if re.fullmatch(r"\d{8}[A-Z]", value):
        want = LETTERS[int(value[:8]) % 23]
        out.update({"type": "DNI", "description": "NIF of a Spanish citizen (DNI number and letter)"})
    elif re.fullmatch(r"\d{1,7}[A-Z]", value):
        padded = value[:-1].zfill(8) + value[-1]
        return {**check(padded), "input": raw, "note": "padded with leading zeros to 8 digits"}
    elif re.fullmatch(r"[XYZ]\d{7}[A-Z]", value):
        want = LETTERS[int(str("XYZ".index(value[0])) + value[1:8]) % 23]
        out.update({"type": "NIE", "description": "identity number of a foreign national"})
    elif re.fullmatch(r"[KLM]\d{7}[A-Z]", value):
        want = LETTERS[int(value[1:8]) % 23]
        kinds = {"K": "Spaniard under 14 without DNI", "L": "Spaniard resident abroad without DNI", "M": "foreigner without NIE"}
        out.update({"type": "NIF_" + value[0], "description": f"special NIF: {kinds[value[0]]}"})
    elif re.fullmatch(r"[ABCDEFGHJNPQRSUVW]\d{7}[0-9A-J]", value):
        digit, letter = entity_control(value[1:8])
        kind = value[0]
        allowed = [letter] if kind in LETTER_ONLY else [digit] if kind in DIGIT_ONLY else [digit, letter]
        out.update({"type": "NIF_entity", "entity": ENTITY_TYPES[kind], "province_code": value[1:3], "description": "NIF of a legal entity (formerly CIF)"})
        out["valid"] = value[-1] in allowed
        if not out["valid"]:
            out["expected_check"] = allowed if len(allowed) > 1 else allowed[0]
            out["reason"] = "wrong check character"
        return out
    else:
        out["reason"] = "not the shape of a DNI, NIE, special NIF or entity NIF"
        return out
    out["valid"] = value[-1] == want
    if not out["valid"]:
        out["expected_check"] = want
        out["reason"] = "wrong check letter"
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if "id" in request:
        print(json.dumps(check(request["id"]), ensure_ascii=False))
        return
    ids = request.get("ids")
    if not isinstance(ids, list) or not ids:
        fail("give id or a non-empty ids list")
    results = [check(i) for i in ids]
    print(json.dumps({"results": results, "valid": sum(r["valid"] for r in results), "invalid": sum(not r["valid"] for r in results)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
