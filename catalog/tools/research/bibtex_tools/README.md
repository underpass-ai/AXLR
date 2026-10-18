# bibtex_tools

Parses a BibTeX file, removes duplicate entries, renames the keys to one style, reports the
required fields each entry lacks, and writes the file back as clean BibTeX. `operations`
chooses among `dedupe`, `normalize_keys` and `check` (applied in that order; all three by
default, `[]` only parses and rewrites).

## Parsing

As BibTeX reads a `.bib` file: everything outside an `@` entry is a comment; entries are
`@type{key, field = value, ...}` or `@type(key, ...)`; type and field names are
case-insensitive (written back in lower case); a value is a braced string (nested braces kept
verbatim), a double-quoted string (a `"` inside braces does not end it), a bare number or an
`@string` macro, and parts are joined with `#`. The twelve month macros `jan`…`dec` are
predefined. `@comment` is skipped, `@preamble` is kept. A repeated field keeps the first value;
an undefined macro is kept as its name; an entry that does not parse is skipped with a warning
naming its line, as BibTeX does, and the rest of the file is still read.

## Duplicates

Two entries are the same work when their DOIs are equal after removing `https://doi.org/`,
`dx.doi.org` and `doi:` and lower-casing; otherwise when their titles are equal after removing
LaTeX accents and commands, braces, case and every non-alphanumeric character, and their years
are equal, unless both carry different DOIs. The first entry is kept; with `merge` (default) it
receives the fields it lacks from the removed one. `duplicates` lists each removal with its
reason, using the input keys.

## Keys

`lastnameYEARword`, lower-case ASCII: the last name of the first author (or editor) without the
"von" part (`Ludwig van Beethoven` and `van Beethoven, Ludwig` give `beethoven`), LaTeX accents
folded (`M{\"u}ller` gives `muller`); the first four-digit year (`nd` without one); the first
title word that is not an article, preposition or similar short word. Without an author or
editor the name is `anon`. When several entries get the same key, all of them get a suffix
`a`, `b`, … in file order (`smith2020deepa`, `smith2020deepb`), as in author-year citations.

## Required fields

The table of btxdoc (Oren Patashnik, "BibTeXing", 1988, section 3.1) for the 14 standard
entry types: `article` author, title, journal, year; `book` author or editor, title,
publisher, year; `booklet` title; `inbook` author or editor, title, chapter or pages,
publisher, year; `incollection` author, title, booktitle, publisher, year; `inproceedings` and
`conference` author, title, booktitle, year; `manual` title; `mastersthesis` and `phdthesis`
author, title, school, year; `misc` none; `proceedings` title, year; `techreport` author,
title, institution, year; `unpublished` author, title, note. An empty field counts as missing.
Other types (biblatex's `@online`, `@thesis`…) are noted and not checked.

## Output

`bibtex` is the rewritten file: the preamble, then each entry with two-space indented
`field = {value},` lines in the original order, macros expanded, and `month` written as the
macro (`month = mar,`) when its value is a month name, abbreviation or number. `@string`
definitions are not written back because their uses are expanded. `entries` (optional) gives
each entry as JSON with its type, key, original key, line and fields.

## Tests

The btxdoc required-field table (one entry missing each kind of field), a file with `@string`,
`#`, month macros, nested braces and a quoted value parsed and written back, DOI and title
deduplication with merging, key collisions, a broken entry and the input errors.

## Limits

No crossref inheritance (an entry using `crossref` may be reported as missing fields it would
inherit), no biblatex data model, no reformatting of names or titles beyond the whitespace of the
layout, and comments between entries are not preserved.
