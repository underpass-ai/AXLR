# pinyin_convert

Hanzi to Hanyu Pinyin, character by character, on the standard library only. It replaces the
tier-2 `pypinyin` (python-pinyin, MIT) for the common case of reading characters; what it does
not do is listed under Limits.

## Data

Three text tables written by `tools/catalog/data/pinyin_convert.py` from the Unicode Han
Database (`Unihan.zip`, Unicode 18.0.0, files dated 2026-07-31, fetched 2026-10-18 from
unicode.org; Unicode License v3, whose notice the tables carry):

| File | Characters |
|:--|:--|
| `readings_core.txt` | the 8105 characters of the Table of General Standard Chinese Characters (通用规范汉字表, 2013), from Unihan's `kTGH` field, in table order |
| `readings_ext_1.txt`, `readings_ext_2.txt` | the other 36 250 code points that have a `kMandarin` reading (CJK Unified Ideographs, their extensions and compatibility ideographs), in code-point order |

That is every one of the 44 355 characters Unihan gives a Mandarin reading. Each line holds the
character and its readings with tone numbers (5 = neutral): first `kMandarin`'s primary value
(the zh-Hans reading), then a second `kMandarin` value (zh-Hant) if any, then the readings of
通用规范汉字字典 (`kTGHZ2013`) and 现代汉语词典 (`kXHC1983`); outside the core table, 汉语大字典
(`kHanyuPinyin`) as well. Two Unihan entries are adjusted, and the generator reports both: U+6077
`qīu` is stored as `qiu1` (the placement rule and every other *iu* syllable give `qiū`), and
U+74F2 `túnwǎ` ("ton-watt"), the only two-syllable reading, is stored as two syllables.

## Method

- **Reading**: the primary reading of each character; the others are listed in `alternatives`.
  Polyphonic characters therefore read the most customary way (行 xíng, 长 zhǎng in Unihan 18,
  的 de), whatever the word.
- **Tone marks** (GB/T 16159 and the Pinyin scheme): the mark goes on *a* or *e* when present,
  otherwise on the *o* of *ou*, otherwise on the last vowel, which puts it on the second letter
  of *iu* and *ui* (liú, duì). Syllables without a vowel take it on *m* or *n* (ǹg, ḿ). The
  generator checks that this rule turns every stored number back into Unihan's spelling.
- **Joining**: `separator` goes between the syllables of a run of Hanzi (default a space). With
  `""` an apostrophe precedes a syllable that starts with *a*, *o* or *e* (Xī'ān). `capitalize`
  upper-cases the first letter of each run. Other text is copied; a space separates pinyin from
  Latin letters or digits that touch it.
- **Sandhi** (`sandhi: true`; the dictionary reading is then in `dictionary`): 一 becomes yí
  before a fourth tone and yì before the other tones, but stays yī at the end of a run, after
  第 or a numeral (第一, 十一, 万一) and before a digit (一二三); 不 becomes bú before a fourth tone;
  in a run of consecutive third tones every syllable but the last becomes second tone (你好
  ní hǎo, 我很好 wó hén hǎo). Real third-tone sandhi follows prosodic words (我 | 很好 is also
  said wǒ hén hǎo), which needs the segmentation this tool does not have.

## Tests

Known values from outside the tool: 中国 written Zhōngguó and the apostrophe in Xī'ān and
Tiān'ānmén (GB/T 16159-2012 orthography rules), the README examples of python-pinyin (中心
zhōng xīn, `TONE3` zhong1 xin1, `NORMAL` zhong xin, heteronyms zhōng/zhòng), Unihan's own
marked spellings for the placement rule (hǎo lüè ōu liú duì shuǐ liù lǜ nǚ ǹg) and the textbook
sandhi of 一 and 不 (yí gè, yì tiān, dì yī, bú shì, bù hǎo) and 你好 ní hǎo.

## Limits

No word segmentation and no phrase dictionary: pypinyin chooses 行 háng in 银行 and 长 cháng in
长城 from its phrase list, this tool does not, and returns the alternatives instead. No zhuyin
(bopomofo), no Wade-Giles, no erhua merging (一点儿 gives yī diǎn ér), no neutral tones from
context (是不是, 看一看); tone sandhi is a rule of thumb, never applied unless asked.
