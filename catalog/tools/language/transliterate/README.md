# transliterate

Writes Cyrillic, Greek and Arabic text in Latin letters, from tables kept in the program. Each
character is handled by the table of its own script, so mixed text works; Latin letters, digits
and punctuation are copied, and letters of other scripts are copied and listed in `unmapped`.
The output names the scripts found and the system used for each.

## Cyrillic

- `iso9` (default), **ISO 9:1995**: one Latin letter, with diacritics where needed, for each
  Cyrillic letter, the same for every language, so the text can be converted back: ж ž, х h,
  ц c, ч č, ш š, щ ŝ, ю û, я â, й j, э è, ы y, ь ʹ, ъ ʺ, and for the other Slavic alphabets
  ґ g̀, є ê, і ì, ї ï (Ukrainian), ў ŭ (Belarusian), ђ đ, ћ ć, ј ǰ, љ l̂, њ n̂, џ d̂
  (Serbian), ѓ ǵ, ќ ḱ, ѕ ẑ (Macedonian), and the pre-1918 letters ѣ ě, ѳ f̀, ѵ ỳ, ѫ ǎ.
  Bulgarian щ and ъ are ŝ and ʺ, as in Russian.
- `scholarly`, the scientific transliteration used in Slavic linguistics, for the Russian
  alphabet: as ISO 9 but х x (some older tables, such as ISO/R 9:1968, write ch), щ šč, ю ju,
  я ja.
- `bgn_pcgn`, **BGN/PCGN 1947** for Russian, the system of English-language maps and gazetteers:
  ж zh, х kh, ц ts, ч ch, ш sh, щ shch, ю yu, я ya, й y, э e, ь ’, ъ ”. е is ye and ё yë at
  the start of a word and after a vowel, й, ъ or ь (Ельцин Yel’tsin, поездка poyezdka, объект
  ob”yekt), otherwise e and ë.

`scholarly` and `bgn_pcgn` cover the Russian alphabet only: other Cyrillic letters (Ukrainian ї,
Serbian ђ) are left and listed in `unmapped`. `cyrillic_signs` writes the soft and hard signs
as the system's marks (`standard`), as ASCII `'` and `"` (`ascii`, which gives the usual
Yel'tsin) or drops them (`omit`).

Case: a capital becomes a capital; a capital with a multi-letter output is written in title case
unless its word is in capitals (Щука Shchuka, ЩИ SHCHI, Θεσσαλονίκη Thessaloniki).

## Greek

**ELOT 743 / ISO 843 transcription**, the Greek national standard approved by the UN for
geographical names and used on road signs and passports. Letters: β v, γ g, η i, θ th, ξ x,
υ y, φ f, χ ch, ψ ps, ω o, final ς s; accents and breathings are dropped. Position rules:

- αυ, ευ, ηυ are av, ev, iv before a vowel or β γ δ ζ λ μ ν ρ, and af, ef, if before θ κ ξ π
  σ τ φ χ ψ and at the end of a word (Ευρώπη Evropi, αυτός aftos, Παύλος Pavlos); ου is ou.
- μπ is b at the start and the end of a word and mp inside it (Μπαμπάς Bampas); γγ ng, γξ nx,
  γχ nch; γκ gk and ντ nt letter by letter.
- A diaeresis on ι or υ, or an accent on the first vowel, means the two vowels are separate,
  so no digraph is formed: Ταΰγετος Taÿgetos, άυλος aylos. The diaeresis is kept as ï ÿ
  (`greek_diaeresis: "drop"` writes i y).

## Arabic

A simplified subset of the **ALA-LC** Arabic table: ب b, ت t, ث th, ج j, ح ḥ, خ kh, د d, ذ dh,
ر r, ز z, س s, ش sh, ص ṣ, ض ḍ, ط ṭ, ظ ẓ, ع ʻ, غ gh, ف f, ق q, ك k, ل l, م m, ن n, ه h, و w,
ي y, hamza ʼ (not written at the start of a word), ة h (t before a vowel mark), ى á, and ا ā
after the first letter; the Persian letters پ p, چ ch, ژ zh, گ g. The article ال is al- before
every letter, with no assimilation (al-shams), as ALA-LC writes it. When the text carries vowel
marks, fatha, kasra and damma are a, i, u, tanwin an, in, un, shadda doubles the consonant, and a
short vowel followed by its long letter becomes ā, ī, ū (final -iyy is ī). Unvocalized Arabic,
the normal case, gives a consonant skeleton with long ā only: **short vowels are not written in
the script and are not restored** (القاهرة comes out al-qāhrh, where ALA-LC has al-Qāhirah);
the output says so. Arabic-Indic digits become 0-9.

## Tests

ISO 9 and BGN/PCGN values are read from the systems' tables: Москва Moskva in both, and the
letters that tell them apart in Хрущёв (Hruŝëv / Khrushchëv), Ельцин (Elʹcin / Yel'tsin), Юрий
(Ûrij / Yuriy) and Майя (Majâ / Mayya), the BGN positional ye, and the ISO 9 letters of
Ukrainian, Serbian, Macedonian and Belarusian. Greek values are the official ELOT 743 spellings
of Greek place names (Athina, Thessaloniki, Peiraias, Irakleio, Chalkida, Kerkyra) and words
worked through the av/af, mp/b and γ rules (Evropi, aftos, Avgoustos, Angelos, sfinx, elencho).
Arabic values follow the ALA-LC table and rules (muḥammad, kitāb, ʻalī, al-shams, al-qamar).

## Limits

No other Cyrillic systems (GOST 7.79 System B, national passport systems of Ukraine or
Russia, Serbian Latin orthography) and no language-specific variants for scholarly and BGN/PCGN
outside Russian; the BGN/PCGN optional middle dot (t·s) is not written. Greek is the modern
transcription only (no ISO 843 letter-by-letter transliteration, no Ancient Greek). Arabic does
not restore vowels, does not split attached prefixes (وال wa-al-, بال bi-al-), and uses the
pausal ة h; ALA-LC capitalization of proper names is not applied. Hebrew and other scripts are
not covered. Text is NFC-normalised first, so a Russian г or к carrying a typographic stress
mark (U+0301) becomes ѓ or ќ and is written ǵ or ḱ.
