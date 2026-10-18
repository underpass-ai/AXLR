# es_verb_conjugate

Conjugates a Spanish verb after the models of the Real Academia Española (*Nueva gramática de
la lengua española*, 2009, ch. 4 and its conjugation models). It covers the indicative (presente,
pretérito imperfecto, pretérito perfecto simple, futuro, condicional and their five compound
tenses), the subjunctive (presente, both pretérito imperfecto forms in -ra and -se, futuro and
the compound tenses), the affirmative and negative imperative, the infinitive, gerund and
participle, and the compound infinitive and gerund. Tense keys use the RAE names; `tense_names`
gives Bello's names alongside (copretérito, pospretérito, antepresente…). `voseo: true` adds the
vos forms: their own present indicative (`amás`, `tenés`, `sos`) and imperative (`amá`, `tené`);
everywhere else, including the present subjunctive, the tú form is given, as the norm prescribes.

## Method

Regular verbs take the endings of `amar`, `temer` and `partir`, with these rules applied to
every verb:

- spelling before e: c → qu (busqué), g → gu (pagué), z → c (empecé), gu → gü (averigüé);
- spelling before a and o: c → z after a consonant (venzo), -zc- after a vowel (conozco,
  conduzco), g → j (cojo, elijo), gu → g (sigo), qu → c (delinco);
- -uir verbs insert y (construyo, construya), and an unstressed i between vowels becomes y
  (leyó, cayendo, oyeron) while a stressed one takes an accent in hiatus (leíste, caído);
- an unstressed i after ñ or ll is dropped (tañó, riñeron).

`irregulares.json` (171 verbs, written by `tools/catalog/data/es_verb_conjugate.py` from
literal tables) lists only what sets each irregular verb apart; the tool derives the rest:

- stem changes in stressed syllables: e>ie (pensar, entender), o>ue (contar, volver, with
  hue- at the start: huelo), u>ue (jugar), i>ie (adquirir); and for -ir verbs the second
  vowel in the unstressed forms that carry it: e>ie/i (sentir, sintió, sintamos, sintiendo),
  o>ue/u (dormir, durmió), e>i (pedir, pidió);
- an irregular yo present (tengo, hago, quepo) that also gives the present subjunctive
  (tenga, haga, quepa); strong preterites (tuv-, pud-, dij-, conduj-, with -eron after j)
  from which the imperfect and future subjunctive follow (tuviera, tuviese, tuviere);
  irregular future and conditional stems (tendr-, har-, dir-); irregular tú imperatives
  (ten, haz, di, sal, ve, sé), participles (hecho, dicho, puesto, visto, vuelto, escrito,
  abierto, muerto, roto, frito, impreso, provisto) and gerunds (yendo, pudiendo, viniendo);
- written accents on the stressed weak vowel (envío, actúo, reúno, prohíbo, aíslo);
- whole tenses for ser, ir, estar, haber, dar, ver, oír, reír, sonreír, freír and huir;
- derived verbs take the base verb's features with their prefix (mantener, obtener, componer,
  convenir, deshacer, atraer, producir…), with the accented imperatives mantén, propón, convén.

Monosyllables are written without accent as the *Ortografía* of 2010 rules: rio, riais, frio,
hui, huis, dio, vio. A verb not in the list is conjugated as regular with the spelling rules
above, and the output says so in `notes`.

## Tests

The expected forms are the RAE paradigms of `amar`, `temer`, `partir`, `ser`, `ir` and `tener`
(as in the *Nueva gramática*'s conjugation models and the DLE tables), plus pedir, jugar,
conducir, construir, leer, reír (rio, riais), mantener (mantén), enviar and buscar, and three
usage errors.

## Limits

The irregular table was transcribed by hand; the DLE web tables could not be fetched by
script (they answer with a browser challenge), so the forms were not machine-checked against
them. A stem-changing or irregular verb missing from the list comes out regular (for example
`errar`, `rehacer`, `predecir`, `bendecir`, `asir`, `erguir`, `yacer`, `guiar`, `criar`).
Where the RAE accepts two forms only one is given, except the regular participles noted in
`participio_regular` (imprimido, freído, proveído) and `satisfaz`/`satisface`. Pronominal
verbs (levantarse) are refused: conjugate the infinitive and add the pronoun. Defective and
impersonal verbs (soler, llover, nevar) are conjugated in full with a note.
