#!/usr/bin/env python3
"""Hanzi to Hanyu Pinyin, character by character, from the bundled Unihan readings.

Input (JSON on stdin):
  text        Chinese text; characters without a reading are copied unchanged
  style       "marks" (zhōng, default) | "numbers" (zhong1) | "none" (zhong)
  separator   between syllables of a run of Hanzi (default " "); with "" an apostrophe is put
              before a syllable starting with a, e or o (Xī'ān)
  capitalize  upper-case the first letter of each run of Hanzi (default false)
  sandhi      apply the tone sandhi of 一, 不 and the third tone (default false)
  umlaut      how ü is written in numbers/none styles: "ü" (default), "v" or "u:"
  neutral_five  write the neutral tone as 5 in numbers style (default false: no digit)
The reading of a character is Unihan's kMandarin primary (zh-Hans); the others it lists are
returned as alternatives. There is no word segmentation, so a polyphonic character always
gets its primary reading.
"""
import json
import pathlib
import sys
import unicodedata

HERE = pathlib.Path(__file__).resolve().parent
COMBINING = {1: "̄", 2: "́", 3: "̌", 4: "̀"}
DIGITS = set("零〇一二三四五六七八九十")       # 一二三, 一十: 一 is a digit and keeps yī
BEFORE = DIGITS | set("两百千万亿第")         # 第一, 十一, 一百零一, 万一: 一 keeps yī


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load():
    table, meta = {}, []
    for path in sorted(HERE.glob("readings_*.txt")):
        for line in path.read_text(encoding="utf-8").splitlines():
            if line.startswith("#"):
                if path.name == "readings_core.txt":
                    meta.append(line[2:])
                continue
            char, _, readings = line.partition("\t")
            if char and readings:
                table[char] = readings.split()
    if not table:
        fail("the bundled reading tables are missing")
    return table, meta


def tone_of(syllable):
    return int(syllable[-1])


def mark(syllable):
    """Tone mark placement: on a or e if present, else on the o of ou, else on the last vowel
    (so iu -> iú, ui -> uí); a syllable with no vowel (m, n, ng) takes it on m or n."""
    base, tone = syllable[:-1], tone_of(syllable)
    if tone == 5:
        return base
    if "a" in base:
        at = base.index("a")
    elif "e" in base:
        at = base.index("e")
    elif "ê" in base:
        at = base.index("ê")
    elif "ou" in base:
        at = base.index("o")
    else:
        vowels = [i for i, c in enumerate(base) if c in "iouü"]
        at = vowels[-1] if vowels else next(i for i, c in enumerate(base) if c in "mn")
    return unicodedata.normalize("NFC", base[:at + 1] + COMBINING[tone] + base[at + 1:])


def render(syllable, options):
    parts = syllable.split("+")  # one Unihan reading has two syllables (tun2+wa3)
    out = []
    for part in parts:
        if options["style"] == "marks":
            out.append(mark(part))
            continue
        base, tone = part[:-1], tone_of(part)
        base = base.replace("ü", options["umlaut"])
        if options["style"] == "numbers":
            out.append(base + ("" if tone == 5 and not options["neutral_five"] else str(tone)))
        else:
            out.append(base)
    return "".join(out)


def with_tone(syllable, tone):
    return syllable[:-1] + str(tone)


def apply_sandhi(run):
    """run: list of (char, primary numbered reading). Returns the spoken readings."""
    base = [reading for _, reading in run]
    spoken = list(base)
    for i, (char, reading) in enumerate(run):
        nxt = base[i + 1] if i + 1 < len(run) else None
        if "+" in reading or (nxt and "+" in nxt):
            continue
        if char == "一" and reading == "yi1" and nxt:
            prev = run[i - 1][0] if i else ""
            if prev in BEFORE or run[i + 1][0] in DIGITS:
                continue
            spoken[i] = with_tone(reading, 2 if tone_of(nxt) == 4 else 4)
        elif char == "不" and reading == "bu4" and nxt and tone_of(nxt) == 4:
            spoken[i] = "bu2"
    # third tone: in a run of consecutive third tones every syllable but the last becomes second
    i = 0
    while i < len(spoken):
        if "+" not in spoken[i] and tone_of(spoken[i]) == 3:
            j = i
            while j + 1 < len(spoken) and "+" not in spoken[j + 1] and tone_of(spoken[j + 1]) == 3:
                j += 1
            for k in range(i, j):
                spoken[k] = with_tone(spoken[k], 2)
            i = j + 1
        else:
            i += 1
    return spoken


def needs_apostrophe(rendered):
    return rendered[:1].lower() in ("a", "e", "o", "ā", "á", "ǎ", "à", "ē", "é", "ě", "è", "ō", "ó", "ǒ", "ò", "ê")


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    try:
        json.dumps(request, ensure_ascii=False).encode("utf-8")
    except UnicodeEncodeError:
        fail("input contains an unpaired surrogate (not valid Unicode text)")
    known = {"text", "style", "separator", "capitalize", "sandhi", "umlaut", "neutral_five"}
    unknown = set(request) - known
    if unknown:
        fail(f"unknown field(s): {', '.join(sorted(unknown))}")
    text = request.get("text")
    if not isinstance(text, str) or not text:
        fail("text must be a non-empty string")
    if len(text) > 100000:
        fail("text is longer than 100000 characters")
    options = {
        "style": request.get("style", "marks"),
        "separator": request.get("separator", " "),
        "capitalize": request.get("capitalize", False),
        "sandhi": request.get("sandhi", False),
        "umlaut": request.get("umlaut", "ü"),
        "neutral_five": request.get("neutral_five", False),
    }
    if options["style"] not in ("marks", "numbers", "none"):
        fail("style must be marks, numbers or none")
    if not isinstance(options["separator"], str) or len(options["separator"]) > 10:
        fail("separator must be a string of at most 10 characters")
    for flag in ("capitalize", "sandhi", "neutral_five"):
        if not isinstance(options[flag], bool):
            fail(f"{flag} must be true or false")
    if options["umlaut"] not in ("ü", "v", "u:"):
        fail("umlaut must be ü, v or u:")
    table, meta = load()

    # split into runs of characters with a reading and stretches of other text
    pieces, run = [], []
    for char in text:
        if char in table:
            run.append(char)
        else:
            if run:
                pieces.append(("hanzi", run))
                run = []
            if pieces and pieces[-1][0] == "other":
                pieces[-1] = ("other", pieces[-1][1] + char)
            else:
                pieces.append(("other", char))
    if run:
        pieces.append(("hanzi", run))

    out, syllables, unknown_chars = [], [], []
    for kind, content in pieces:
        if kind == "other":
            out.append(content)
            for char in content:
                if unicodedata.category(char) == "Lo" and "CJK" in unicodedata.name(char, ""):
                    unknown_chars.append(char)
            continue
        primary = [(char, table[char][0]) for char in content]
        spoken = apply_sandhi(primary) if options["sandhi"] else [r for _, r in primary]
        words = []
        for index, (char, reading) in enumerate(primary):
            rendered = render(spoken[index], options)
            if index and options["separator"] == "" and needs_apostrophe(rendered):
                rendered = "'" + rendered
            words.append(rendered)
            item = {"char": char, "pinyin": render(spoken[index], options).lstrip("'")}
            if options["sandhi"] and spoken[index] != reading:
                item["dictionary"] = render(reading, options)
            alternatives = []
            primary_shown = render(reading, options)
            for other in table[char][1:]:
                shown = render(other, options)
                if shown != primary_shown and shown not in alternatives:
                    alternatives.append(shown)
            if alternatives:
                item["alternatives"] = alternatives
            syllables.append(item)
        joined = options["separator"].join(words)
        if options["capitalize"] and joined:
            joined = joined[0].upper() + joined[1:]
        out.append(joined)
    # keep pinyin apart from Latin letters or digits that touch a run of Hanzi
    result_text = ""
    for index, (kind, _) in enumerate(pieces):
        piece = out[index]
        if index and result_text[-1:].isascii() and result_text[-1:].isalnum() and piece[:1].isalnum():
            piece = " " + piece
        result_text += piece
    result = {
        "pinyin": result_text,
        "style": options["style"],
        "sandhi": options["sandhi"],
        "syllables": syllables,
        "source": "Unihan database (" + ", ".join(m for m in meta if m.startswith(("Unicode Version", "Date"))) + "): kMandarin primary reading; alternatives from kMandarin, kTGHZ2013, kXHC1983 (kHanyuPinyin outside the 8105 general-standard characters); Unicode License v3",
    }
    if unknown_chars:
        result["no_reading"] = sorted(set(unknown_chars))
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
