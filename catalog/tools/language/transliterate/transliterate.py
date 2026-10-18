#!/usr/bin/env python3
"""Table-based transliteration of Cyrillic, Greek and Arabic script into Latin letters.

Cyrillic: ISO 9:1995 (all Slavic Cyrillic letters, one Latin letter per Cyrillic letter),
scholarly (scientific) transliteration of Russian, or BGN/PCGN 1947 for Russian.
Greek: ELOT 743 / ISO 843 transcription (modern pronunciation, accents dropped).
Arabic: a simplified ALA-LC subset (consonants and long vowels; short vowels only when the
text carries the vowel marks).

Each character is handled by its own script, so mixed text works; anything else is copied.

Input (JSON on stdin):
  text              the text (required)
  cyrillic_system   "iso9" (default) | "scholarly" | "bgn_pcgn"
  cyrillic_signs    "standard" (default) | "ascii" (' and ") | "omit": soft and hard signs
  greek_diaeresis   "keep" (default: ï, ÿ) | "drop" (i, y)
"""
import json
import sys
import unicodedata

MAX_TEXT = 100000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


# ---------------------------------------------------------------- Cyrillic
ISO9 = {
    "а": "a", "б": "b", "в": "v", "г": "g", "ґ": "g̀", "д": "d", "ѓ": "ǵ", "ђ": "đ", "е": "e", "ё": "ë",
    "є": "ê", "ж": "ž", "з": "z", "ѕ": "ẑ", "и": "i", "і": "ì", "ї": "ï", "й": "j", "ј": "ǰ", "к": "k",
    "л": "l", "љ": "l̂", "м": "m", "н": "n", "њ": "n̂", "о": "o", "п": "p", "р": "r", "с": "s",
    "т": "t", "ћ": "ć", "ќ": "ḱ", "у": "u", "ў": "ŭ", "ф": "f", "х": "h", "ц": "c", "ч": "č",
    "џ": "d̂", "ш": "š", "щ": "ŝ", "ъ": "ʺ", "ы": "y", "ь": "ʹ", "э": "è", "ю": "û", "я": "â",
    "ѣ": "ě", "ѳ": "f̀", "ѵ": "ỳ", "ѫ": "ǎ",
}
SCHOLARLY = {
    "а": "a", "б": "b", "в": "v", "г": "g", "д": "d", "е": "e", "ё": "ë", "ж": "ž", "з": "z", "и": "i",
    "й": "j", "к": "k", "л": "l", "м": "m", "н": "n", "о": "o", "п": "p", "р": "r", "с": "s", "т": "t",
    "у": "u", "ф": "f", "х": "x", "ц": "c", "ч": "č", "ш": "š", "щ": "šč", "ъ": "ʺ", "ы": "y",
    "ь": "ʹ", "э": "è", "ю": "ju", "я": "ja",
}
BGN = {
    "а": "a", "б": "b", "в": "v", "г": "g", "д": "d", "е": "e", "ё": "ë", "ж": "zh", "з": "z", "и": "i",
    "й": "y", "к": "k", "л": "l", "м": "m", "н": "n", "о": "o", "п": "p", "р": "r", "с": "s", "т": "t",
    "у": "u", "ф": "f", "х": "kh", "ц": "ts", "ч": "ch", "ш": "sh", "щ": "shch", "ъ": "”", "ы": "y",
    "ь": "’", "э": "e", "ю": "yu", "я": "ya",
}
CYRILLIC_SYSTEMS = {"iso9": (ISO9, "ISO 9:1995"), "scholarly": (SCHOLARLY, "scholarly transliteration (Russian)"),
                    "bgn_pcgn": (BGN, "BGN/PCGN 1947 (Russian)")}
BGN_YE_AFTER = set("аеёиоуыэюяйъь")
SOFT_HARD = {"ʹ": "'", "ʺ": "\"", "’": "'", "”": "\""}


def is_cyrillic(char):
    return "Ѐ" <= char <= "ӿ"


# ---------------------------------------------------------------- Greek
GREEK = {"α": "a", "β": "v", "γ": "g", "δ": "d", "ε": "e", "ζ": "z", "η": "i", "θ": "th", "ι": "i", "κ": "k",
         "λ": "l", "μ": "m", "ν": "n", "ξ": "x", "ο": "o", "π": "p", "ρ": "r", "σ": "s", "ς": "s", "τ": "t",
         "υ": "y", "φ": "f", "χ": "ch", "ψ": "ps", "ω": "o", "ϲ": "s"}
GREEK_VOWELS = set("αεηιουω")
GREEK_VOICED = set("βγδζλμνρ")
GREEK_ACCENTS = {"́", "̀", "͂", "́", "̀"}
GREEK_PUNCT = {";": "?", "·": ";"}


def greek_record(char):
    """(base lowercase letter, is upper, has diaeresis, has accent) or None."""
    parts = unicodedata.normalize("NFD", char)
    base = parts[0]
    lower = base.lower()
    if lower not in GREEK:
        return None
    marks = parts[1:]
    return lower, base != lower, "̈" in marks, any(m in GREEK_ACCENTS for m in marks)


# ---------------------------------------------------------------- Arabic
ARABIC = {
    "ء": "ʼ", "أ": "ʼ", "إ": "ʼ", "ؤ": "ʼ", "ئ": "ʼ", "ب": "b", "ت": "t", "ث": "th",
    "ج": "j", "ح": "ḥ", "خ": "kh", "د": "d", "ذ": "dh", "ر": "r", "ز": "z", "س": "s", "ش": "sh", "ص": "ṣ",
    "ض": "ḍ", "ط": "ṭ", "ظ": "ẓ", "ع": "ʻ", "غ": "gh", "ف": "f", "ق": "q", "ك": "k", "ل": "l", "م": "m",
    "ن": "n", "ه": "h", "و": "w", "ي": "y", "ى": "á", "ة": "h", "ا": "ā", "ٱ": "", "آ": "ʼā",
    # Persian and Urdu letters of the ALA-LC Persian table
    "پ": "p", "چ": "ch", "ژ": "zh", "گ": "g", "ک": "k", "ی": "y", "ڤ": "v",
}
HARAKAT = {"َ": "a", "ُ": "u", "ِ": "i", "ً": "an", "ٌ": "un", "ٍ": "in",
           "ْ": "", "ٰ": "ā", "ٔ": "ʼ", "ٕ": "ʼ", "ٓ": "", "ـ": ""}
SHADDA = "ّ"
ARABIC_PUNCT = {"،": ",", "؛": ";", "؟": "?", "٪": "%", "٫": ".", "٬": ","}
VOWEL_MARKS = {"َ", "ُ", "ِ", "ً", "ٌ", "ٍ"}


def is_arabic(char):
    return char in ARABIC or char in HARAKAT or char == SHADDA


def arabic_digit(char):
    if "٠" <= char <= "٩":
        return str(ord(char) - 0x0660)
    if "۰" <= char <= "۹":
        return str(ord(char) - 0x06F0)
    return None


# ---------------------------------------------------------------- case

def apply_case(output, upper, text, start, end, is_letter):
    """Upper-case output for an upper-case source: all capitals inside an all-capitals word,
    otherwise title case (Щ -> Shch, ЩИ -> SHCHI)."""
    if not upper or not output:
        return output
    after = text[end] if end < len(text) else ""
    before = text[start - 1] if start > 0 else ""
    if after and is_letter(after):
        caps = after.isupper()
    elif before and is_letter(before):
        caps = before.isupper()
    else:
        caps = False
    if caps:
        return output.upper()
    return output[0].upper() + output[1:]


# ---------------------------------------------------------------- converters

def convert_cyrillic(text, i, system, signs, unmapped):
    table = CYRILLIC_SYSTEMS[system][0]
    char = text[i]
    lower = char.lower()
    if lower not in table:
        if char not in unmapped and len(unmapped) < 50:
            unmapped.append(char)
        return char
    out = table[lower]
    if system == "bgn_pcgn" and lower in ("е", "ё"):
        before = text[i - 1].lower() if i > 0 else ""
        if not is_cyrillic(before) or before in BGN_YE_AFTER:
            out = "y" + out
    if lower in ("ъ", "ь"):
        if signs == "ascii":
            out = SOFT_HARD.get(out, out)
        elif signs == "omit":
            out = ""
    return apply_case(out, char != lower, text, i, i + 1, is_cyrillic)


def convert_greek_run(text, start, end, diaeresis):
    """Transcribe the Greek letters text[start:end] (one word) per ELOT 743."""
    records = [greek_record(c) for c in text[start:end]]
    out = []
    j = 0
    count = len(records)

    def is_letter(c):
        return greek_record(c) is not None

    while j < count:
        base, upper, dia, accent = records[j]
        nxt = records[j + 1] if j + 1 < count else None
        span = 1
        if base in ("α", "ε", "η") and nxt and nxt[0] == "υ" and not nxt[2] and not accent:
            after = records[j + 2][0] if j + 2 < count else None
            voiced = after is not None and (after in GREEK_VOWELS or after in GREEK_VOICED)
            piece = GREEK[base] + ("v" if voiced else "f")
            span = 2
        elif base == "ο" and nxt and nxt[0] == "υ" and not nxt[2] and not accent:
            piece, span = "ou", 2
        elif base == "μ" and nxt and nxt[0] == "π":
            piece = "b" if j == 0 or j + 2 == count else "mp"
            span = 2
        elif base == "γ" and nxt and nxt[0] in ("γ", "ξ", "χ"):
            piece = {"γ": "ng", "ξ": "nx", "χ": "nch"}[nxt[0]]
            span = 2
        else:
            piece = GREEK[base]
            if dia and diaeresis == "keep":
                piece = {"i": "ï", "y": "ÿ"}.get(piece, piece)
        out.append(apply_case(piece, upper, text, start + j, start + j + span, is_letter))
        j += span
    return "".join(out)


def convert_arabic_run(text, start, end, notes):
    """Romanize one Arabic word text[start:end]."""
    word = text[start:end]
    out = []
    i = 0
    word_start = 0  # index of the first letter after the article
    if word[:2] in ("ال", "ٱل") and sum(c in ARABIC for c in word) > 2:
        out.append("al-")  # ALA-LC: the article is al- before sun and moon letters alike
        i = 2
        while i < len(word) and word[i] in HARAKAT:  # sukun on the lam
            i += 1
        word_start = i
    last_vowel = ""  # short vowel just written, joined with a following long-vowel letter
    last_consonant = ""
    consonant_at = -1
    first_after_article = False
    while i < len(word):
        char = word[i]
        following = word[i + 1] if i + 1 < len(word) else ""
        if char == SHADDA:
            # doubles the consonant (written before its vowel mark, which NFC may put first);
            # the sun letter after al- is not doubled (al-shams)
            if last_consonant and not first_after_article:
                out.insert(consonant_at + 1, last_consonant)
                last_consonant = ""
            i += 1
            continue
        if char in HARAKAT:
            piece = HARAKAT[char]
            if char == "\u064b" and following in ("ا", "ى"):
                i += 1  # the alif after tanwin fath is silent
            out.append(piece)
            last_vowel = piece if piece in ("a", "i", "u") else ""
            i += 1
            continue
        unvoweled = following not in VOWEL_MARKS and following != SHADDA
        if i == word_start and char in ("ا", "ٱ", "أ", "ء"):
            piece = ""  # initial hamza is not romanized; its vowel comes from a vowel mark, if any
        elif i == word_start and char == "إ":
            piece = "i" if following != "\u0650" else ""
        elif i == word_start and char == "آ":
            piece = "ā"
        elif char in ("ا", "ى") and last_vowel == "a":
            out[-1] = ""
            piece = "ā" if char == "ا" else "á"
        elif char == "ي" and last_vowel == "i" and unvoweled:
            out[-1] = ""
            piece = "ī"
        elif char == "و" and last_vowel == "u" and unvoweled:
            out[-1] = ""
            piece = "ū"
        elif char == "ة":
            piece = "t" if following in VOWEL_MARKS else "h"
        else:
            piece = ARABIC[char]
        out.append(piece)
        last_consonant = piece if piece and piece not in ("ā", "ī", "ū", "á") else ""
        consonant_at = len(out) - 1
        first_after_article = word_start > 0 and i == word_start
        last_vowel = ""
        i += 1
    if not any(c in VOWEL_MARKS for c in word):
        notes.add("Arabic short vowels are not written in the text and are not restored")
    result = "".join(out)
    if result.endswith("iyy"):  # ALA-LC: final -iyy is written ī (ʻAlī)
        result = result[:-3] + "ī"
    return result


# ---------------------------------------------------------------- main

def main():
    try:
        request = json.loads(sys.stdin.buffer.read().decode("utf-8"))
    except (ValueError, UnicodeDecodeError) as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    unknown = sorted(set(request) - {"text", "cyrillic_system", "cyrillic_signs", "greek_diaeresis"})
    if unknown:
        fail(f"unknown fields: {', '.join(unknown)}")
    text = request.get("text")
    if not isinstance(text, str) or not text:
        fail("text must be a non-empty string")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    system = request.get("cyrillic_system", "iso9")
    if system not in CYRILLIC_SYSTEMS:
        fail("cyrillic_system must be iso9, scholarly or bgn_pcgn")
    signs = request.get("cyrillic_signs", "standard")
    if signs not in ("standard", "ascii", "omit"):
        fail("cyrillic_signs must be standard, ascii or omit")
    diaeresis = request.get("greek_diaeresis", "keep")
    if diaeresis not in ("keep", "drop"):
        fail("greek_diaeresis must be keep or drop")

    # Arabic presentation forms (lam-alef ligatures and the like) to their letters.
    text = "".join(unicodedata.normalize("NFKC", c) if "ﭐ" <= c <= "﷿" or "ﹰ" <= c <= "﻿"
                   else c for c in text)
    text = unicodedata.normalize("NFC", text)
    out, unmapped, notes, scripts = [], [], set(), []

    def seen(name):
        if name not in scripts:
            scripts.append(name)

    i = 0
    while i < len(text):
        char = text[i]
        if is_cyrillic(char):
            seen("cyrillic")
            out.append(convert_cyrillic(text, i, system, signs, unmapped))
            i += 1
        elif greek_record(char) is not None:
            seen("greek")
            end = i
            while end < len(text) and (greek_record(text[end]) is not None
                                       or unicodedata.combining(text[end])):
                end += 1
            letters = "".join(c for c in text[i:end] if not unicodedata.combining(c))
            out.append(convert_greek_run(letters, 0, len(letters), diaeresis))
            i = end
        elif is_arabic(char):
            seen("arabic")
            end = i
            while end < len(text) and is_arabic(text[end]):
                end += 1
            out.append(convert_arabic_run(text, i, end, notes))
            i = end
        elif char in GREEK_PUNCT:
            out.append(GREEK_PUNCT[char])
            i += 1
        elif char in ARABIC_PUNCT or arabic_digit(char) is not None:
            out.append(ARABIC_PUNCT.get(char) or arabic_digit(char))
            i += 1
        else:
            if unicodedata.category(char).startswith("L") and ord(char) > 0x24F and char not in unmapped \
                    and len(unmapped) < 50:
                unmapped.append(char)
            out.append(char)
            i += 1
    names = {"cyrillic": CYRILLIC_SYSTEMS[system][1], "greek": "ELOT 743 / ISO 843 transcription",
             "arabic": "ALA-LC (simplified)"}
    systems = {name: names[name] for name in scripts}
    if unmapped:
        notes.add("letters outside the selected tables are copied unchanged and listed in unmapped")
    result = {"result": "".join(out), "scripts": scripts, "systems": systems}
    if unmapped:
        result["unmapped"] = unmapped
    if notes:
        result["notes"] = sorted(notes)
    sys.stdout.buffer.write((json.dumps(result, ensure_ascii=False) + "\n").encode("utf-8"))


if __name__ == "__main__":
    main()
