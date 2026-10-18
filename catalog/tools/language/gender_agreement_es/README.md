# gender_agreement_es

Checks that the words of a Spanish noun phrase agree in gender and number, and suggests the
corrected form of the word that does not. **It is a heuristic, not a parser**, and every
answer says so in `heuristic`.

## Method

1. The text is split into phrases at punctuation and at prepositions, conjunctions, pronouns,
   frequent adverbs and a list of frequent verbs. Each phrase is read as determiners
   (articles, demonstratives, possessives, quantifiers, numerals), then content words; the
   first content word that is not a usual prenominal adjective (`buen`, `gran`, `nuevo`,
   `último`…) is the noun and the others are adjectives. A bare adjective after `ser`,
   `estar`, `parecer`, `quedar`, `resultar` or `seguir` is checked against the previous
   phrase's noun (`la casa es blanco`).
2. The noun's gender comes from a lexicon of the nouns the ending does not give: the Greek
   `-ma` masculines (`el problema`, `el sistema`, `el tema`, `el idioma`…), `el día`, `el
   mapa`, `el planeta`, `la mano`, `la foto`, `la moto`, `la flor`, `la labor`, feminine and
   masculine nouns in `-e`, `-l`, `-z`, `-n`, `-s` (`la calle`, `el coche`, `la piel`, `el
   lápiz`, `la luz`, `la imagen`, `el análisis`, `la crisis`), common-gender nouns that take
   the determiner's gender (`el/la estudiante`, `colega`, `testigo`, `-ista`) and nouns used
   in both genders (`el/la mar`, `azúcar`, `arte`). Otherwise from the ending: `-ción`,
   `-sión`, `-dad`, `-tad`, `-tud`, `-umbre`, `-sis`, `-ie`, `-triz`, `-a` feminine; `-o`,
   `-aje`, `-or`, `-ón` masculine. Plurals are reduced to their singular (`luces` → `luz`,
   `canciones` → `canción`); nouns that are the same in both numbers (`crisis`, `lunes`) take
   the determiner's number. `gender_source` tells which rule decided.
3. Feminine nouns beginning with stressed *a* or *ha* (`agua`, `águila`, `alma`, `arma`,
   `área`, `aula`, `hacha`, `hambre`, `ave`…, from the RAE's *Diccionario panhispánico de
   dudas*, s.v. *el*): directly before the noun in the singular the article is `el` (`la agua`
   is an error) and `un`, `algún`, `ningún` are the usual forms (`una águila` is accepted and
   reported as a note); every other determiner and every adjective stays feminine (`esta
   agua`, `mucha hambre`, `el agua fría`); with an adjective in between, `la` returns
   (`la fría agua`), and so does the plural (`las aguas`).
4. Adjectives mark gender in `-o/-a`, `-or/-ora`, `-ón/-ona`, `-és/-esa` and their plurals;
   `-e`, `-l`, `-z`, `-ista`, `mejor`, `mayor`… are the same in both genders and only number is
   checked. Suggestions change the ending: `frío` → `fría`, `trabajador` → `trabajadora`,
   `francés` → `francesa`, `alegre` → `alegres`, `los` → `el`.

## Tests

The known values are the RAE's own examples of the stressed-*a* rule (DPD, s.v. *el*, *este*,
*un*, *hambre*): `el agua fría`, `las aguas`, `la agua` → `el agua`, `el agua frío` →
`fría`, `este agua` → `esta agua`, `mucho hambre` → `mucha`, `la fría agua`, `un águila`;
plus the lexicon exceptions `el problema`, `la mano`, `el día`, and number, common-gender,
`-or`/`-és` and predicate cases checked by hand.

## Limits

Words are classed by lists and endings, not by a dictionary: a verb not in the list can be
taken for an adjective (`los niños corren rápido` is not split at `corren`), a noun with an
unusual ending outside the lexicon gets no gender and is checked for number only, and a
noun used as an adjective (`hombre rana`) is read as an adjective. Coordinated nouns (`el
padre y la madre unidos`), relative clauses, and agreement across sentences are not
handled; only adjectives right after a copula are linked to the subject. Proper names
and acronyms are not exempted from the stressed-*a* rule.
