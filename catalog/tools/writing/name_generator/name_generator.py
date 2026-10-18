#!/usr/bin/env python3
"""Seeded name generator for characters, places and products, by style, from syllable tables
written for this tool. The same input (seed included) always gives the same names.

Input (JSON on stdin):
  kind        "character" | "place" | "product"
  style       character: spanish, english, nordic, japanese, greek, fantasy
              place: spanish, english, nordic, fantasy;  product: tech, pharma, brand
  count       names wanted (default 10, 1 to 200)
  seed        integer (default 0); random.Random(seed) drives every choice
  min_length, max_length   letters per generated word (default 3 and 12)
  gender      character: "any" (default) | "female" | "male"
  surname     character: add a surname (default true)
  unique      no name twice (default true)
  list_styles true to list the styles and stop
"""
import json
import random
import sys

# Each table: start syllables, middle syllables (0 to max_middle of them), endings.
GIVEN = {
    "spanish": {"start": ["Al", "Be", "Car", "Da", "E", "Fer", "Gon", "I", "Ja", "Lu", "Ma", "Ne", "O", "Pa",
                          "Ra", "Se", "Te", "Va", "Xi", "Ro", "Cla", "Ma", "Ni", "Gua"],
                "middle": ["lo", "ri", "ma", "ne", "li", "ta", "ber", "dro", "nes", "ci"],
                "female": ["a", "ia", "ela", "ina", "ira", "na", "la", "sa", "da", "ra"],
                "male": ["o", "io", "el", "an", "ín", "és", "ar", "no", "lo", "do", "ro", "to"],
                "max_middle": 1},
    "english": {"start": ["Ar", "Bran", "Char", "Ed", "El", "Har", "Jo", "Mar", "Ol", "Rob", "Sam", "Tho", "Wil",
                          "Ev", "Gra", "Ha", "Lu", "Ro"],
                "middle": ["a", "e", "i", "o", "en", "er", "li"],
                "female": ["ella", "ie", "y", "a", "ine", "elle", "ia", "lyn", "na", "ry"],
                "male": ["ert", "son", "ton", "an", "ard", "iam", "er", "ric", "win", "rick"],
                "max_middle": 1},
    "nordic": {"start": ["As", "Bjor", "Ei", "Frey", "Gun", "Hal", "Ing", "Kjell", "Lei", "Ols", "Ragn", "Sig",
                         "Thor", "Ulf", "Vig", "Ast", "Sol"],
               "middle": ["a", "e", "i", "ul", "var", "dis"],
               "female": ["a", "rid", "hild", "dis", "un", "veig", "ny"],
               "male": ["ar", "ulf", "mund", "geir", "stein", "ald", "var"],
               "max_middle": 1},
    "japanese": {"start": ["A", "Ha", "Hi", "Ka", "Ke", "Ki", "Ma", "Mi", "Na", "Ri", "Sa", "Shi", "Ta", "To",
                           "Yu", "Ya", "Ko", "No"],
                 "middle": ["ka", "ki", "mi", "na", "ri", "sa", "ta", "ya", "zu", "ro"],
                 "female": ["ko", "mi", "na", "ka", "e", "ri"], "male": ["ro", "ta", "shi", "to", "ki", "ya"],
                 "max_middle": 1},
    "greek": {"start": ["Al", "Ari", "Dem", "Eu", "Kal", "Lys", "Nik", "Phil", "Theo", "Xan", "Zen", "Iph", "Pen",
                        "Kle", "Myr"],
              "middle": ["o", "e", "i", "an", "ar", "ip"],
              "female": ["a", "ia", "ope", "ene", "is", "ione", "lia", "ra", "na"],
              "male": ["os", "as", "on", "es", "ios", "ander", "los", "tes", "mos"],
              "max_middle": 1},
    "fantasy": {"start": ["Ael", "Bry", "Cae", "Dra", "Eld", "Fae", "Gal", "Ith", "Kael", "Lor", "Mor", "Nym",
                          "Ory", "Syl", "Thal", "Vey", "Zar", "Ry"],
                "middle": ["a", "e", "i", "o", "an", "ar", "el", "ith", "or", "yn"],
                "female": ["a", "ia", "wyn", "elle", "eth", "ira", "is", "lia", "ra"],
                "male": ["or", "an", "eth", "ion", "us", "ar", "ik", "dor", "mir"],
                "max_middle": 2},
}
SURNAME = {
    "spanish": {"start": ["Gar", "Fer", "Ló", "Mar", "Ro", "Sán", "Gó", "Díaz", "Ruiz", "Mo", "Al", "Var", "Cas", "Na"],
                "middle": ["", "ci", "ri", "re", "va", "ta"],
                "end": ["ez", "a", "es", "ero", "illo", "ón", "al", "rez", "nez", "dez", "ra", "do", "les"]},
    "english": {"start": ["Ash", "Black", "Brad", "Carl", "Hart", "Hill", "Mill", "Stan", "Thorn", "West", "Wood"],
                "middle": [""], "end": ["ford", "ley", "ton", "wood", "er", "son", "well", "field"]},
    "nordic": {"start": ["Berg", "Dahl", "Hag", "Lund", "Ny", "Sand", "Strand", "Lind", "Holm", "Vik"],
               "middle": [""], "end": ["sen", "son", "berg", "dal", "ström", "gren", "lund", "qvist"]},
    "japanese": {"start": ["Ya", "Ta", "Su", "Wa", "Ko", "Na", "Ma", "I", "Ha", "Mo"],
                 "middle": ["ma", "na", "ka", "ta", "mo", "shi"], "end": ["da", "ka", "moto", "mura", "no", "ta", "zaki"]},
    "greek": {"start": ["Pap", "Ant", "Kon", "Dim", "Geor", "Nik", "Vas", "Ioan"],
              "middle": ["a", "o", "i"], "end": ["opoulos", "idis", "akis", "ou", "as", "eas"]},
    "fantasy": {"start": ["Storm", "Shadow", "Iron", "Silver", "Raven", "Frost", "Oak", "Ember", "Moon"],
                "middle": [""], "end": ["wind", "blade", "vale", "song", "heart", "fall", "shield", "brook"]},
}
PLACE = {
    "spanish": {"prefix": ["", "", "", "Villa", "San ", "Santa ", "Puerto ", "Castro"],
                "root": ["Al", "Bena", "Cala", "Monte", "Torre", "Val", "Peña", "Rio", "Agua", "Sierra", "Fuen"],
                "end": ["mar", "verde", "dolid", "seca", "llana", "rubia", "fría", "real", "nueva", "alta", "ejo"]},
    "english": {"prefix": ["", "", "", "", "Upper ", "Lower ", "Great ", "Little "],
                "root": ["Ash", "Brad", "Chel", "Dun", "Ely", "Ford", "Glen", "Hal", "Kings", "Lang", "Mar", "Stan",
                         "Thorn", "Wey"],
                "end": ["ton", "ford", "bury", "ham", "wick", "by", "field", "mouth", "stead", "worth", "ley"]},
    "nordic": {"prefix": [""],
               "root": ["Ar", "Berg", "Fjell", "Gran", "Hav", "Is", "Kal", "Lille", "Nor", "Sol", "Ty", "Vest"],
               "end": ["vik", "heim", "fjord", "dal", "by", "holm", "nes", "vann", "strand", "sund"]},
    "fantasy": {"prefix": ["", "", "", "Port ", "Mount ", "Fort "],
                "root": ["Ael", "Dun", "Eld", "Gal", "Kor", "Mor", "Ny", "Ost", "Syl", "Tor", "Val", "Zar"],
                "end": ["haven", "dor", "mere", "gard", "wyn", "thal", "spire", "reach", "moor", "ia"]},
}
PRODUCT = {
    "tech": {"start": ["Ze", "Lu", "No", "Qua", "Vi", "Syn", "Ora", "Flu", "Ki", "Pi", "Xe", "Bri"],
             "middle": ["", "", "ma", "ne", "ro", "va", "li"], "end": ["ify", "ly", "io", "ex", "ix", "ora", "nova", "a", "on"]},
    "pharma": {"start": ["Ab", "Cel", "Dex", "Flo", "Lev", "Nor", "Pra", "Tri", "Ven", "Zy", "Ox", "Mir"],
               "middle": ["a", "i", "o", "e"], "end": ["tex", "rol", "zine", "vax", "dara", "lix", "mab", "tan", "pril"]},
    "brand": {"start": ["Ba", "Ko", "Lo", "Mi", "Ru", "Sa", "Te", "Vo", "Za", "Pa", "Ne", "Do"],
              "middle": ["", "", "ra", "li", "mo", "ka"], "end": ["", "ko", "na", "lo", "ra", "o", "ia"]},
}
STYLES = {"character": sorted(GIVEN), "place": sorted(PLACE), "product": sorted(PRODUCT)}
DEFAULT = {"character": "english", "place": "english", "product": "tech"}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def get_int(request, field, default, low, high):
    value = request.get(field, default)
    if not isinstance(value, int) or isinstance(value, bool) or not low <= value <= high:
        fail(f"{field} must be an integer from {low} to {high}")
    return value


def get_bool(request, field, default):
    value = request.get(field, default)
    if not isinstance(value, bool):
        fail(f"{field} must be true or false")
    return value


def letters(word):
    return sum(c.isalpha() for c in word)


VOWELS = set("aeiouyáéíóúäöåø")


def pick(rng, options, previous):
    """Choose a syllable; after a vowel, prefer one that starts with a consonant (no 'Eloo')."""
    if previous and previous[-1].lower() in VOWELS:
        fitting = [o for o in options if not o or o[0].lower() not in VOWELS]
        if fitting:
            options = fitting
    return rng.choice(options)


def build(rng, start, middle, end, max_middle):
    parts = [rng.choice(start)]
    for _ in range(rng.randint(0, max_middle)):
        parts.append(pick(rng, middle, "".join(parts)))
    parts.append(pick(rng, end, "".join(parts)))
    word = "".join(parts)
    return word[:1].upper() + word[1:], [p for p in parts if p]


def generate(rng, kind, style, gender, surname):
    if kind == "character":
        table = GIVEN[style]
        sex = gender if gender != "any" else rng.choice(["female", "male"])
        given, parts = build(rng, table["start"], table["middle"], table[sex], table["max_middle"])
        words, all_parts = [given], [parts]
        if surname:
            s = SURNAME[style]
            family, fparts = build(rng, s["start"], s["middle"], s["end"], 1)
            words.append(family)
            all_parts.append(fparts)
        return words, all_parts, sex
    if kind == "place":
        table = PLACE[style]
        prefix = rng.choice(table["prefix"])
        root, parts = build(rng, table["root"], [""], table["end"], 0)
        words = [prefix.strip(), root] if prefix.endswith(" ") else [prefix + root.lower() if prefix else root]
        return words, [[prefix.strip()] if prefix.strip() else [], parts], None
    table = PRODUCT[style]
    word, parts = build(rng, table["start"], table["middle"], table["end"], 1)
    return [word], [parts], None


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    if get_bool(request, "list_styles", False):
        print(json.dumps({"styles": STYLES}))
        return
    kind = request.get("kind")
    if not isinstance(kind, str) or kind not in STYLES:
        fail("kind must be character, place or product")
    style = request.get("style", DEFAULT[kind])
    if style not in STYLES[kind]:
        fail(f"style for {kind} must be one of " + ", ".join(STYLES[kind]))
    count = get_int(request, "count", 10, 1, 200)
    seed = request.get("seed", 0)
    if not isinstance(seed, int) or isinstance(seed, bool):
        fail("seed must be an integer")
    min_len = get_int(request, "min_length", 3, 1, 40)
    max_len = get_int(request, "max_length", 12, 1, 40)
    if min_len > max_len:
        fail("min_length is greater than max_length")
    gender = request.get("gender", "any")
    if gender not in ("any", "female", "male"):
        fail("gender must be any, female or male")
    surname = get_bool(request, "surname", True)
    unique = get_bool(request, "unique", True)
    rng = random.Random(seed)
    names, seen, attempts = [], set(), 0
    while len(names) < count and attempts < count * 200:
        attempts += 1
        words, parts, sex = generate(rng, kind, style, gender, surname)
        checked = [w for w in words if w and " " not in w] if kind != "place" else [words[-1]]
        if not all(min_len <= letters(w) <= max_len for w in checked):
            continue
        name = " ".join(w for w in words if w)
        if unique and name in seen:
            continue
        seen.add(name)
        item = {"name": name, "parts": parts}
        if sex:
            item["gender"] = sex
        names.append(item)
    lengths = [letters(w) for item in names for w in (item["name"].split() if kind == "character" else item["name"].split()[-1:])]
    checks = {"within_bounds": all(min_len <= n <= max_len for n in lengths),
              "unique": len({item["name"] for item in names}) == len(names) if unique else None}
    result = {"kind": kind, "style": style, "seed": seed, "count": len(names), "names": names, "checks": checks,
              "constraints": {"min_length": min_len, "max_length": max_len, "unique": unique,
                              "applied_to": "each generated word (given name and surname; a place's main word)"},
              "generator": "Python random.Random(seed), Mersenne Twister; syllable tables of this tool (MIT)"}
    if len(names) < count:
        result["warning"] = f"only {len(names)} names met the constraints in {attempts} attempts"
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
