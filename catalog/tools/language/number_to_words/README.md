# number_to_words

Writes a number out in words in Spanish, English, French, German or Portuguese, as a cardinal,
an ordinal or a currency amount (for cheques, invoices and contracts). Integers are exact up to
10^36; a decimal string is read digit for digit as given.

## Rules by language

**Spanish** (RAE, *Diccionario panhispánico de dudas* and *Nueva gramática*). `uno` alone,
`un` before a masculine noun (`before_noun`), `una` before a feminine one, also at the end of a
compound: `veintiuno` / `veintiún` / `veintiuna`, `treinta y un`. `cien` for exactly 100,
`ciento` in compounds; `doscientas` agrees with a feminine noun, also before `mil`
(`doscientas veintiuna mil`), but `millón` is masculine (`veintiún millones`). Long scale:
`un millón` (10^6), `mil millones` (10^9), `un billón` (10^12), `trillón` (10^18) up to
`quintillón`. Ordinals: `primero`/`primer`, `tercero`/`tercer`, `undécimo`, `duodécimo`,
`decimotercero`…, `vigésimo primero`, `centésimo`, `ducentésimo` … `noningentésimo`,
`milésimo`, `dosmilésimo`, `millonésimo`, with feminine `-a`; given below 10^9.

**French** (Académie française; Rectifications de l'orthographe of 1990). `vingt et un`,
`soixante et onze`, but `quatre-vingt-un`; `quatre-vingts` and `deux cents` take an *s* when
they end the numeral or come before `millions`/`milliards` (nouns), not before `mille` or
another numeral (`deux cent mille`, `quatre-vingts millions`). `orthography: "1990"` joins all
the numerals with hyphens (`vingt-et-un`, `mille-deux-cent-trente-quatre`) and leaves the
nouns `million` and `milliard` separate. `fr-BE`: `septante`, `nonante`; `fr-CH`: also
`huitante`. Long scale: `million`, `milliard`, `billion`, `billiard`… Ordinals add `-ième`
(`cinquième`, `neuvième`, `vingt et unième`, `quatre-vingtième`), `premier`/`première`.

**German** (Duden). Below a million the number is one word, with `ein` in compounds and `eins`
at the end: `einhunderteins`, `eintausendzweihundertvierunddreißig`. `eine Million`,
`zwei Millionen`, `Milliarde`, `Billion`, `Billiarde`… are separate capitalised nouns. Ordinals:
`erste`, `dritte`, `siebte`, `achte`, `-te` up to 19 and `-ste` from 20 (`zwanzigste`,
`einhundertste`, `einmillionste`), in the base form after the definite article. `before_noun`
gives `ein`/`eine` for 1.

**English**. Short scale (million, billion = 10^9 … decillion). `en-US` writes
`one hundred one`, `en-GB` puts *and* after the hundreds and before a final part below 100
(`one thousand and five`). Ordinals: `first`, `second`, `third`, `fifth`, `eighth`, `ninth`,
`twelfth`, `-ieth`, `-th`.

**Portuguese**. `e` joins hundreds, tens and units, and goes before the last group when it is
below 100 or a round hundred (`mil e cem`, `mil duzentos e trinta e quatro`,
`um milhão e duzentos mil`). `cem`/`cento`, feminine `uma`, `duas`, `duzentas`. `pt-PT`: long
scale (`mil milhões`, `um bilião`) and `dezasseis`, `dezassete`, `dezanove`; `pt-BR`: short
scale (`um bilhão`, `um trilhão`) and `dezesseis`, `dezessete`, `dezenove`. Ordinals up to
1999 (`vigésimo primeiro`, `milésimo nongentésimo nonagésimo nono`).

## Decimals, negatives, currency

A cardinal's fractional part follows `coma` / `point` / `virgule` / `Komma` / `vírgula`,
read digit by digit (default for en and de: `three point one four`) or as a number with its
leading zeros (default for es, fr, pt: `tres coma cero cinco`). Negatives take `menos`,
`minus`, `moins`. Currency amounts are rounded half up to whole cents, and the unit and
subunit names agree in number and gender (`veintiún euros con cincuenta céntimos`,
`vingt et une livres`, `duas libras`, `ein Euro und fünfzig Cent`); Spanish, French and
Portuguese put `de`/`d'` after a round million (`un millón de euros`, `un million d'euros`).
Bundled names: EUR, USD, GBP, CHF, MXN, BRL; give `unit` for any other.

## Tests

The expected words come from the cited rules: RAE's examples (`veintiuno`, `veintiún`,
`veintiuna`, `un millón`, `un billón`, `cien`/`ciento uno`), the Académie's `quatre-vingts`
and `soixante et onze` and the 1990 `soixante-et-onze`, Duden's
`eintausendzweihundertvierunddreißig`, and the British/American *and*.

## Limits

No Italian. Ordinals are the masculine or feminine singular base form (no German declension,
no plural). Portuguese ordinals stop at 1999 and Spanish ones below 10^9. Spanish 21 to 29
ordinals are written in two words (`vigésimo primero`); RAE also accepts `vigesimoprimero`.
Alternatives that the rules allow are not offered (`hundert` for `einhundert`,
`quatorze` in Brazil, `setuagésimo`). Gender before `mil` follows `gender` (RAE accepts both
`veintiún mil` and `veintiuna mil` with a feminine noun).
