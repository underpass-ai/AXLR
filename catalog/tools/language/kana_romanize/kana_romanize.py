#!/usr/bin/env python3
"""Romanize Japanese hiragana and katakana: modified (revised) Hepburn or Kunrei-shiki (ISO 3602).

Kana are read mora by mora (yoon and katakana extensions before single kana), then sokuon,
syllabic n and long vowels are resolved. Kanji and other characters are copied unchanged and
listed in "unconverted": reading kanji needs a dictionary.

Input (JSON on stdin):
  text         the Japanese text (required)
  system       "hepburn" (default) | "kunrei"
  long_vowels  "mark" (default: macron in Hepburn, circumflex in Kunrei) | "as_is"
  n_before_bmp "n" (default, modern Hepburn) | "m" (traditional Hepburn: shimbun)
  capitalize   bool, default false: upper-case the first letter of each space-separated word
"""
import json
import re
import sys
import unicodedata

MAX_TEXT = 100000

# (Hepburn, Kunrei) for single kana.
SINGLE = {
    "あ": "a", "い": "i", "う": "u", "え": "e", "お": "o",
    "か": "ka", "き": "ki", "く": "ku", "け": "ke", "こ": "ko",
    "が": "ga", "ぎ": "gi", "ぐ": "gu", "げ": "ge", "ご": "go",
    "さ": "sa", "し": ("shi", "si"), "す": "su", "せ": "se", "そ": "so",
    "ざ": "za", "じ": ("ji", "zi"), "ず": "zu", "ぜ": "ze", "ぞ": "zo",
    "た": "ta", "ち": ("chi", "ti"), "つ": ("tsu", "tu"), "て": "te", "と": "to",
    "だ": "da", "ぢ": ("ji", "zi"), "づ": "zu", "で": "de", "ど": "do",
    "な": "na", "に": "ni", "ぬ": "nu", "ね": "ne", "の": "no",
    "は": "ha", "ひ": "hi", "ふ": ("fu", "hu"), "へ": "he", "ほ": "ho",
    "ば": "ba", "び": "bi", "ぶ": "bu", "べ": "be", "ぼ": "bo",
    "ぱ": "pa", "ぴ": "pi", "ぷ": "pu", "ぺ": "pe", "ぽ": "po",
    "ま": "ma", "み": "mi", "む": "mu", "め": "me", "も": "mo",
    "や": "ya", "ゆ": "yu", "よ": "yo",
    "ら": "ra", "り": "ri", "る": "ru", "れ": "re", "ろ": "ro",
    "わ": "wa", "ゐ": "i", "ゑ": "e", "を": "o",
    "ゔ": "vu",
    "ぁ": "a", "ぃ": "i", "ぅ": "u", "ぇ": "e", "ぉ": "o", "ゃ": "ya", "ゅ": "yu", "ょ": "yo", "ゎ": "wa",
    "ゕ": "ka", "ゖ": "ke",
    "ヷ": "va", "ヸ": "vi", "ヹ": "ve", "ヺ": "vo",  # ヷ ヸ ヹ ヺ
}
BARE_VOWELS = {"あ": "a", "い": "i", "う": "u", "え": "e", "お": "o"}

# Yoon: i-row kana + small ya/yu/yo.
YOON_BASE = {"き": ("ky", "ky"), "ぎ": ("gy", "gy"), "し": ("sh", "sy"), "じ": ("j", "zy"), "ち": ("ch", "ty"),
             "ぢ": ("j", "zy"), "に": ("ny", "ny"), "ひ": ("hy", "hy"), "び": ("by", "by"), "ぴ": ("py", "py"),
             "み": ("my", "my"), "り": ("ry", "ry")}
COMBO = {}
for base, (hep, kun) in YOON_BASE.items():
    for small, vowel in (("ゃ", "a"), ("ゅ", "u"), ("ょ", "o")):
        COMBO[base + small] = (hep + vowel, kun + vowel, False)
# Extensions for loanwords (katakana, also accepted in hiragana); the flag marks spellings
# outside ISO 3602, which are written in Hepburn in both systems.
EXTENDED = {
    "しぇ": ("she", "sye"), "じぇ": ("je", "zye"), "ちぇ": ("che", "tye"),
    "ふぁ": "fa", "ふぃ": "fi", "ふぇ": "fe", "ふぉ": "fo", "ふゅ": "fyu",
    "てぃ": "ti", "でぃ": "di", "とぅ": "tu", "どぅ": "du", "てゅ": "tyu", "でゅ": "dyu",
    "うぃ": "wi", "うぇ": "we", "うぉ": "wo",
    "ゔぁ": "va", "ゔぃ": "vi", "ゔぇ": "ve", "ゔぉ": "vo", "ゔゅ": "vyu",
    "つぁ": "tsa", "つぃ": "tsi", "つぇ": "tse", "つぉ": "tso",
    "いぇ": "ye", "くぁ": "kwa", "くぃ": "kwi", "くぇ": "kwe", "くぉ": "kwo", "ぐぁ": "gwa",
    "すぃ": "si", "ずぃ": "zi",
}
for key, value in EXTENDED.items():
    hep, kun = value if isinstance(value, tuple) else (value, value)
    COMBO[key] = (hep, kun, not isinstance(value, tuple))

VOICED = dict(zip("かきくけこさしすせそたちつてとはひふへほう", "がぎぐげござじずぜぞだぢづでどばびぶべぼゔ"))
PUNCT = {"。": ".", "、": ",", "「": "\"", "」": "\"", "『": "\"", "』": "\"", "・": " ", "〜": "~", "～": "~",
         "　": " ", "（": "(", "）": ")"}
MARK = {"hepburn": dict(zip("aeiou", "āēīōū")), "kunrei": dict(zip("aeiou", "âêîôû"))}


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def to_hiragana(char):
    code = ord(char)
    if 0x30A1 <= code <= 0x30F6:
        return chr(code - 0x60)
    if char in ("ヽ", "ヾ"):  # ヽ ヾ
        return chr(code - 0x60)
    return char


def single(kana, system):
    value = SINGLE[kana]
    if isinstance(value, tuple):
        return value[0] if system == "hepburn" else value[1]
    return value


def tokenize(text, system):
    """List of tokens: dicts with kind mora|n|sokuon|long|other."""
    tokens = []
    chars = [to_hiragana(c) for c in text]
    i = 0
    while i < len(chars):
        char = chars[i]
        pair = char + chars[i + 1] if i + 1 < len(chars) else ""
        if pair in COMBO:
            hep, kun, outside = COMBO[pair]
            tokens.append({"kind": "mora", "text": hep if system == "hepburn" else kun, "kana": pair,
                           "outside": outside and system == "kunrei"})
            i += 2
            continue
        if char in ("ゝ", "ゞ"):  # iteration marks repeat the previous kana
            previous = next((t for t in reversed(tokens) if t["kind"] == "mora"), None)
            if previous and len(previous["kana"]) == 1:
                kana = previous["kana"]
                if char == "ゞ":
                    kana = VOICED.get(kana, kana)
                tokens.append({"kind": "mora", "text": single(kana, system), "kana": kana})
            else:
                tokens.append({"kind": "other", "text": text[i], "source": text[i]})
            i += 1
            continue
        if char == "ー":
            tokens.append({"kind": "long", "text": ""})
        elif char == "っ":
            tokens.append({"kind": "sokuon", "text": ""})
        elif char == "ん":
            tokens.append({"kind": "n", "text": "n"})
        elif char in SINGLE:
            tokens.append({"kind": "mora", "text": single(char, system), "kana": char,
                           "bare": BARE_VOWELS.get(char)})
        else:
            tokens.append({"kind": "other", "text": PUNCT.get(char, text[i]), "source": text[i]})
        i += 1
    return tokens


def render(tokens, system, long_vowels, n_before_bmp, notes, unconverted):
    out = []
    marks = MARK[system]
    previous = None  # index in out of the last mora with its vowel, for long vowels
    for index, token in enumerate(tokens):
        kind = token["kind"]
        following = tokens[index + 1] if index + 1 < len(tokens) else None
        if kind == "mora":
            if token.get("outside"):
                notes.add("extended katakana outside ISO 3602 are written in Hepburn")
            bare = token.get("bare")
            if long_vowels == "mark" and bare and previous is not None:
                position, vowel, lengthened = previous
                if not lengthened and (bare == vowel and vowel in "aueo" or (vowel == "o" and bare == "u")):
                    out[position] = out[position][:-1] + marks[vowel]
                    previous = (position, vowel, True)
                    continue
            out.append(token["text"])
            last = token["text"][-1]
            previous = (len(out) - 1, last, False) if last in "aeiou" else None
        elif kind == "long":
            if previous is None:
                out.append("-")
            elif long_vowels == "as_is":
                position, vowel, _ = previous
                out.append(vowel)
                previous = None
            else:
                position, vowel, lengthened = previous
                if not lengthened:
                    out[position] = out[position][:-1] + marks[vowel]
                    previous = (position, vowel, True)
        elif kind == "sokuon":
            nxt = following["text"] if following and following["kind"] == "mora" else ""
            if nxt and nxt[0] not in "aeiouy":
                out.append("t" if system == "hepburn" and nxt.startswith("ch") else nxt[0])
            else:
                notes.add("a sokuon (small tsu) not followed by a consonant has no romanization and was dropped")
            previous = None
        elif kind == "n":
            nxt = following["text"] if following and following["kind"] == "mora" else ""
            if nxt and nxt[0] in "aeiouy":
                out.append("n'")
            elif system == "hepburn" and n_before_bmp == "m" and nxt[:1] in ("b", "m", "p"):
                out.append("m")
            else:
                out.append("n")
            previous = None
        else:
            source = token.get("source", "")
            if ord(source) > 0x7F and source not in PUNCT and not source.isspace() \
                    and source not in unconverted and len(unconverted) < 50:
                unconverted.append(source)
            out.append(token["text"])
            previous = None
    return "".join(out)


def main():
    try:
        request = json.loads(sys.stdin.buffer.read().decode("utf-8"))
    except (ValueError, UnicodeDecodeError) as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    unknown = sorted(set(request) - {"text", "system", "long_vowels", "n_before_bmp", "capitalize"})
    if unknown:
        fail(f"unknown fields: {', '.join(unknown)}")
    text = request.get("text")
    if not isinstance(text, str) or not text:
        fail("text must be a non-empty string")
    if len(text) > MAX_TEXT:
        fail(f"text is longer than {MAX_TEXT} characters")
    system = request.get("system", "hepburn")
    if system not in ("hepburn", "kunrei"):
        fail("system must be hepburn or kunrei")
    long_vowels = request.get("long_vowels", "mark")
    if long_vowels not in ("mark", "as_is"):
        fail("long_vowels must be mark or as_is")
    n_before_bmp = request.get("n_before_bmp", "n")
    if n_before_bmp not in ("n", "m"):
        fail("n_before_bmp must be n or m")
    capitalize = request.get("capitalize", False)
    if not isinstance(capitalize, bool):
        fail("capitalize must be a boolean")

    # Half-width katakana and full-width ASCII to their usual forms; NFC joins combining dakuten.
    text = "".join(unicodedata.normalize("NFKC", c) if 0xFF00 <= ord(c) <= 0xFFEF else c for c in text)
    text = unicodedata.normalize("NFC", text)
    notes, unconverted = set(), []
    result = render(tokenize(text, system), system, long_vowels, n_before_bmp, notes, unconverted)
    if capitalize:
        result = re.sub(r"(^|\s)([^\W\d_])", lambda m: m.group(1) + m.group(2).upper(), result)
    if unconverted:
        notes.add("kanji and other characters are left as they are (reading kanji needs a dictionary such as pykakasi)")
    answer = {"result": result,
              "system": "modified Hepburn" if system == "hepburn" else "Kunrei-shiki (ISO 3602)",
              "long_vowels": long_vowels}
    if unconverted:
        answer["unconverted"] = unconverted
    if notes:
        answer["notes"] = sorted(notes)
    sys.stdout.buffer.write((json.dumps(answer, ensure_ascii=False) + "\n").encode("utf-8"))


if __name__ == "__main__":
    main()
