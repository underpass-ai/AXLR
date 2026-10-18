#!/usr/bin/env python3
"""Numbers to words: cardinals, ordinals and currency amounts in Spanish, English, French,
German and Portuguese.

Input (JSON on stdin):
  number      integer, or a decimal string such as "-1234.05" (numbers are read via str())
  language    "es" | "en" | "fr" | "de" | "pt"
  mode        "cardinal" (default) | "ordinal" | "currency"
  variant     en: "en-US" (default) | "en-GB" ("and" after hundreds);
              fr: "fr-FR" (default) | "fr-BE" (septante, nonante) | "fr-CH" (septante, huitante, nonante);
              pt: "pt-PT" (default, long scale, dezasseis) | "pt-BR" (short scale, dezesseis)
  gender      "masculine" (default) | "feminine"   (es, fr, pt; de: before_noun "eine")
  before_noun boolean: cardinal used before a noun (es un/veintiún, de ein/eine)
  orthography fr: "traditional" (default) | "1990" (every compound numeral hyphenated)
  decimals    "digits" | "number": how a cardinal's fractional part is read
              (default: digits for en and de, number for es, fr and pt)
  currency    "EUR" | "USD" | "GBP" | "CHF" | "MXN" | "BRL"   (mode currency)
  unit        {"singular", "plural", "gender", "sub_singular", "sub_plural", "sub_gender"}
              your own currency names instead of currency (100 subunits per unit)
"""
import json
import re
import sys
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation

LIMIT = 10 ** 36


def fail(message):
    print(json.dumps({"error": message}, ensure_ascii=False))
    sys.exit(2)


def groups_of(n, size):
    out = []
    while n:
        n, g = divmod(n, size)
        out.append(g)
    return out or [0]


# ---------------------------------------------------------------- English
EN_ONES = ["zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve",
           "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"]
EN_TENS = ["", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"]
EN_SCALES = ["", "thousand", "million", "billion", "trillion", "quadrillion", "quintillion", "sextillion", "septillion",
             "octillion", "nonillion", "decillion"]


def en_below_100(n):
    if n < 20:
        return EN_ONES[n]
    t, u = divmod(n, 10)
    return EN_TENS[t] + ("-" + EN_ONES[u] if u else "")


def en_below_1000(n, british):
    h, r = divmod(n, 100)
    parts = [EN_ONES[h] + " hundred"] if h else []
    if r:
        if h and british:
            parts.append("and")
        parts.append(en_below_100(r))
    return " ".join(parts)


def en_cardinal(n, opts):
    if n == 0:
        return "zero"
    british = opts["variant"] == "en-GB"
    groups = groups_of(n, 1000)
    words = []
    for k in range(len(groups) - 1, -1, -1):
        if groups[k]:
            words.append(en_below_1000(groups[k], british) + (" " + EN_SCALES[k] if k else ""))
    if british and n > 100 and 0 < groups[0] < 100 and len(words) > 1:
        words[-1] = "and " + words[-1]
    return " ".join(words)


EN_ORD = {"one": "first", "two": "second", "three": "third", "five": "fifth", "eight": "eighth", "nine": "ninth",
          "twelve": "twelfth"}


def en_ordinal(n, opts):
    words = en_cardinal(n, opts)
    head, sep, last = words.rpartition(" ")
    pre, hyphen, unit = last.rpartition("-")
    if unit in EN_ORD:
        unit = EN_ORD[unit]
    elif unit.endswith("y"):
        unit = unit[:-1] + "ieth"
    else:
        unit += "th"
    return head + sep + pre + hyphen + unit


# ---------------------------------------------------------------- Spanish
ES_UNITS = ["cero", "uno", "dos", "tres", "cuatro", "cinco", "seis", "siete", "ocho", "nueve", "diez", "once", "doce",
            "trece", "catorce", "quince", "dieciséis", "diecisiete", "dieciocho", "diecinueve", "veinte", "veintiuno",
            "veintidós", "veintitrés", "veinticuatro", "veinticinco", "veintiséis", "veintisiete", "veintiocho", "veintinueve"]
ES_TENS = ["", "", "", "treinta", "cuarenta", "cincuenta", "sesenta", "setenta", "ochenta", "noventa"]
ES_HUNDREDS = ["", "ciento", "doscientos", "trescientos", "cuatrocientos", "quinientos", "seiscientos", "setecientos",
               "ochocientos", "novecientos"]
ES_LONG = ["", "millón", "billón", "trillón", "cuatrillón", "quintillón"]


def es_one(word, form):
    """uno -> un / una at the end of a number (veintiuno, treinta y uno)."""
    if form == "apocope":
        return word[:-1] if word == "uno" else ("veintiún" if word == "veintiuno" else word)
    if form == "feminine":
        return "una" if word == "uno" else ("veintiuna" if word == "veintiuno" else word)
    return word


def es_below_1000(n, form):
    """form: standalone (uno), apocope (un), feminine (una, doscientas)."""
    if n == 100:
        return "cien"
    h, r = divmod(n, 100)
    parts = []
    if h:
        hundreds = ES_HUNDREDS[h]
        if form == "feminine" and h > 1:
            hundreds = hundreds[:-2] + "as"
        parts.append(hundreds)
    if r:
        if r < 30:
            parts.append(es_one(ES_UNITS[r], form))
        else:
            t, u = divmod(r, 10)
            parts.append(ES_TENS[t] + (" y " + es_one(ES_UNITS[u], form) if u else ""))
    return " ".join(parts)


def es_below_million(n, form):
    """form applies to the units group; the thousands group is apocopated (veintiún mil) or feminine."""
    thousands, rest = divmod(n, 1000)
    parts = []
    if thousands:
        parts.append("mil" if thousands == 1 else es_below_1000(thousands, "feminine" if form == "feminine" else "apocope") + " mil")
    if rest or not thousands:
        if rest:
            parts.append(es_below_1000(rest, form))
    return " ".join(parts)


def es_cardinal(n, opts):
    if n == 0:
        return "cero"
    form = "feminine" if opts["gender"] == "feminine" else ("apocope" if opts["before_noun"] else "standalone")
    groups = groups_of(n, 10 ** 6)
    if len(groups) > len(ES_LONG):
        fail("number too large for Spanish (long scale up to quintillones)")
    words = []
    for k in range(len(groups) - 1, 0, -1):
        g = groups[k]
        if g:
            noun = ES_LONG[k]
            words.append("un " + noun if g == 1 else es_below_million(g, "apocope") + " " + noun.replace("ó", "o") + "es")
    if groups[0]:
        words.append(es_below_million(groups[0], form))
    return " ".join(words)


ES_ORD_UNITS = ["", "primero", "segundo", "tercero", "cuarto", "quinto", "sexto", "séptimo", "octavo", "noveno"]
ES_ORD_TEENS = ["décimo", "undécimo", "duodécimo", "decimotercero", "decimocuarto", "decimoquinto", "decimosexto",
                "decimoséptimo", "decimoctavo", "decimonoveno"]
ES_ORD_TENS = ["", "", "vigésimo", "trigésimo", "cuadragésimo", "quincuagésimo", "sexagésimo", "septuagésimo",
               "octogésimo", "nonagésimo"]
ES_ORD_HUNDREDS = ["", "centésimo", "ducentésimo", "tricentésimo", "cuadringentésimo", "quingentésimo", "sexcentésimo",
                   "septingentésimo", "octingentésimo", "noningentésimo"]


def es_ord_below_1000(n):
    h, r = divmod(n, 100)
    parts = [ES_ORD_HUNDREDS[h]] if h else []
    if 10 <= r < 20:
        parts.append(ES_ORD_TEENS[r - 10])
    elif r:
        t, u = divmod(r, 10)
        if t:
            parts.append(ES_ORD_TENS[t])
        if u:
            parts.append(ES_ORD_UNITS[u])
    return parts


def es_compound(words):
    """dos mil -> dosmil, veintiún -> veintiun: written as one word, the cardinal loses its accents."""
    return words.replace(" ", "").translate(str.maketrans("áéíóú", "aeiou"))


def es_ordinal(n, opts):
    if n >= 10 ** 9:
        fail("Spanish ordinals are given below one thousand million")
    millions, rest = divmod(n, 10 ** 6)
    thousands, units = divmod(rest, 1000)
    parts = []
    if millions:
        parts.append("millonésimo" if millions == 1 else es_compound(es_below_million(millions, "apocope")) + "millonésimo")
    if thousands:
        parts.append("milésimo" if thousands == 1 else es_compound(es_below_1000(thousands, "apocope")) + "milésimo")
    parts += es_ord_below_1000(units)
    if opts["gender"] == "feminine":
        parts = [p[:-1] + "a" for p in parts]
    elif opts["before_noun"] and parts[-1] in ("primero", "tercero", "decimotercero"):
        parts[-1] = parts[-1][:-1]
    return " ".join(parts)


# ---------------------------------------------------------------- Portuguese
PT_UNITS = ["zero", "um", "dois", "três", "quatro", "cinco", "seis", "sete", "oito", "nove", "dez", "onze", "doze", "treze",
            "catorze", "quinze", "dezasseis", "dezassete", "dezoito", "dezanove"]
PT_BR_TEENS = {16: "dezesseis", 17: "dezessete", 19: "dezenove"}
PT_TENS = ["", "", "vinte", "trinta", "quarenta", "cinquenta", "sessenta", "setenta", "oitenta", "noventa"]
PT_HUNDREDS = ["", "cento", "duzentos", "trezentos", "quatrocentos", "quinhentos", "seiscentos", "setecentos", "oitocentos",
               "novecentos"]
PT_LONG = ["", "milhão", "bilião", "trilião", "quatrilião", "quintilião"]
PT_SHORT = ["", "mil", "milhão", "bilhão", "trilhão", "quatrilhão", "quintilhão", "sextilhão", "septilhão", "octilhão",
            "nonilhão", "decilhão"]


def pt_unit(n, feminine, brazil):
    if feminine and n in (1, 2):
        return "uma" if n == 1 else "duas"
    if brazil and n in PT_BR_TEENS:
        return PT_BR_TEENS[n]
    return PT_UNITS[n]


def pt_below_1000(n, feminine, brazil):
    if n == 100:
        return "cem"
    h, r = divmod(n, 100)
    parts = []
    if h:
        parts.append(PT_HUNDREDS[h][:-2] + "as" if feminine and h > 1 else PT_HUNDREDS[h])
    if r:
        if r < 20:
            parts.append(pt_unit(r, feminine, brazil))
        else:
            t, u = divmod(r, 10)
            parts.append(PT_TENS[t] + (" e " + pt_unit(u, feminine, brazil) if u else ""))
    return " e ".join(parts)


def pt_plural(noun):
    return noun[:-2] + "ões"


def pt_cardinal(n, opts):
    if n == 0:
        return "zero"
    feminine = opts["gender"] == "feminine"
    brazil = opts["variant"] == "pt-BR"
    parts = []  # (words, value of this 1000-group)
    if brazil:
        groups = groups_of(n, 1000)
        if len(groups) > len(PT_SHORT):
            fail("number too large for Portuguese (short scale up to decilhões)")
        for k in range(len(groups) - 1, -1, -1):
            g = groups[k]
            if not g:
                continue
            if k == 0:
                parts.append((pt_below_1000(g, feminine, True), g))
            elif k == 1:
                parts.append(("mil" if g == 1 else pt_below_1000(g, feminine, True) + " mil", g))
            else:
                noun = PT_SHORT[k]
                parts.append(("um " + noun if g == 1 else pt_below_1000(g, False, True) + " " + pt_plural(noun), g))
    else:
        groups = groups_of(n, 10 ** 6)
        if len(groups) > len(PT_LONG):
            fail("number too large for Portuguese (long scale up to quintiliões)")
        for k in range(len(groups) - 1, -1, -1):
            g = groups[k]
            if not g:
                continue
            thousands, rest = divmod(g, 1000)
            fem = feminine and k == 0
            if k == 0:
                if thousands:
                    parts.append(("mil" if thousands == 1 else pt_below_1000(thousands, fem, False) + " mil", thousands))
                if rest:
                    parts.append((pt_below_1000(rest, fem, False), rest))
            else:
                noun = PT_LONG[k]
                if g == 1:
                    parts.append(("um " + noun, 1))
                else:
                    head = ("mil" if thousands == 1 else pt_below_1000(thousands, False, False) + " mil") if thousands else ""
                    tail = pt_below_1000(rest, False, False) if rest else ""
                    if head and tail:
                        head += " e " if (rest < 100 or rest % 100 == 0) else " "
                    parts.append((head + tail + " " + pt_plural(noun), rest or thousands))
    words = [w for w, _ in parts]
    last_value = parts[-1][1]
    if len(parts) > 1 and (last_value < 100 or last_value % 100 == 0):
        words[-1] = "e " + words[-1]
    return " ".join(words)


PT_ORD_UNITS = ["", "primeiro", "segundo", "terceiro", "quarto", "quinto", "sexto", "sétimo", "oitavo", "nono"]
PT_ORD_TENS = ["", "décimo", "vigésimo", "trigésimo", "quadragésimo", "quinquagésimo", "sexagésimo", "septuagésimo",
               "octogésimo", "nonagésimo"]
PT_ORD_HUNDREDS = ["", "centésimo", "ducentésimo", "tricentésimo", "quadringentésimo", "quingentésimo", "sexcentésimo",
                   "septingentésimo", "octingentésimo", "nongentésimo"]


def pt_ordinal(n, opts):
    if n >= 2000:
        fail("Portuguese ordinals are given up to 1999")
    thousands, rest = divmod(n, 1000)
    h, r = divmod(rest, 100)
    t, u = divmod(r, 10)
    parts = (["milésimo"] if thousands else []) + ([PT_ORD_HUNDREDS[h]] if h else []) + ([PT_ORD_TENS[t]] if t else []) \
        + ([PT_ORD_UNITS[u]] if u else [])
    if opts["gender"] == "feminine":
        parts = [p[:-1] + "a" for p in parts]
    return " ".join(parts)


# ---------------------------------------------------------------- French
FR_UNITS = ["zéro", "un", "deux", "trois", "quatre", "cinq", "six", "sept", "huit", "neuf", "dix", "onze", "douze", "treize",
            "quatorze", "quinze", "seize", "dix-sept", "dix-huit", "dix-neuf"]
FR_TENS = ["", "", "vingt", "trente", "quarante", "cinquante", "soixante", "septante", "huitante", "nonante"]
FR_SCALES = ["", "mille", "million", "milliard", "billion", "billiard", "trillion", "trilliard", "quadrillion",
             "quadrilliard", "quintillion", "quintilliard"]


def fr_below_100(n, variant, feminine):
    one = "une" if feminine else "un"
    if n < 20:
        return one if n == 1 else FR_UNITS[n]
    t, u = divmod(n, 10)
    swiss = variant == "fr-CH"
    belgian = variant in ("fr-BE", "fr-CH")
    if t in (7, 9) and not belgian:
        base = "soixante" if t == 7 else "quatre-vingt"
        if t == 7 and u == 1:
            return "soixante et onze"
        return base + "-" + FR_UNITS[10 + u]
    if t == 8 and not swiss:
        return "quatre-vingts" if u == 0 else "quatre-vingt-" + (one if u == 1 else FR_UNITS[u])
    if u == 0:
        return FR_TENS[t]
    if u == 1:
        return FR_TENS[t] + " et " + one
    return FR_TENS[t] + "-" + FR_UNITS[u]


def fr_below_1000(n, variant, feminine):
    h, r = divmod(n, 100)
    parts = []
    if h:
        parts.append(("" if h == 1 else FR_UNITS[h] + " ") + ("cents" if h > 1 and r == 0 else "cent"))
    if r:
        parts.append(fr_below_100(r, variant, feminine))
    return " ".join(parts)


def fr_cardinal(n, opts):
    if n == 0:
        return "zéro"
    feminine = opts["gender"] == "feminine"
    variant, reform = opts["variant"], opts["orthography"] == "1990"
    groups = groups_of(n, 1000)
    if len(groups) > len(FR_SCALES):
        fail("number too large for French (up to quintilliards)")
    chunks = []  # (text, is_noun_scale)
    for k in range(len(groups) - 1, -1, -1):
        g = groups[k]
        if not g:
            continue
        if k == 0:
            chunks.append((fr_below_1000(g, variant, feminine), False))
        elif k == 1:
            if g == 1:
                chunks.append(("mille", False))
            else:
                head = fr_below_1000(g, variant, False)
                if head.endswith("cents") or head.endswith("vingts"):
                    head = head[:-1]  # deux cent mille, quatre-vingt mille
                chunks.append((head + " mille", False))
        else:
            noun = FR_SCALES[k]
            chunks.append(("un " + noun if g == 1 else fr_below_1000(g, variant, False) + " " + noun + "s", True))
    if not reform:
        return " ".join(text for text, _ in chunks)
    out = []
    for text, noun in chunks:
        if noun:
            number, _, scale = text.rpartition(" ")
            out.append(number.replace(" ", "-") + " " + scale)
        else:
            out.append(text.replace(" ", "-"))
    joined = []
    for i, (text, noun) in enumerate(chunks):
        piece = out[i]
        if joined and not chunks[i - 1][1] and not noun:
            joined[-1] += "-" + piece
        else:
            joined.append(piece)
    return " ".join(joined)


def fr_ordinal(n, opts):
    if n == 1:
        return "première" if opts["gender"] == "feminine" else "premier"
    words = fr_cardinal(n, dict(opts, gender="masculine"))
    if re.match(r"^un (million|milliard|billion|billiard|trillion|trilliard|quadrillion|quadrilliard|quintillion|quintilliard)s?$", words):
        words = words[3:]
    if words.endswith(("vingts", "cents")) or re.search(r"(llion|lliard)s$", words):
        words = words[:-1]
    if words.endswith("cinq"):
        return words + "uième"
    if words.endswith("neuf"):
        return words[:-1] + "vième"
    if words.endswith("e"):
        return words[:-1] + "ième"
    return words + "ième"


# ---------------------------------------------------------------- German
DE_UNITS = ["null", "eins", "zwei", "drei", "vier", "fünf", "sechs", "sieben", "acht", "neun", "zehn", "elf", "zwölf",
            "dreizehn", "vierzehn", "fünfzehn", "sechzehn", "siebzehn", "achtzehn", "neunzehn"]
DE_TENS = ["", "", "zwanzig", "dreißig", "vierzig", "fünfzig", "sechzig", "siebzig", "achtzig", "neunzig"]
DE_SCALES = ["", "tausend", "Million", "Milliarde", "Billion", "Billiarde", "Trillion", "Trilliarde", "Quadrillion",
             "Quadrilliarde", "Quintillion", "Quintilliarde"]


def de_below_100(n, final):
    if n < 20:
        return ("eins" if final else "ein") if n == 1 else DE_UNITS[n]
    t, u = divmod(n, 10)
    if not u:
        return DE_TENS[t]
    return ("ein" if u == 1 else DE_UNITS[u]) + "und" + DE_TENS[t]


def de_below_1000(n, final):
    h, r = divmod(n, 100)
    out = (("ein" if h == 1 else DE_UNITS[h]) + "hundert") if h else ""
    return out + (de_below_100(r, final) if r else "")


def de_cardinal(n, opts):
    if n == 0:
        return "null"
    if n == 1 and opts["before_noun"]:
        return "eine" if opts["gender"] == "feminine" else "ein"
    groups = groups_of(n, 1000)
    if len(groups) > len(DE_SCALES):
        fail("number too large for German (up to Quintilliarden)")
    words = []
    for k in range(len(groups) - 1, 1, -1):
        g = groups[k]
        if g:
            noun = DE_SCALES[k]
            plural = noun + ("n" if noun.endswith("e") else "en")
            words.append("eine " + noun if g == 1 else de_below_1000(g, True) + " " + plural)
    low = ""
    if len(groups) > 1 and groups[1]:
        low = de_below_1000(groups[1], False) + "tausend"
    if groups[0]:
        low += de_below_1000(groups[0], True)
    if low:
        words.append(low)
    return " ".join(words)


DE_ORD = {"eins": "erste", "drei": "dritte", "sieben": "siebte", "acht": "achte"}


def de_ordinal(n, opts):
    words = de_cardinal(n, dict(opts, before_noun=False))
    words = words.replace("eine ", "ein").replace(" ", "").lower()
    r = n % 100
    if 0 < r < 20:
        for cardinal in sorted(DE_UNITS[1:20], key=len, reverse=True):
            if words.endswith(cardinal):
                stem = words[: -len(cardinal)]
                return stem + DE_ORD.get(cardinal, cardinal + "te")
    return words + "ste"


# ---------------------------------------------------------------- shared
CARDINAL = {"en": en_cardinal, "es": es_cardinal, "fr": fr_cardinal, "de": de_cardinal, "pt": pt_cardinal}
ORDINAL = {"en": en_ordinal, "es": es_ordinal, "fr": fr_ordinal, "de": de_ordinal, "pt": pt_ordinal}
MINUS = {"en": "minus", "es": "menos", "fr": "moins", "de": "minus", "pt": "menos"}
POINT = {"en": "point", "es": "coma", "fr": "virgule", "de": "Komma", "pt": "vírgula"}
JOIN_CENTS = {"en": " and ", "es": " con ", "fr": " et ", "de": " und ", "pt": " e "}
VARIANTS = {"en": ("en-US", "en-GB"), "fr": ("fr-FR", "fr-BE", "fr-CH"), "pt": ("pt-PT", "pt-BR"), "es": ("es",), "de": ("de",)}

# singular, plural, gender, subunit singular, subunit plural, subunit gender
CURRENCIES = {
    "EUR": {"en": ("euro", "euros", "m", "cent", "cents", "m"), "es": ("euro", "euros", "m", "céntimo", "céntimos", "m"),
            "fr": ("euro", "euros", "m", "centime", "centimes", "m"), "de": ("Euro", "Euro", "m", "Cent", "Cent", "m"),
            "pt": ("euro", "euros", "m", "cêntimo", "cêntimos", "m"), "pt-BR": ("euro", "euros", "m", "centavo", "centavos", "m")},
    "USD": {"en": ("dollar", "dollars", "m", "cent", "cents", "m"), "es": ("dólar", "dólares", "m", "centavo", "centavos", "m"),
            "fr": ("dollar", "dollars", "m", "cent", "cents", "m"), "de": ("Dollar", "Dollar", "m", "Cent", "Cent", "m"),
            "pt": ("dólar", "dólares", "m", "cêntimo", "cêntimos", "m"), "pt-BR": ("dólar", "dólares", "m", "centavo", "centavos", "m")},
    "GBP": {"en": ("pound", "pounds", "m", "penny", "pence", "m"), "es": ("libra", "libras", "f", "penique", "peniques", "m"),
            "fr": ("livre", "livres", "f", "penny", "pence", "m"), "de": ("Pfund", "Pfund", "n", "Penny", "Pence", "m"),
            "pt": ("libra", "libras", "f", "péni", "pence", "m"), "pt-BR": ("libra", "libras", "f", "pêni", "pence", "m")},
    "CHF": {"en": ("franc", "francs", "m", "centime", "centimes", "m"), "es": ("franco", "francos", "m", "céntimo", "céntimos", "m"),
            "fr": ("franc", "francs", "m", "centime", "centimes", "m"), "de": ("Franken", "Franken", "m", "Rappen", "Rappen", "m"),
            "pt": ("franco", "francos", "m", "cêntimo", "cêntimos", "m"), "pt-BR": ("franco", "francos", "m", "centavo", "centavos", "m")},
    "MXN": {"en": ("peso", "pesos", "m", "centavo", "centavos", "m"), "es": ("peso", "pesos", "m", "centavo", "centavos", "m"),
            "fr": ("peso", "pesos", "m", "centavo", "centavos", "m"), "de": ("Peso", "Pesos", "m", "Centavo", "Centavos", "m"),
            "pt": ("peso", "pesos", "m", "centavo", "centavos", "m"), "pt-BR": ("peso", "pesos", "m", "centavo", "centavos", "m")},
    "BRL": {"en": ("real", "reais", "m", "centavo", "centavos", "m"), "es": ("real", "reales", "m", "centavo", "centavos", "m"),
            "fr": ("réal", "réaux", "m", "centavo", "centavos", "m"), "de": ("Real", "Real", "m", "Centavo", "Centavos", "m"),
            "pt": ("real", "reais", "m", "centavo", "centavos", "m"), "pt-BR": ("real", "reais", "m", "centavo", "centavos", "m")},
}


def parse_number(value):
    if isinstance(value, bool) or not isinstance(value, (int, float, str)):
        fail("number must be an integer, a number or a decimal string")
    text = str(value).strip().replace("_", "")
    if isinstance(value, str) and not re.fullmatch(r"[+\-]?(\d+(\.\d*)?|\.\d+)([eE][+\-]?\d+)?", text):
        fail("number string must look like -1234.56 (point as decimal separator, no thousands separators)")
    try:
        number = Decimal(text)
    except InvalidOperation:
        fail("number is not a valid decimal")
    if not number.is_finite():
        fail("number must be finite")
    if isinstance(value, str) and "." in text and "e" not in text.lower():
        fraction = text.split(".", 1)[1]
    else:
        fixed = format(number, "f")
        fraction = fixed.split(".", 1)[1].rstrip("0") if "." in fixed else ""
    integer = int(abs(number).to_integral_value(rounding="ROUND_DOWN"))
    return number, number < 0, integer, fraction


def read_fraction(fraction, language, style, opts):
    if style == "digits":
        names = [CARDINAL[language](int(d), dict(opts, gender="masculine", before_noun=False)) for d in fraction]
        return " ".join(names)
    zeros = len(fraction) - len(fraction.lstrip("0"))
    rest = fraction.lstrip("0")
    zero = CARDINAL[language](0, opts)
    words = [zero] * zeros
    if rest:
        if len(rest) > 36:
            fail("too many decimal digits to read as a number; use decimals: digits")
        words.append(CARDINAL[language](int(rest), dict(opts, gender="masculine", before_noun=False)))
    return " ".join(words)


def counted(n, names, language, opts, is_sub):
    singular, plural, gender = names[3:6] if is_sub else names[0:3]
    o = dict(opts, gender="feminine" if gender == "f" else "masculine", before_noun=True)
    words = CARDINAL[language](n, o)
    noun = singular if n == 1 else plural
    if language in ("es", "pt", "fr") and n and n % 10 ** 6 == 0:
        if language == "fr" and noun[:1].lower() in "aeiouyhéèê":
            return words + " d'" + noun
        return words + " de " + noun
    return words + " " + noun


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"number", "language", "mode", "variant", "gender", "before_noun", "orthography", "decimals", "currency", "unit"}
    unknown = set(request) - allowed
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    language = request.get("language")
    if language not in CARDINAL:
        fail("language must be es, en, fr, de or pt")
    mode = request.get("mode", "cardinal")
    if mode not in ("cardinal", "ordinal", "currency"):
        fail("mode must be cardinal, ordinal or currency")
    variant = request.get("variant", VARIANTS[language][0])
    if variant not in VARIANTS[language]:
        fail(f"variant for {language} must be one of {', '.join(VARIANTS[language])}")
    gender = request.get("gender", "masculine")
    if gender not in ("masculine", "feminine"):
        fail("gender must be masculine or feminine")
    before_noun = request.get("before_noun", False)
    if not isinstance(before_noun, bool):
        fail("before_noun must be a boolean")
    orthography = request.get("orthography", "traditional")
    if orthography not in ("traditional", "1990"):
        fail("orthography must be traditional or 1990")
    decimals = request.get("decimals", "digits" if language in ("en", "de") else "number")
    if decimals not in ("digits", "number"):
        fail("decimals must be digits or number")
    if "number" not in request:
        fail("number is required")
    number, negative, integer, fraction = parse_number(request["number"])
    if integer >= LIMIT:
        fail("number must be below 10^36")
    opts = {"variant": variant, "gender": gender, "before_noun": before_noun, "orthography": orthography}
    result = {"number": request["number"] if isinstance(request["number"], str) else str(request["number"]),
              "language": language, "mode": mode, "variant": variant}

    if mode == "ordinal":
        if negative or fraction.strip("0") or integer < 1:
            fail("ordinals need a positive integer")
        result["words"] = ORDINAL[language](integer, opts)
        result["gender"] = gender
    elif mode == "cardinal":
        words = CARDINAL[language](integer, opts)
        if fraction:
            words += " " + POINT[language] + " " + read_fraction(fraction, language, decimals, opts)
        if negative and (integer or fraction.strip("0")):
            words = MINUS[language] + " " + words
        result["words"] = words
        result["scale"] = "short" if language == "en" or variant == "pt-BR" else "long"
        if fraction:
            result["decimals"] = decimals
    else:
        unit = request.get("unit")
        code = request.get("currency")
        if unit is not None:
            keys = ("singular", "plural", "gender", "sub_singular", "sub_plural", "sub_gender")
            if not isinstance(unit, dict) or set(unit) - set(keys) or not {"singular", "plural"} <= set(unit):
                fail("unit must be an object with singular and plural (optional gender m/f, sub_singular, sub_plural, sub_gender)")
            names = (unit["singular"], unit["plural"], unit.get("gender", "m"), unit.get("sub_singular", ""),
                     unit.get("sub_plural", ""), unit.get("sub_gender", "m"))
            if not all(isinstance(x, str) for x in names) or names[2] not in ("m", "f", "n") or names[5] not in ("m", "f", "n"):
                fail("unit names must be strings and genders m, f or n")
            code = "custom"
        else:
            if code not in CURRENCIES:
                fail("currency must be one of " + ", ".join(CURRENCIES) + " (or give unit)")
            table = CURRENCIES[code]
            names = table.get(variant, table[language]) if variant == "pt-BR" else table[language]
        if negative:
            fail("currency amounts must not be negative")
        cents_total = (abs(number) * 100).quantize(Decimal(1), rounding=ROUND_HALF_UP)
        units, cents = divmod(int(cents_total), 100)
        if cents and not names[3]:
            fail("this unit has no subunit names for the cents")
        if units >= LIMIT:
            fail("amount must be below 10^36")
        pieces = []
        if units or not cents:
            pieces.append(counted(units, names, language, opts, False))
        if cents:
            pieces.append(counted(cents, names, language, opts, True))
        result["currency"] = code
        result["amount"] = f"{units}.{cents:02d}"
        result["words"] = JOIN_CENTS[language].join(pieces)
        if (abs(number) * 100) != cents_total:
            result["rounded"] = "the amount was rounded half up to whole subunits"
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
