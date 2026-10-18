#!/usr/bin/env python3
"""Readability formulas. English: Flesch reading ease, Flesch-Kincaid grade, Gunning fog,
SMOG, Coleman-Liau, ARI. Spanish: Fernández-Huerta and Szigriszt-Pazos (INFLESZ scale).
Spanish syllables follow the orthographic rules (strong and weak vowels, accents, hiatus);
English syllables are a heuristic count of vowel groups.

Input (JSON on stdin):
  text      the text
  language  "en" | "es"   (default "en")
"""
import json
import math
import re
import sys
import unicodedata

WORD = re.compile(r"[^\W\d_]+(?:['’-][^\W\d_]+)*", re.UNICODE)
TERMINATOR = re.compile(r"[.!?…]+(?=[\s\"'”»)\]]|$)|\n{2,}")
ABBREVIATIONS = {"mr", "mrs", "ms", "dr", "prof", "sr", "sra", "srta", "dra", "st", "vs", "etc", "eg", "ie", "ud", "uds", "no", "núm", "pág", "fig", "approx", "cf"}
STRONG_ES = set("aeoáéóíú")
VOWELS_ES = set("aeiouáéíóúü")


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def syllables_es(word):
    w = word.lower()
    if w == "y":
        return 1
    w = re.sub(r"y$", "i", w)
    w = re.sub(r"(?<=[qg])u(?=[eiéí])", "", w)
    count, prev = 0, None
    for ch in w:
        if ch in VOWELS_ES:
            if prev is None or prev not in VOWELS_ES:
                count += 1
            elif ch in STRONG_ES and prev in STRONG_ES:
                count += 1
        prev = ch
    return max(count, 1)


def syllables_en(word):
    w = word.lower().replace("’", "'")
    w = re.sub(r"'s$", "", w)
    if len(w) <= 3:
        return 1
    w = re.sub(r"(?<=[^aeiouy])e$", "", w) if not re.search(r"[^aeiouy]le$", w) else w
    w = re.sub(r"(?<=[^aeiouytd])ed$", "d", w)
    if not re.search(r"(?:[sxzcg]|[cs]h)es$", w):
        w = re.sub(r"(?<=[^aeiouy])es$", "s", w)
    w = re.sub(r"^y", "", w)
    count = len(re.findall(r"[aeiouy]+", w))
    count += len(re.findall(r"(?<![ts])i[aou]|eo|u[ao](?!r)", w))
    return max(count, 1)


def split_sentences(text):
    """Sentences end at . ! ? … before a space or the end, or at a blank line; a period after
    a single capital (U.S.) or a common abbreviation (Dr., etc.) does not end one."""
    sentences, start = [], 0
    for m in TERMINATOR.finditer(text):
        before = re.search(r"([^\W\d_]+)\.*$", text[start:m.start()]) if m.group(0) == "." else None
        token = before.group(1) if before else ""
        if m.group(0) == "." and ((len(token) == 1 and token.isupper()) or token.lower().replace(".", "") in ABBREVIATIONS):
            continue
        sentences.append(text[start:m.end()])
        start = m.end()
    sentences.append(text[start:])
    return [s for s in sentences if WORD.search(s)]


def stats(text, language):
    sentences = split_sentences(text)
    words = WORD.findall(text)
    if not words:
        fail("text has no words")
    count = syllables_es if language == "es" else syllables_en
    syl = [count(w) for w in words]
    letters = sum(len(w.replace("'", "").replace("’", "").replace("-", "")) for w in words)
    n_sent = max(len(sentences), 1)
    return {
        "sentences": n_sent, "words": len(words), "syllables": sum(syl), "letters": letters,
        "polysyllables": sum(1 for s in syl if s >= 3),
        "complex_words": sum(1 for w, s in zip(words, syl) if s >= 3 and not w[0].isupper() and not re.search(r"(es|ed|ing)$", w.lower())),
    }


def r(x):
    return round(x, 2)


def english(s):
    wps, spw = s["words"] / s["sentences"], s["syllables"] / s["words"]
    fre = 206.835 - 1.015 * wps - 84.6 * spw
    bands = [(90, "very easy"), (80, "easy"), (70, "fairly easy"), (60, "standard"), (50, "fairly difficult"), (30, "difficult"), (-1e9, "very difficult")]
    return {
        "flesch_reading_ease": r(fre),
        "flesch_band": next(label for floor, label in bands if fre >= floor),
        "flesch_kincaid_grade": r(0.39 * wps + 11.8 * spw - 15.59),
        "gunning_fog": r(0.4 * (wps + 100 * s["complex_words"] / s["words"])),
        "smog": r(1.0430 * math.sqrt(s["polysyllables"] * 30 / s["sentences"]) + 3.1291),
        "coleman_liau": r(0.0588 * (100 * s["letters"] / s["words"]) - 0.296 * (100 * s["sentences"] / s["words"]) - 15.8),
        "automated_readability_index": r(4.71 * s["letters"] / s["words"] + 0.5 * wps - 21.43),
    }


def spanish(s):
    wps, spw = s["words"] / s["sentences"], s["syllables"] / s["words"]
    fh = 206.84 - 60 * spw - 1.02 * wps
    sp = 206.835 - 62.3 * spw - wps
    inflesz = [(80, "muy fácil"), (65, "bastante fácil"), (55, "normal"), (40, "algo difícil"), (-1e9, "muy difícil")]
    return {
        "fernandez_huerta": r(fh),
        "szigriszt_pazos": r(sp),
        "inflesz_band": next(label for floor, label in inflesz if sp > floor or floor < 0),
    }


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    text, language = request.get("text"), request.get("language", "en")
    if not isinstance(text, str) or not text.strip():
        fail("text must be a non-empty string")
    if language not in ("en", "es"):
        fail("language must be en or es")
    text = unicodedata.normalize("NFC", text)
    s = stats(text, language)
    s["words_per_sentence"] = r(s["words"] / s["sentences"])
    s["syllables_per_word"] = r(s["syllables"] / s["words"])
    out = {"language": language, "counts": s, "scores": english(s) if language == "en" else spanish(s)}
    print(json.dumps(out, ensure_ascii=False))


if __name__ == "__main__":
    main()
