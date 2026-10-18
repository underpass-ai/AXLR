# unicode_tools

Unicode inspection with the standard library's `unicodedata`. Choose an `operation`:

| Operation | Answer |
|:--|:--|
| `normalize` | the text in NFC, NFD, NFKC and NFKD (or the one `form` given), each with its code points, length and whether the input was already in that form (`unicodedata.is_normalized`) |
| `strip_accents` | NFD, drop every combining mark (general category M), recompose to NFC. Letters that do not decompose (`ß`, `Ł`, `ø`, `đ`) are kept and listed in `non_ascii_letters_left` |
| `codepoints` | per character: `U+XXXX`, the Unicode name, the general category and its meaning, the script, the canonical combining class, the decomposition and the UTF-8 bytes |
| `bytes` | the UTF-8, UTF-16BE, UTF-16LE, UTF-32BE and UTF-32LE bytes in hex (no BOM), their lengths and the number of UTF-16 code units |
| `decode` | `hex` bytes decoded as `encoding` (UTF-8 by default) into text and code points; invalid sequences are an error naming the byte offset |
| `confusables` | words that mix Latin, Cyrillic and Greek letters, and words made only of Cyrillic or Greek letters that look Latin, with each lookalike and the word's Latin "skeleton" |

Every answer carries `unidata_version`: names, categories and normalisation are those of the
Unicode version built into the Python that runs the tool (15.0.0 for Python 3.12, 13.0.0 for
3.10), so a character newer than that has no name and category `Cn`.

## Homoglyphs

The table in the program has the 69 lines of Unicode TR39 `confusables.txt` (version 18.0.0,
dated 2026-08-06, https://www.unicode.org/Public/security/latest/confusables.txt) that map one
Greek or Cyrillic letter in U+0370–U+04FF to one Latin letter, for example
`0430 ; 0061 ; MA # ( а → a ) CYRILLIC SMALL LETTER A → LATIN SMALL LETTER A`. Coptic letters in
the Greek block are left out. Each pair was checked against the file when the table was made.
It is not the full TR39 skeleton algorithm: digits, punctuation, other scripts and
multi-character prototypes are not covered.

`unicodedata` has no Script property, so the script of a letter comes from a small range table
in the program (the Latin, Greek and Cyrillic ranges of Scripts.txt that matter here; any other
letter is `Other`, combining marks inherit the script of the letter before them). A word is a
run of letters and the marks attached to them. A word is reported when it has letters of more
than one script (`mixed_script`), or when every letter is in the lookalike table
(`all_letters_look_latin`: Cyrillic `сосо` reads as `coco`, Greek `ΑΝΝΑ` as `ANNA`). Ordinary
Russian or Greek words that merely contain some lookalike letters are not reported.

## Tests

- UnicodeData.txt: U+00E9 has the canonical decomposition `0065 0301`, so NFC of `é` is U+00E9
  and NFD is U+0065 U+0301; the names LATIN SMALL LETTER E WITH ACUTE and COMBINING ACUTE ACCENT
  (combining class 230).
- UAX #15: the ligature U+FB01 is a compatibility character, unchanged by NFC, `f` `i` under
  NFKC.
- RFC 3629 section 7: `A≢Α.` is `41 E2 89 A2 CE 91 2E`, Japanese "nihongo" is
  `E6 97 A5 E6 9C AC E8 AA 9E`, and `EF BB BF F0 A3 8E B4` is a BOM followed by U+233B4.
- RFC 2781 section 5: U+12345 followed by `=Ra` is `D8 08 DF 45 00 3D 00 52 00 61` in UTF-16BE
  and `08 D8 45 DF 3D 00 52 00 61 00` in UTF-16LE.
- confusables.txt: the line `0430 ; 0061` (a Cyrillic а inside `paypal` makes it a mixed-script
  word), and `0441 ; 0063`, `043E ; 006F` (Cyrillic `сосо`).
- Usage errors: invalid UTF-8, a lone surrogate, a bad operation or form.

## Limits

Input up to 20,000 characters; lists are cut at 2,000 entries. No case folding, collation,
grapheme clusters, bidi or the full TR39 restriction levels. Strip-accents removes every mark,
which changes meaning in some languages (Vietnamese tones, Hebrew points, Devanagari vowel
signs).

## Unicode licence notice

The homoglyph pairs come from `confusables.txt`, a Unicode Data File used under the Unicode
License v3; the program's docstring carries its copyright and permission notice: Copyright ©
1991-2026 Unicode, Inc. Permission is hereby granted, free of charge, to any person obtaining a
copy of data files and any associated documentation (the "Data Files") or software and any
associated documentation (the "Software") to deal in the Data Files or Software without
restriction, including without limitation the rights to use, copy, modify, merge, publish,
distribute, and/or sell copies of the Data Files or Software, and to permit persons to whom the
Data Files or Software are furnished to do so, provided that either (a) this copyright and
permission notice appear with all copies of the Data Files or Software, or (b) this copyright
and permission notice appear in associated Documentation. THE DATA FILES AND SOFTWARE ARE
PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND.
