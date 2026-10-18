# dedupe_sort

Cleans a list of lines: removes duplicates, counts them, and sorts what is left. Give `text`
(split on any line ending; CRLF files are written back with CRLF), `lines` or a relative `path`.

## Comparing lines

Two lines are duplicates when their comparison forms are equal. The form is the line, with
`trim` stripped of surrounding whitespace (the output is trimmed too), with `normalize`
converted to Unicode NFC (é precomposed and e + combining acute become one) or NFKC (also
compatibility characters: ﬁ → fi, full-width letters), with `ignore_accents` stripped of
combining marks after NFD decomposition (café = cafe, Ñ = N), and with `ignore_case`
case-folded (`str.casefold`, so Straße = STRASSE). The first occurrence is kept, in its own
spelling; `counts` gives each kept line with how many input lines it stands for, and
`duplicates` the lines seen more than once, most frequent first. `dedupe: false` keeps every
line. `only` keeps just the duplicated or just the unique lines; `drop_empty` removes blank ones.

## Sorting

`sort: "lexical"` orders by the comparison form's code points (no locale: the result is the
same on every machine), then by the line itself; `"natural"` splits the form into runs of
digits and non-digits and compares digit runs as numbers (file2 < file10, v1.9 < v1.10;
numbers sort before letters at the same position). With `ignore_accents` and `ignore_case`,
"Ángel" sorts among the a's instead of after z. `reverse` reverses; `sort: "none"` keeps input
order.

## Tests

Natural against code-point order for file1, file2, file10, file20; exact dedupe with counts
worked by hand; case folding of Straße; trimming; NFC turning U+0065 U+0301 into U+00E9; an
accent- and case-insensitive natural sort of Spanish names; reverse natural sort of versions;
`only` both ways; CRLF; errors.

## Limits

No language-specific collation (Spanish ñ sorts as n when accents are ignored, after z
otherwise; use a locale-aware tool for dictionary order); negative numbers and decimals are not
recognised as such in natural sort (the digits are compared run by run).
