#!/usr/bin/env python3
"""Revised Romanization of Korean (Ministry of Culture and Tourism notice 2000-8, NIKL).

Each Hangul syllable U+AC00..U+D7A3 is decomposed arithmetically into its initial, medial and
final jamo; the sound changes of the rules (liaison, coda neutralisation, nasalisation, the
ㄹ/ㄴ assimilations, palatalisation, aspiration) are applied between the syllables of each run
of Hangul; then the jamo are written with the tables of Chapter 2. Mode "transliteration" is the
letter-by-letter spelling of Article 8.

Input (JSON on stdin):
  text            the Korean text (required)
  mode            "pronunciation" (default) | "transliteration"
  capitalize      bool, default false: upper-case the first letter of each Hangul word
  keep_h          bool, default false: keep h after ㄱ/ㄷ/ㅂ (nouns such as 묵호 Mukho)
"""
import json
import sys
import unicodedata

INITIALS = ["ㄱ", "ㄲ", "ㄴ", "ㄷ", "ㄸ", "ㄹ", "ㅁ", "ㅂ", "ㅃ", "ㅅ", "ㅆ", "ㅇ", "ㅈ", "ㅉ", "ㅊ", "ㅋ", "ㅌ", "ㅍ", "ㅎ"]
MEDIALS = ["ㅏ", "ㅐ", "ㅑ", "ㅒ", "ㅓ", "ㅔ", "ㅕ", "ㅖ", "ㅗ", "ㅘ", "ㅙ", "ㅚ", "ㅛ", "ㅜ", "ㅝ", "ㅞ", "ㅟ", "ㅠ", "ㅡ",
           "ㅢ", "ㅣ"]
FINALS = ["", "ㄱ", "ㄲ", "ㄳ", "ㄴ", "ㄵ", "ㄶ", "ㄷ", "ㄹ", "ㄺ", "ㄻ", "ㄼ", "ㄽ", "ㄾ", "ㄿ", "ㅀ", "ㅁ", "ㅂ", "ㅄ", "ㅅ",
          "ㅆ", "ㅇ", "ㅈ", "ㅊ", "ㅋ", "ㅌ", "ㅍ", "ㅎ"]

# Chapter 2, Article 1: vowels.
VOWEL = {"ㅏ": "a", "ㅐ": "ae", "ㅑ": "ya", "ㅒ": "yae", "ㅓ": "eo", "ㅔ": "e", "ㅕ": "yeo", "ㅖ": "ye", "ㅗ": "o",
         "ㅘ": "wa", "ㅙ": "wae", "ㅚ": "oe", "ㅛ": "yo", "ㅜ": "u", "ㅝ": "wo", "ㅞ": "we", "ㅟ": "wi", "ㅠ": "yu",
         "ㅡ": "eu", "ㅢ": "ui", "ㅣ": "i"}
# Chapter 2, Article 2: consonants before a vowel (ㄹ is r there; ㄹㄹ is ll, handled below).
ONSET = {"ㄱ": "g", "ㄲ": "kk", "ㄴ": "n", "ㄷ": "d", "ㄸ": "tt", "ㄹ": "r", "ㅁ": "m", "ㅂ": "b", "ㅃ": "pp",
         "ㅅ": "s", "ㅆ": "ss", "ㅇ": "", "ㅈ": "j", "ㅉ": "jj", "ㅊ": "ch", "ㅋ": "k", "ㅌ": "t", "ㅍ": "p", "ㅎ": "h"}
# Codas after neutralisation (Standard Pronunciation, articles 9-11): ㄼ is l (여덟 yeodeol).
NEUTRAL = {"": "", "ㄱ": "k", "ㄲ": "k", "ㅋ": "k", "ㄳ": "k", "ㄺ": "k",
           "ㄴ": "n", "ㄵ": "n", "ㄶ": "n",
           "ㄷ": "t", "ㅅ": "t", "ㅆ": "t", "ㅈ": "t", "ㅊ": "t", "ㅌ": "t", "ㅎ": "t",
           "ㄹ": "l", "ㄼ": "l", "ㄽ": "l", "ㄾ": "l", "ㅀ": "l",
           "ㅁ": "m", "ㄻ": "m",
           "ㅂ": "p", "ㅍ": "p", "ㅄ": "p", "ㄿ": "p",
           "ㅇ": "ng"}
SPLIT = {"ㄳ": ("ㄱ", "ㅅ"), "ㄵ": ("ㄴ", "ㅈ"), "ㄶ": ("ㄴ", "ㅎ"), "ㄺ": ("ㄹ", "ㄱ"), "ㄻ": ("ㄹ", "ㅁ"),
         "ㄼ": ("ㄹ", "ㅂ"), "ㄽ": ("ㄹ", "ㅅ"), "ㄾ": ("ㄹ", "ㅌ"), "ㄿ": ("ㄹ", "ㅍ"), "ㅀ": ("ㄹ", "ㅎ"),
         "ㅄ": ("ㅂ", "ㅅ")}
ASPIRATE = {"ㄱ": "ㅋ", "ㄷ": "ㅌ", "ㅈ": "ㅊ"}
NASAL = {"k": "ng", "t": "n", "p": "m"}

# Article 8 (transliteration): every letter by its value, ㄱ ㄷ ㅂ ㄹ always g d b l.
TR_INITIAL = dict(ONSET, **{"ㄹ": "l"})
TR_FINAL = {"": "", "ㄱ": "g", "ㄲ": "kk", "ㄳ": "gs", "ㄴ": "n", "ㄵ": "nj", "ㄶ": "nh", "ㄷ": "d", "ㄹ": "l",
            "ㄺ": "lg", "ㄻ": "lm", "ㄼ": "lb", "ㄽ": "ls", "ㄾ": "lt", "ㄿ": "lp", "ㅀ": "lh", "ㅁ": "m", "ㅂ": "b",
            "ㅄ": "bs", "ㅅ": "s", "ㅆ": "ss", "ㅇ": "ng", "ㅈ": "j", "ㅊ": "ch", "ㅋ": "k", "ㅌ": "t", "ㅍ": "p",
            "ㅎ": "h"}
TR_FINAL_SET = set(TR_FINAL.values())
TR_ONSET_SET = {v for v in TR_INITIAL.values() if v}

MAX_TEXT = 100000


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def decompose(char):
    index = ord(char) - 0xAC00
    return INITIALS[index // 588], MEDIALS[(index % 588) // 28], FINALS[index % 28]


def boundary(final, initial, next_vowel, keep_h):
    """Sound changes between a syllable's final and the next initial.
    Returns (romanized coda, onset jamo)."""
    if final == "":
        return "", initial
    if initial == "ㅇ" and final != "ㅇ":
        # Liaison (Standard Pronunciation art. 13-14); ㅎ is silent before a vowel (art. 12.4).
        if final == "ㅎ":
            return "", "ㅇ"
        if final in ("ㄶ", "ㅀ"):
            final = SPLIT[final][0]
            return "", final
        if final in SPLIT:
            first, moved = SPLIT[final]
        else:
            first, moved = "", final
        if next_vowel == "ㅣ" and moved in ("ㄷ", "ㅌ"):  # palatalisation (art. 17)
            moved = "ㅈ" if moved == "ㄷ" else "ㅊ"
        return NEUTRAL[first], moved
    if initial == "ㅎ":
        if NEUTRAL[final] in ("k", "t", "p") and final not in ("ㅎ",):
            if keep_h and final in ("ㄱ", "ㄷ", "ㅂ", "ㄳ", "ㄺ", "ㅄ"):
                return NEUTRAL[final], "ㅎ"
            first = SPLIT[final][0] if final in ("ㄺ", "ㄼ", "ㄿ") else ""
            if NEUTRAL[final] == "k":
                onset = "ㅋ"
            elif NEUTRAL[final] == "p":
                onset = "ㅍ"
            elif final in ("ㅈ", "ㅊ"):
                onset = "ㅊ"
            else:
                onset = "ㅌ"
            if onset == "ㅌ" and next_vowel == "ㅣ" and final in ("ㄷ", "ㅌ"):  # 굳히다 guchida
                onset = "ㅊ"
            return NEUTRAL[first], onset
        if final in ("ㅎ", "ㄶ", "ㅀ"):
            rest = {"ㅎ": "", "ㄶ": "ㄴ", "ㅀ": "ㄹ"}[final]
            return NEUTRAL[rest], "ㅎ"
        return NEUTRAL[final], "ㅎ"
    if final in ("ㅎ", "ㄶ", "ㅀ"):
        rest = {"ㅎ": "", "ㄶ": "ㄴ", "ㅀ": "ㄹ"}[final]
        if initial in ASPIRATE:  # 좋고 joko, 놓다 nota, 않고 anko, 싫다 silta
            return NEUTRAL[rest], ASPIRATE[initial]
        if initial == "ㅅ":
            return NEUTRAL[rest], initial
        if initial == "ㄴ":  # 놓는 nonneun, 않는 anneun, 끓는 kkeulleun
            final = "ㄴ" if final == "ㅎ" else rest
    coda = NEUTRAL[final]
    onset = initial
    if onset == "ㄹ":
        if coda in ("k", "t", "p", "m", "ng"):  # 종로 Jongno, 왕십리 Wangsimni, 백리 baengni
            onset = "ㄴ"
        elif coda == "n":  # 신라 Silla, 대관령 Daegwallyeong
            coda = "l"
    elif onset == "ㄴ" and coda == "l":  # 별내 Byeollae
        onset = "ㄹ"
    if onset in ("ㄴ", "ㅁ") and coda in NASAL:  # 백마 Baengma, 왕십리 Wangsimni
        coda = NASAL[coda]
    return coda, onset


def romanize_word(syllables, keep_h):
    parts = []
    count = len(syllables)
    onsets = [s[0] for s in syllables]
    codas = [NEUTRAL[s[2]] for s in syllables]
    for i in range(count - 1):
        coda, onset = boundary(syllables[i][2], syllables[i + 1][0], syllables[i + 1][1], keep_h)
        codas[i] = coda
        onsets[i + 1] = onset
    for i in range(count):
        onset = onsets[i]
        if onset == "ㄹ" and i > 0 and codas[i - 1] == "l":
            text = "l"
        else:
            text = ONSET[onset]
        parts.append(text + VOWEL[syllables[i][1]] + codas[i])
    return "".join(parts)


def transliterate_word(syllables):
    out = []
    for i, (initial, medial, final) in enumerate(syllables):
        onset = TR_INITIAL[initial]
        if i > 0:
            previous = TR_FINAL[syllables[i - 1][2]]
            if initial == "ㅇ":
                out.append("-")
            elif previous and ambiguous(previous, onset):
                out.append("-")
        out.append(onset + VOWEL[medial] + TR_FINAL[final])
    return "".join(out)


def ambiguous(coda, onset):
    """True when coda+onset can be split into another valid (final, initial) pair."""
    cluster = coda + onset
    for cut in range(len(cluster)):
        other_coda, other_onset = cluster[:cut], cluster[cut:]
        if (other_coda, other_onset) != (coda, onset) and other_coda in TR_FINAL_SET and other_onset in TR_ONSET_SET:
            return True
    return False


def main():
    try:
        request = json.loads(sys.stdin.buffer.read().decode("utf-8"))
    except (ValueError, UnicodeDecodeError) as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    allowed = {"text", "mode", "capitalize", "keep_h"}
    unknown = sorted(set(request) - allowed)
    if unknown:
        fail(f"unknown fields: {', '.join(unknown)}")
    text = request.get("text")
    if not isinstance(text, str) or not text:
        fail("text must be a non-empty string")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    mode = request.get("mode", "pronunciation")
    if mode not in ("pronunciation", "transliteration"):
        fail("mode must be pronunciation or transliteration")
    capitalize = request.get("capitalize", False)
    keep_h = request.get("keep_h", False)
    if not isinstance(capitalize, bool) or not isinstance(keep_h, bool):
        fail("capitalize and keep_h must be booleans")

    text = unicodedata.normalize("NFC", text)
    out = []
    unconverted = []
    word = []

    def flush():
        if word:
            roman = transliterate_word(word) if mode == "transliteration" else romanize_word(word, keep_h)
            if capitalize:
                roman = roman[0].upper() + roman[1:]
            out.append(roman)
            word.clear()

    syllable_count = 0
    for char in text:
        if 0xAC00 <= ord(char) <= 0xD7A3:
            word.append(decompose(char))
            syllable_count += 1
            continue
        flush()
        if ord(char) > 0x7F and not char.isspace() and char not in unconverted and len(unconverted) < 50:
            unconverted.append(char)
        out.append(char)
    flush()
    result = {"result": "".join(out), "mode": mode, "system": "Revised Romanization of Korean (2000)",
              "syllables": syllable_count}
    if unconverted:
        result["unconverted"] = unconverted
    if mode == "pronunciation":
        result["note"] = ("sound changes are applied within each run of Hangul; changes that depend on "
                          "morphology (n-insertion, tensing, 신문로 Sinmunno-type ㄴㄴ) are not detected")
    sys.stdout.buffer.write((json.dumps(result, ensure_ascii=False) + "\n").encode("utf-8"))


if __name__ == "__main__":
    main()
