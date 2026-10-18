#!/usr/bin/env python3
"""Write the reading tables of catalog/tools/language/pinyin_convert/ from the Unicode Han
Database (Unihan), fetched once (this generator uses the network; the tool never does).

Source: https://www.unicode.org/Public/UCD/latest/ucd/Unihan.zip
  Unicode 18.0.0, Unihan files dated 2026-07-31, fetched 2026-10-18.
  Licence: Unicode License v3 (https://www.unicode.org/license.txt), which permits copying
  and redistribution of the data files with the copyright notice; the tables carry it.
Fields used:
  kMandarin   the customary Mandarin reading(s): the first value is the reading for
              zh-Hans (mainland), a second one, if present, for zh-Hant (Taiwan)
  kTGH        position in the Table of General Standard Chinese Characters
              (通用规范汉字表, 2013): its 8105 characters are the core table, in table order
  kTGHZ2013   readings in 通用规范汉字字典 (2013)
  kXHC1983    readings in 现代汉语词典 (1983)
  kHanyuPinyin readings in 汉语大字典 (used for alternatives outside the core table only)

Output (text, one character per line, readings with tone numbers 1-4 and 5 for the neutral
tone, primary first, alternatives after it; ü and ê kept as such):
  readings_core.txt    the 8105 kTGH characters, in kTGH order
  readings_ext_N.txt   every other code point with kMandarin, in code-point order, split so
                       that no file exceeds 256 KiB
The generator checks that the tool's tone-mark placement rule (a or e, else the o of ou, else
the last vowel) turns every numbered reading back into Unihan's marked spelling.

Usage: python3 tools/catalog/data/pinyin_convert.py [--zip Unihan.zip]
"""
import datetime as dt
import io
import pathlib
import re
import sys
import unicodedata
import urllib.request
import zipfile

URL = "https://www.unicode.org/Public/UCD/latest/ucd/Unihan.zip"
OUT = pathlib.Path(__file__).resolve().parents[3] / "catalog" / "tools" / "language" / "pinyin_convert"
LIMIT = 256 * 1024 - 2048
MARKS = {"\u0304": 1, "\u0301": 2, "\u030c": 3, "\u0300": 4}
COMBINING = {1: "\u0304", 2: "\u0301", 3: "\u030c", 4: "\u0300"}
# Unihan 18.0 has one reading of two syllables (U+74F2, "ton-watt"); it is split by hand and
# stored as tun2+wa3. Any other multi-syllable reading stops the generator.
MULTI = {"t\u00fanw\u01ce": ["t\u00fan", "w\u01ce"]}
# A reading whose mark sits where the rule does not put it is stored by the rule and reported;
# Unihan 18.0 has one: U+6077 kMandarin "q\u012bu" (the rule, like every other iu syllable, gives qiu1).


def numbered(marked):
    """'zhōng' -> 'zhong1', 'lǜ' -> 'lü4', 'de' -> 'de5'."""
    tone = 5
    out = []
    for ch in unicodedata.normalize("NFD", marked.strip().lower()):
        if ch in MARKS:
            tone = MARKS[ch]
        else:
            out.append(ch)
    return unicodedata.normalize("NFC", "".join(out)) + str(tone)


def mark(syllable):
    """Copy of the tool's rule: 'liu2' -> 'liú'."""
    base, tone = syllable[:-1], int(syllable[-1])
    if tone == 5:
        return base
    letters = unicodedata.normalize("NFC", base)
    if "a" in letters:
        at = letters.index("a")
    elif "e" in letters or "ê" in letters:
        at = letters.index("e") if "e" in letters else letters.index("ê")
    elif "ou" in letters:
        at = letters.index("o")
    else:
        vowels = [i for i, c in enumerate(letters) if c in "iouü"]
        if vowels:
            at = vowels[-1]
        else:
            nasal = [i for i, c in enumerate(letters) if c in "mn"]
            at = nasal[0]
    return unicodedata.normalize("NFC", letters[:at + 1] + COMBINING[tone] + letters[at + 1:])


def readings_of(field, value):
    if field == "kMandarin":
        return value.split()
    found = []
    for group in value.split():
        _, _, spellings = group.partition(":")
        found.extend(s for s in spellings.split(",") if s)
    return found


def load(zip_bytes):
    archive = zipfile.ZipFile(io.BytesIO(zip_bytes))
    fields = {}
    header = []
    for name in ("Unihan_Readings.txt", "Unihan_OtherMappings.txt"):
        for line in archive.read(name).decode("utf-8").splitlines():
            if line.startswith("#"):
                if name == "Unihan_Readings.txt" and re.match(r"# (Date|Unicode Version|©)", line):
                    header.append(line[2:])
                continue
            parts = line.split("\t")
            if len(parts) == 3 and parts[1] in ("kMandarin", "kTGH", "kTGHZ2013", "kXHC1983", "kHanyuPinyin"):
                fields.setdefault(chr(int(parts[0][2:], 16)), {})[parts[1]] = parts[2]
    return fields, header


def main():
    args = sys.argv[1:]
    if args[:1] == ["--zip"] and len(args) == 2:
        zip_bytes = pathlib.Path(args[1]).read_bytes()
    else:
        request = urllib.request.Request(URL, headers={"User-Agent": "AXLR-catalog data generator"})
        with urllib.request.urlopen(request, timeout=300) as reply:
            zip_bytes = reply.read()
    fields, header = load(zip_bytes)
    core, rest, bad = [], [], []
    for char, values in fields.items():
        if "kMandarin" not in values:
            continue
        sources = ["kMandarin", "kTGHZ2013", "kXHC1983"] + ([] if "kTGH" in values else ["kHanyuPinyin"])
        spellings = []
        for field in sources:
            for reading in readings_of(field, values.get(field, "")):
                marks = sum(c in MARKS for c in unicodedata.normalize("NFD", reading))
                if marks > 1:
                    if reading not in MULTI:
                        bad.append((char, reading, "several syllables", ""))
                        continue
                    syllable = "+".join(numbered(part) for part in MULTI[reading])
                else:
                    syllable = numbered(reading)
                    if mark(syllable) != unicodedata.normalize("NFC", reading.lower()):
                        print(f"note: {char} U+{ord(char):04X} {field} {reading!r} stored as {syllable} ({mark(syllable)})")
                if syllable not in spellings:
                    spellings.append(syllable)
        line = char + "\t" + " ".join(spellings)
        if "kTGH" in values:
            core.append((int(values["kTGH"].split(":")[1]), line))
        else:
            rest.append((ord(char), line))
    if bad:
        for item in bad[:20]:
            print("tone-mark rule mismatch:", item, file=sys.stderr)
        raise SystemExit(f"{len(bad)} readings do not round-trip through the tone-mark rule")
    today = dt.date.today().isoformat()
    notice = [f"# Source: Unihan database, {URL}", *[f"# {h}" for h in header],
              "# Licence: Unicode License v3, https://www.unicode.org/license.txt",
              "# Copyright (c) 1991-2026 Unicode, Inc. Distributed under the terms of the Unicode License v3.",
              f"# Fetched {today} by tools/catalog/data/pinyin_convert.py; readings with tone numbers (5 = neutral),",
              "# first = kMandarin primary (zh-Hans), then other kMandarin, kTGHZ2013, kXHC1983"]
    for old in OUT.glob("readings_*.txt"):
        old.unlink()
    files = []

    def write(name, lines, what):
        text = "\n".join(notice + [f"# {what}"] + lines) + "\n"
        (OUT / name).write_text(text, encoding="utf-8")
        files.append((name, len(lines), len(text.encode("utf-8"))))

    write("readings_core.txt", [line for _, line in sorted(core)],
          "Table of General Standard Chinese Characters (2013), kTGH order")
    chunk, size, number = [], 0, 1
    for _, line in sorted(rest):
        if size + len(line.encode("utf-8")) + 1 > LIMIT - 1200:
            write(f"readings_ext_{number}.txt", chunk, "other characters with kMandarin (+ kHanyuPinyin alternatives), code-point order")
            chunk, size, number = [], 0, number + 1
        chunk.append(line)
        size += len(line.encode("utf-8")) + 1
    if chunk:
        write(f"readings_ext_{number}.txt", chunk, "other characters with kMandarin (+ kHanyuPinyin alternatives), code-point order")
    for name, count, size in files:
        print(f"{name}: {count} characters, {size} bytes")


if __name__ == "__main__":
    main()
