# hangul_romanize

Romanizes Korean written in Hangul with the Revised Romanization of Korean (RR), the system of
the Ministry of Culture and Tourism notice 2000-8 maintained by the National Institute of the
Korean Language, which is used on road signs, in passports and in official names.

## Method

Each precomposed syllable U+AC00–U+D7A3 is split arithmetically (Unicode §3.12): with
`i = code − 0xAC00`, the initial is `i // 588`, the medial `(i % 588) // 28` and the final
`i % 28`, indices into the 19 initials, 21 vowels and 27 finals (plus none). Each run of
Hangul syllables without spaces is one word. Non-Hangul characters are copied unchanged, and
non-ASCII ones (Hanja, for example) are listed in `unconverted`.

**Pronunciation mode** (default). Between each final and the next initial:

- Liaison (연음): before a silent ㅇ the final moves to the next syllable (설악 Seorak); a double
  final keeps its first letter and moves the second (읽어 ilgeo, 없어 eopseo); ㅎ is silent
  before a vowel (좋아 joa, 싫어 sireo); final ㅇ stays.
- Palatalisation: ㄷ/ㅌ moved onto 이 become ㅈ/ㅊ (해돋이 haedoji, 같이 gachi), and ㄷ + 히 is
  치 (굳히다 guchida).
- Aspiration: ㅎ (also in ㄶ, ㅀ) before ㄱ/ㄷ/ㅈ gives ㅋ/ㅌ/ㅊ (좋고 joko, 놓다 nota, 않고
  anko), and ㄱ/ㄷ/ㅂ/ㅈ before ㅎ give ㅋ/ㅌ/ㅍ/ㅊ (잡혀 japyeo, 낳지 nachi). With
  `keep_h: true` the h is kept after ㄱ/ㄷ/ㅂ (and ㄳ, ㄺ, ㅄ), as the rules require for nouns (묵호 Mukho,
  집현전 Jiphyeonjeon); ㅎ before ㄴ becomes ㄴ (놓는 nonneun).
- Coda neutralisation: finals are written k (ㄱ ㄲ ㅋ ㄳ ㄺ), n (ㄴ ㄵ ㄶ), t (ㄷ ㅅ ㅆ ㅈ ㅊ ㅌ
  ㅎ), l (ㄹ ㄼ ㄽ ㄾ ㅀ), m (ㅁ ㄻ), p (ㅂ ㅍ ㅄ ㄿ) or ng (ㅇ).
- ㄹ after k, t, p, m or ng becomes ㄴ (종로 Jongno, 백리 baengni); then k/t/p before ㄴ or ㅁ
  become ng/n/m (백마 Baengma, 왕십리 Wangsimni, 읽는 ingneun). ㄴ + ㄹ and ㄹ + ㄴ are ll
  (신라 Silla, 별내 Byeollae, 대관령 Daegwallyeong).
- Letters: ㄱ ㄷ ㅂ ㅈ are g d b j before a vowel and k t p t (neutralised) in codas; ㄹ is r
  before a vowel, l in a coda and ll for ㄹㄹ (울릉 Ulleung); tensing is not written
  (압구정 Apgujeong).

**Transliteration mode** (Article 8) spells the letters, not the sounds, so it can be turned
back into Hangul: ㄱ ㄷ ㅂ ㄹ are always g d b l, finals are written letter by letter (ㅄ bs,
ㅆ ss, ㅊ ch), a silent ㅇ after another syllable becomes `-`, and a hyphen also separates a
final from the next initial when their letters could be split another way (ㅆ + ㅅ "sss" could
also be ㅅ + ㅆ, so 없었습니다 is eobs-eoss-seubnida).

`capitalize: true` upper-cases the first letter of each word, as for proper names.

## Tests

The known values are the examples printed in the 2000 rules: the ㄹ and coda examples of
Chapter 2 Article 2 (Seorak, Ulleung, Daegwallyeong, Baegam, Okcheon, Hapdeok, Wolgot,
beotkkot, Hanbat), the sound-change examples of Chapter 3 Article 1 (Baengma, Jongno,
Wangsimni, Byeollae, Silla, haedoji, gachi, guchida, joko, nota, japyeo, nachi, Mukho,
Jiphyeonjeon) and the Article 8 transliterations (jib, jip, bakk, gabs, buskkoch, meogneun,
doglib, munli, mul-yeos, gud-i, johda, gagog, jolangmal, eobs-eoss-seubnida). Double finals
before ㄴ and a vowel are checked by hand against the Standard Pronunciation rules.

## Limits

Sound changes that depend on the word's morphology cannot be seen in the spelling: ㄴ/ㄹ
insertion (학여울 Hangnyeoul and 알약 allyak in the rules come out Hagyeoul and aryak), ㄹ read
as ㄴ after ㄴ in some compounds (신문로 Sinmunno comes out Sinmullo), and palatalisation or
liaison across a morpheme boundary that the rules treat differently. `keep_h` applies to the
whole text, so a sentence mixing nouns and verbs needs two calls. Not handled: the optional
hyphen for ambiguous syllables (중앙 Jung-ang), personal names (no sound changes between given
name syllables), the hyphen before administrative units (-do, -si, -gun, -gu, -eup, -myeon,
-ri) and the exceptions for established spellings. Old Hangul and standalone jamo are copied
unchanged.
