# kana_romanize

Romanizes Japanese written in hiragana and katakana, in modified (revised) Hepburn, the
system of the ALA-LC romanization table and of most dictionaries and signs, or in
Kunrei-shiki, the system of the 1954 Cabinet notification standardised as ISO 3602.

## Method

Katakana are mapped onto hiragana (half-width katakana are normalised first, and combining
dakuten joined), so both scripts give the same romanization. The text is read mora by mora: a
kana with a following small ゃ/ゅ/ょ (yōon: きゃ kya, しゃ sha, ちゃ cha, じゃ ja) or a
loanword pair (ファ fa, ティ ti, ディ di, トゥ tu, ウィ wi, ウェ we, ヴァ va, シェ she, ジェ
je, チェ che, ツァ tsa, イェ ye, クァ kwa) is taken before a single kana. Then:

- Sokuon っ doubles the first consonant of the next mora (がっこう gakkō, いっしょ issho, みっつ
  mittsu), and in Hepburn っち is tchi (まっちゃ matcha). A っ before a vowel or at the end has no
  romanization and is dropped with a note.
- ん is n, and n' before a vowel or y (きんえん kin'en, しんよう shin'yō). Modern Hepburn writes
  n before b, m and p as well (shinbun); `n_before_bmp: "m"` gives traditional Hepburn
  (shimbun).
- Long vowels (`long_vowels: "mark"`): ああ ā, うう ū, ええ ē, おお and おう ō, and katakana ー
  lengthens the previous vowel (コーヒー kōhī); えい stays ei and いい stays ii, as in the Hepburn
  tables. Kunrei uses the circumflex (tôkyô). `long_vowels: "as_is"` writes the vowels as spelled
  (toukyou) and ー as a repetition of the vowel (koohii).
- Kunrei-shiki follows the table of ISO 3602: し si, ち ti, つ tu, ふ hu, じ and ぢ zi, づ zu,
  しゃ sya, ちゃ tya, じゃ zya, っち tti. The loanword extensions are not in ISO 3602; they are
  written in Hepburn and the output says so.
- を is o, ゐ i and ゑ e; the iteration marks ゝ ゞ (and ヽ ヾ) repeat the previous kana, voiced
  for ゞ (いすゞ isuzu). Japanese punctuation becomes its ASCII counterpart (。 . 、 , 「」 ").

`capitalize: true` upper-cases the first letter of each space-separated word.

## Tests

The known values come from the published tables and their examples: Tōkyō, gakkō, matcha,
issho, kin'en and shin'yō as the ALA-LC Japanese romanization table spells them (long vowels with
macrons, っち as tch, the apostrophe after ん), shinbun and the traditional shimbun (the spelling
in the name of the Asahi Shimbun), and the ISO 3602 / Kunrei syllables si, ti, tu, hu, zi, tya
(sinbun, tya, huzisan, mattya, tôkyô). The long-vowel cases (ōsaka, okāsan, sūgaku, onēsan, sensei,
oniisan, kōhī) follow the Hepburn long-vowel rules stated above.

## Limits

Kanji are not read: they are copied unchanged and listed in `unconverted` (reading them needs a
dictionary, as pykakasi does in a tier 2 tool). Morpheme boundaries are invisible in kana, so
おう across a boundary is still ō (おもう comes out omō, where dictionaries write omou), the
particles は and へ come out ha and he, not the wa and e they are read as, and words are not
split. Rare kana combinations not in the tables are romanized kana by kana.
