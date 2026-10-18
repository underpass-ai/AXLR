# encoding_detect

Works out how a text file was encoded and gives its text in UTF-8. Give the bytes as `hex`
(or a relative `path`), or name the `encoding` to just convert.

## Method, in order

1. **Byte-order mark**: `00 00 FE FF` UTF-32BE, `FF FE 00 00` UTF-32LE, `EF BB BF` UTF-8,
   `FE FF` UTF-16BE, `FF FE` UTF-16LE (Unicode Standard, table 2-4). The BOM is removed.
2. **UTF-16 and UTF-32 without BOM**: text in the Latin range has a NUL in every other byte
   (UTF-16) or three of every four (UTF-32); which side the NULs fall on gives the byte order.
   For other scripts, NULs on one side only (in spaces and digits) are accepted when the
   decoding is all letters, marks, numbers, punctuation and spaces.
3. **7-bit ASCII**: reported as `ascii` (valid in UTF-8 and every candidate), or ISO-2022-JP
   when its escape sequences are present.
4. **Strict UTF-8** (RFC 3629): if the bytes decode, it is UTF-8; random single-byte text with
   accented letters almost never forms valid multi-byte sequences.
5. **Single-byte candidates** (cp1252, latin-1, ISO-8859-15, cp850, mac_roman by default):
   each decoding is scored per non-ASCII character. Accented lower-case letters inside words,
   upper-case letters at word starts, typographic quotes, dashes, the ellipsis and the euro sign
   score up; C1 control characters (latin-1's 0x80–0x9F), box-drawing characters, mathematical
   and other symbols, Greek letters in a Latin text and mid-word capitals score down. The best
   average wins; on a tie the earlier candidate wins (cp1252 first, as browsers read latin-1
   labels as windows-1252), and `equivalent_encodings` lists the candidates that give exactly
   the same text. `confidence` grows with the margin over the first candidate that decodes
   differently. Every candidate's score and a sample are in `candidates`.

The output has `encoding`, `confidence`, `method`, `bom`, the decoded `text`, `utf8_hex` (for up to
4096 characters), the character count and the line endings.

## Tests

Hand-built byte strings: "café" as `63 61 66 e9` (cp1252, with latin-1 and ISO-8859-15
equivalent) and as `63 61 66 c3 a9` (UTF-8); UTF-8, UTF-16LE and UTF-32BE BOMs; UTF-16 without BOM
for English and Greek; cp1252's typographic characters against latin-1; the ISO-8859-15 euro
at `A4`; German text in cp850 and mac_roman; ASCII; a given encoding; and errors.

## Limits

A heuristic, tuned for Western European text: short inputs with one or two non-ASCII bytes can
be ambiguous (see `equivalent_encodings` and `confidence`); Cyrillic, Greek, Central European,
CJK and other legacy code pages are not ranked unless named in `candidates`; mixed encodings in
one file are not split. The charset_normalizer package covers far more (a tier-2 tool).
