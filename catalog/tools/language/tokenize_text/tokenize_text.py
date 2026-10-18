#!/usr/bin/env python3
"""Unicode-aware word and sentence tokenisation with counts and offsets.

Tokens, in order of precedence at each position: URLs, e-mail addresses, numbers, words
(letters, combining marks and digits, with internal apostrophes and hyphens), single CJK
characters (Han, hiragana, katakana), and punctuation or symbols. Sentences end at . ! ? …
and the CJK full stops, except after the abbreviations of the chosen language, initials,
dotted acronyms, German ordinals and before a lowercase word.

Input (JSON on stdin):
  text        string (required)
  language    "en" | "es" | "fr" | "de"   (abbreviation list; default "en")
  tokens      boolean, include the token list (default true)
  sentences   boolean, include the sentence list (default true)
  top         integer, most frequent words to report (default 10, 0 for none)
  lowercase   boolean, case-fold words for frequencies and unique counts (default true)
Offsets are code-point indexes into text, end exclusive.
"""
import json
import re
import sys
import unicodedata

ABBREVIATIONS = {
    "en": {"mr", "mrs", "ms", "dr", "prof", "sr", "jr", "st", "mt", "vs", "etc", "e.g", "i.e", "cf", "approx", "dept",
           "no", "nos", "vol", "vols", "fig", "figs", "p", "pp", "ch", "ed", "eds", "est", "inc", "ltd", "co", "corp",
           "jan", "feb", "mar", "apr", "jun", "jul", "aug", "sep", "sept", "oct", "nov", "dec", "gen", "col", "lt",
           "sgt", "capt", "gov", "rev", "hon", "u.s", "u.k", "a.m", "p.m", "ave", "blvd", "rd"},
    "es": {"sr", "sra", "srta", "sres", "dr", "dra", "dres", "d", "dña", "da", "lic", "ing", "arq", "prof", "profa", "excmo",
           "excma", "ilmo", "ilma", "etc", "ej", "p", "pp", "pág", "págs", "núm", "n", "art", "arts", "cap", "caps", "vol",
           "vols", "ed", "fig", "aprox", "cía", "admón", "av", "avda", "c", "pl", "pza", "dpto", "depto", "tel", "telf",
           "ud", "uds", "vd", "vds", "ee.uu", "s.a", "s.l", "a.c", "d.c", "cf", "vid", "op", "cit", "ibíd", "id",
           "ene", "feb", "mar", "abr", "jun", "jul", "ago", "sept", "oct", "nov", "dic", "aa.vv", "sto", "sta", "fr"},
    "fr": {"m", "mm", "mme", "mmes", "mlle", "mlles", "dr", "pr", "me", "mgr", "st", "ste", "etc", "cf", "p", "pp", "vol",
           "chap", "art", "fig", "env", "réf", "n", "no", "nos", "av", "bd", "boul", "c.-à-d", "ex", "éd", "janv", "févr",
           "avr", "juil", "sept", "oct", "nov", "déc", "apr", "s.a", "id", "ibid", "op", "cit", "tél", "vs"},
    "de": {"hr", "hrn", "fr", "frl", "dr", "prof", "dipl", "ing", "nr", "s", "z.b", "u.a", "u.ä", "usw", "bzw", "ca", "vgl",
           "evtl", "ggf", "inkl", "exkl", "zzgl", "abs", "art", "bd", "bde", "jh", "jhd", "str", "tel", "d.h", "o.ä",
           "z.t", "u.u", "s.o", "s.u", "mio", "mrd", "jan", "feb", "märz", "apr", "jun", "jul", "aug", "sep", "sept",
           "okt", "nov", "dez", "st", "geb", "gest", "verh", "bzgl", "etc", "sog", "allg", "max", "min", "hl"},
}
CAN_END = {"etc", "usw", "u.a", "inc", "ltd", "co", "corp", "a.m", "p.m", "ee.uu", "u.s", "u.k", "s.a", "s.l"}

URL = re.compile(r"(?:https?://|ftp://|www\.)[^\s<>\"'«»]+", re.IGNORECASE)
EMAIL = re.compile(r"[\w.+\-]+@[\w\-]+(?:\.[\w\-]+)+")
NUMBER = re.compile(r"[+\-−]?\d+(?:[.,  '](?=\d)\d+)*(?:[eE][+\-]?\d+)?(?:\s?[%‰])?")
APOSTROPHES = "'’ʼ"
HYPHENS = "-‐‑"
TERMINALS = ".!?…‼⁇⁈⁉"
CJK_TERMINALS = "。！？．"
OPENERS = "\"'\u00bf\u00a1\u00ab\u2039\u201c\u2018\u201e([{\u300c\u300e\u2014\u2013-"
CLOSERS = "\"'’”»›)]}」』）"


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def is_cjk(ch):
    code = ord(ch)
    return (0x3040 <= code <= 0x30FF or 0x3400 <= code <= 0x4DBF or 0x4E00 <= code <= 0x9FFF or 0xF900 <= code <= 0xFAFF
            or 0x31F0 <= code <= 0x31FF or 0xFF66 <= code <= 0xFF9F or 0x20000 <= code <= 0x3134F)


def is_wordchar(ch):
    if is_cjk(ch):
        return False
    category = unicodedata.category(ch)
    return category[0] in "LMN" or ch == "_"


def trim_url(url):
    while url and url[-1] in ".,;:!?…":
        url = url[:-1]
    while url and url[-1] in ")]}" and url.count({")": "(", "]": "[", "}": "{"}[url[-1]]) < url.count(url[-1]):
        url = url[:-1]
    return url


def scan(text):
    tokens, i, n = [], 0, len(text)
    while i < n:
        ch = text[i]
        if ch.isspace():
            i += 1
            continue
        previous = text[i - 1] if i else " "
        if not is_wordchar(previous):
            match = URL.match(text, i)
            if match:
                url = trim_url(match.group())
                if len(url) > 4:
                    tokens.append((url, "url", i, i + len(url)))
                    i += len(url)
                    continue
            match = EMAIL.match(text, i)
            if match:
                tokens.append((match.group(), "email", i, match.end()))
                i = match.end()
                continue
            if ch.isdigit() or (ch in "+-−" and i + 1 < n and text[i + 1].isdigit() and (i == 0 or previous.isspace() or previous in "([")):
                match = NUMBER.match(text, i)
                if match and not (match.end() < n and unicodedata.category(text[match.end()])[0] == "L" and _alnum_word(text, i)):
                    tokens.append((match.group(), "number", i, match.end()))
                    i = match.end()
                    continue
        if is_cjk(ch):
            j = i + 1
            while j < n and unicodedata.category(text[j]) == "Mn":
                j += 1
            tokens.append((text[i:j], "cjk", i, j))
            i = j
            continue
        if is_wordchar(ch):
            j = i + 1
            while j < n:
                if is_wordchar(text[j]):
                    j += 1
                elif text[j] in APOSTROPHES + HYPHENS and j + 1 < n and is_wordchar(text[j + 1]):
                    j += 2
                else:
                    break
            word = text[i:j]
            kind = "number" if all(c.isdigit() for c in word) else "word"
            tokens.append((word, kind, i, j))
            i = j
            continue
        j = i + 1
        if ch in ".!?-":
            while j < n and text[j] in (".!?" if ch in "!?" else ch):
                j += 1
        while j < n and (unicodedata.category(text[j]) in ("Mn", "Me", "Sk") and ord(text[j]) >= 0x300 or text[j] in "‍️︎"
                         or (text[j - 1] == "‍")):
            j += 1
        category = unicodedata.category(ch)
        kind = "symbol" if category[0] == "S" else "punctuation"
        tokens.append((text[i:j], kind, i, j))
        i = j
    return tokens


def _alnum_word(text, i):
    """True when the digits at i run straight into letters (mp3 is handled as a word, 3D too)."""
    j = i
    while j < len(text) and text[j].isdigit():
        j += 1
    return j < len(text) and unicodedata.category(text[j])[0] == "L"


def abbreviation_before(text, tokens, k, language):
    """Is the period token k the end of an abbreviation? Returns (is_abbreviation, can_end)."""
    if k == 0 or tokens[k - 1][3] != tokens[k][2]:
        return False, True
    # gather a dotted chain like e.g / z.B / EE.UU / c.-à-d immediately before the period
    start = k - 1
    while start >= 2 and tokens[start - 1][0] in (".", "-", ".-") and tokens[start - 1][3] == tokens[start][2] \
            and tokens[start - 2][3] == tokens[start - 1][2] and tokens[start - 2][1] == "word":
        start -= 2
    chain = text[tokens[start][2]:tokens[k - 1][3]].lower()
    word = tokens[k - 1]
    if word[1] == "word" and chain in ABBREVIATIONS[language]:
        return True, chain in CAN_END
    if start < k - 1 and all(tokens[m][1] == "word" and len(tokens[m][0]) <= 3 for m in range(start, k, 2)):
        return True, True  # dotted acronym such as U.N or i.e not in the list
    if word[1] == "word" and len(word[0]) == 1 and word[0].isupper():
        return True, False  # an initial: J. R. R. Tolkien
    if language == "de" and word[1] == "number" and word[0].isdigit():
        return True, False  # German ordinal: am 3. Oktober
    return False, True


def paragraph_break(text, a, b):
    return "\n\n" in text[a:b].replace("\r", "").replace(" ", "").replace("\t", "")


def sentence_spans(text, tokens, language):
    spans, start, resume = [], None, 0
    for k, (tok, kind, a, b) in enumerate(tokens):
        if k < resume:
            continue  # closing quotes or brackets already given to the previous sentence
        if start is None:
            start = a
        if k + 1 == len(tokens):
            break
        next_start = tokens[k + 1][2]
        terminal = kind == "punctuation" and (tok[0] in TERMINALS or tok[0] in CJK_TERMINALS)
        if not terminal:
            if paragraph_break(text, b, next_start):
                spans.append((start, b))
                start = None
            continue
        end, m = b, k + 1
        while m < len(tokens) and tokens[m][0][0] in CLOSERS and tokens[m][2] == end:
            end = tokens[m][3]
            m += 1
        if m == len(tokens):
            spans.append((start, end))
            start = None
            break
        gap = text[end:tokens[m][2]]
        q = m
        while q + 1 < len(tokens) and tokens[q][1] == "punctuation" and tokens[q][0][0] in OPENERS:
            q += 1
        following = tokens[q][0]
        if paragraph_break(text, end, tokens[m][2]):
            pass
        elif tok[0] in CJK_TERMINALS:
            pass
        elif not gap or following[0].islower():
            continue
        elif tok == ".":
            is_abbr, can_end = abbreviation_before(text, tokens, k, language)
            if (k >= 1 and tokens[k - 1][1] == "word" and len(tokens[k - 1][0]) == 1 and m + 1 < len(tokens)
                    and tokens[m][1] == "word" and len(tokens[m][0]) == 1 and tokens[m + 1][0] == "."):
                continue  # spaced abbreviation: z. B., d. h., u. a.
            if is_abbr and not (can_end and following[0].isupper()):
                continue
        spans.append((start, end))
        start, resume = None, m
    if start is not None:
        spans.append((start, tokens[-1][3]))
    merged = []
    for a, b in spans:
        if merged and all(c in CLOSERS or c.isspace() for c in text[a:b]):
            merged[-1] = (merged[-1][0], b)
        else:
            merged.append((a, b))
    return merged


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"text", "language", "tokens", "sentences", "top", "lowercase"}
    unknown = set(request) - allowed
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    text = request.get("text")
    if not isinstance(text, str):
        fail("text must be a string")
    if len(text) > 2_000_000:
        fail("text is longer than 2,000,000 characters")
    language = request.get("language", "en")
    if language not in ABBREVIATIONS:
        fail("language must be en, es, fr or de")
    flags = {}
    for name in ("tokens", "sentences", "lowercase"):
        value = request.get(name, True)
        if not isinstance(value, bool):
            fail(f"{name} must be a boolean")
        flags[name] = value
    top = request.get("top", 10)
    if not isinstance(top, int) or isinstance(top, bool) or top < 0:
        fail("top must be a non-negative integer")

    tokens = scan(text)
    spans = sentence_spans(text, tokens, language) if tokens else []
    words = [t[0] for t in tokens if t[1] == "word"]
    keyed = [w.casefold() if flags["lowercase"] else w for w in words]
    frequency = {}
    for w in keyed:
        frequency[w] = frequency.get(w, 0) + 1
    kinds = {}
    for t in tokens:
        kinds[t[1]] = kinds.get(t[1], 0) + 1
    paragraphs = [p for p in re.split(r"\n\s*\n", text) if p.strip()]
    counts = {
        "characters": len(text),
        "characters_no_spaces": sum(1 for c in text if not c.isspace()),
        "tokens": len(tokens),
        "words": kinds.get("word", 0),
        "numbers": kinds.get("number", 0),
        "urls": kinds.get("url", 0),
        "emails": kinds.get("email", 0),
        "cjk": kinds.get("cjk", 0),
        "punctuation": kinds.get("punctuation", 0),
        "symbols": kinds.get("symbol", 0),
        "unique_words": len(frequency),
        "sentences": len(spans),
        "paragraphs": len(paragraphs),
        "mean_word_length": round(sum(len(w) for w in words) / len(words), 2) if words else 0,
    }
    result = {"language": language, "counts": counts, "offsets": "code points, end exclusive"}
    if top:
        ranked = sorted(frequency.items(), key=lambda item: (-item[1], item[0]))[:top]
        result["top_words"] = [{"word": w, "count": c} for w, c in ranked]
    if flags["tokens"]:
        result["tokens"] = [{"text": t, "type": k, "start": a, "end": b} for t, k, a, b in tokens]
    if flags["sentences"]:
        result["sentences"] = [{"text": text[a:b], "start": a, "end": b} for a, b in spans]
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
