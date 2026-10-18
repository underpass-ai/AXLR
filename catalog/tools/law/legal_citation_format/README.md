# legal_citation_format

Builds citations from their parts (`system` + `parts`) or reads one (`parse`, the system is
detected) for four systems.

| System | Formats | Reads |
|:--|:--|:--|
| `boe` | `Ley 39/2015, de 1 de octubre, <title>. «BOE» núm. 236, de 2 de octubre de 2015, pp. 89343-89410.`, the ELI `https://www.boe.es/eli/es/<type>/<yyyy>/<mm>/<dd>/<number>` (types `l`, `lo`, `rd`, `rdl`, `rdlg`) and, with `boe_id`, the consolidated and original text links | such a citation, or a `BOE-A-…` identifier |
| `eurlex` | the act's name with the numbering in force for its date, the full citation with author, date, title and Official Journal reference (`OJ L 119, 4.5.2016, p. 1`, or `OJ L, 2023/2854, 22.12.2023` for the act-by-act journal), its CELEX number, EUR-Lex URL and ELI; `language: es` gives `Reglamento (UE)…`, `DO L 119 de 4.5.2016, p. 1` | a CELEX number, or an act name such as `Directive 95/46/EC` |
| `ecli` | builds `ECLI:<country>:<court>:<year>:<ordinal>` | validates any ECLI |
| `us_case` | `Name v. Name, <vol> <reporter> <page>[, <pin>] ([court] <year>)` with a Markdown version that italicises the name | such a citation |

## Rules

- EU numbering: from 1 January 2015 every act is `Type (EU) YYYY/N`; before, regulations are
  `Regulation (EC) No N/YYYY` and directives and decisions `Directive YY/N/EC`, with two-digit
  years before 1999. The domain (EU, EC, EEC) is inferred from the year unless `domain` is
  given (with a warning). The CELEX number of a legal act is `3` + year + `R`/`L`/`D` + the
  number on four digits.
- ECLI (Council conclusions of 29 April 2011, OJ C 127, 29.4.2011, p. 1): five components
  separated by colons: `ECLI`, a two-letter country code (EU Member States, `EU` for the EU
  courts; Greece is `EL`), a court code of up to seven letters or digits, the four-digit year and an
  ordinal of up to 25 letters, digits and dots. ECLIs are case-insensitive; capitals are used.
- US: Supreme Court reporters (`U.S.`, `S. Ct.`, `L. Ed.`) take no court in the
  parenthetical; others should name it, and the tool warns when it is missing.

## Tests

Known values: Ley 39/2015 and its BOE publication data; the GDPR's CELEX number, title and
Official Journal reference as published on EUR-Lex; Directive 95/46/EC and Regulation (EC)
No 1049/2001 to their CELEX numbers; ECLI syntax from the Council conclusions and the Google
Spain judgment's ECLI:EU:C:2014:317; Brown v. Board of Education, 347 U.S. 483 (1954).

## Limits

The tool does not look anything up: BOE-A identifiers, issue numbers and pages must be given.
Bluebook case-name abbreviations (table T6), short forms, regional and parallel citations
are not applied. Orders (`Orden`) get no ELI because theirs includes a department code.
Not legal advice; the output carries that disclaimer.
