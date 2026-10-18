# language_detect

Guesses which of 20 languages a text is in: English, Spanish, French, German, Portuguese,
Italian, Dutch, Catalan, Romanian, Polish, Czech, Swedish, Danish, Norwegian (Bokmål), Finnish,
Hungarian, Turkish, Indonesian, Russian and Greek. It is a heuristic. Short texts, texts that mix
languages and close pairs (Danish and Norwegian, Spanish and Catalan or Portuguese) are
unreliable, and a language without a profile can only come out as `unknown` or as the nearest
profiled language.

**Data source note.** The profiles are not built from the Universal Declaration of Human Rights.
The brief pointed to the "UDHR in Unicode" translations, but on 2026-10-18 https://unicode.org/udhr/
said: "Effective January 2024, the Unicode Consortium is no longer hosting the UDHR in Unicode
project." The UN's terms of use (https://www.un.org/en/about-us/terms-of-use) grant copies "for
the User's personal, non-commercial use, without any right to resell or redistribute them or to
compile or create derivative works therefrom"; the OHCHR translation page answered with a bot
challenge; the successor project UDHR in XML (http://efele.net/udhr, github.com/eric-muller/udhr)
has no licence file, only the sentence "The UDHR and all its translations are obviously part of
our commons." None of these is a clear permissive licence, so the profiles are counted from
Unicode CLDR locale data instead, under the Unicode License v3. `profiles.json` records the same
finding in `udhr_not_used`, and every answer repeats it in `source`.

## Method

Cavnar and Trenkle, "N-Gram-Based Text Categorization" (1994). The text is put in NFC and lower
case, every character that is not a letter becomes a space, and each word is padded with one
space on each side (`" de "`). Its 1-, 2- and 3-grams are counted and ranked by frequency (ties
in code point order), keeping the top 500. For each language profile (its 500 most frequent
n-grams, ranked the same way) the out-of-place distance is the sum over the text's n-grams of
|rank in text − rank in profile|, or 500 when the n-gram is not in the profile, divided by
500 × the number of n-grams: 0 means the same ranking, 1 nothing in common. `score` is
1 − distance; `margin` is the gap between the best two distances.

The answer is `unknown` (with a `reason`) when the text has fewer than 3 letters, when none of
its n-grams is in any profile (another script), when the best distance is above `max_distance`
(0.6 by default: the profiled languages score 0.39–0.57 on the test sentences, Vietnamese 0.65),
or when the best two are closer than `min_margin` (0.01 by default). `reliable` is true only for
a named language and a text of at least 30 letters. `languages` restricts the candidates, `top_k`
sets how many are listed.

## Profiles

`profiles.json` is written by `tools/catalog/data/language_detect.py`, which fetched Unicode
CLDR 48.2 (tag `release-48-2`) on 2026-10-18: for each language `common/main/<lang>.xml` (the
names of languages, territories, scripts, months, weekdays, calendar fields, units, keys and
types and the other labels; patterns, symbols, exemplar character sets, number formats, time
zone city names and person-name samples are left out) and `common/annotations/<lang>.xml` (emoji
names and keywords). That is 109,000–180,000 letters of text per language. The file holds only
the 500 n-gram counts per language and the size of each corpus, never the text. Norwegian is
CLDR's `no` locale, which is Bokmål.

## Tests

The known-value tests use text from neither CLDR nor the UDHR: the first sentence of the
Wikipedia article on each capital, fetched on 2026-10-18 from the REST summary API (for example
`https://es.wikipedia.org/api/rest_v1/page/summary/Madrid`), each test naming the article and its
revision: Madrid (es), London (en), Paris (fr), Berlin (de), Lisboa (pt), Roma (it), and also
Barcelona (ca), Stockholm (sv), Helsinki (fi), Москва (ru), Ελλάδα (el), İstanbul (tr), Praha
(cs) and Budapest (hu). All are identified correctly. Others check the `unknown` cases (Japanese,
Vietnamese, a short Spanish phrase that is as close to Catalan, two letters), the `languages`
restriction and the usage errors.

Not in the tests because the tool gets it wrong: the first sentence of the Norwegian article on
Oslo comes out as Danish (distance 0.437 against 0.467). Of the twenty capitals' first
sentences, nineteen are right.

## Limits

The CLDR corpus is labels and emoji keywords rather than running prose, so function words are
under-represented; the margin between Spanish and Catalan on the Madrid sentence is only 0.015.
Expect errors below a sentence or two, for Danish and Norwegian, and for any language outside the
twenty (Swahili or Esperanto are matched to the nearest profile when they are not far enough
away). Mixed-language texts get one label. Digits and punctuation are ignored.

## Unicode licence notice

The profiles are derived from Unicode CLDR data files used under the Unicode License v3, whose
notice `profiles.json` carries in `unicode_license_notice`: Copyright © 2004-2026 Unicode, Inc.
Permission is hereby granted, free of charge, to any person obtaining a copy of data files and
any associated documentation (the "Data Files") or software and any associated documentation (the
"Software") to deal in the Data Files or Software without restriction, including without
limitation the rights to use, copy, modify, merge, publish, distribute, and/or sell copies of the
Data Files or Software, and to permit persons to whom the Data Files or Software are furnished to
do so, provided that either (a) this copyright and permission notice appear with all copies of
the Data Files or Software, or (b) this copyright and permission notice appear in associated
Documentation. THE DATA FILES AND SOFTWARE ARE PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND.
