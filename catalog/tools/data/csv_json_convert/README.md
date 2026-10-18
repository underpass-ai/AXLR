# csv_json_convert

Converts between CSV and JSON in both directions, keeping nested structure through dotted
column names.

## CSV to JSON

The delimiter is sniffed (comma, semicolon, tab, pipe) unless given. Each row becomes an
object keyed by the header. With `nest` (the default) a column name is split on `separator`
(`.`): `address.city` becomes `{"address": {"city": …}}`, and a numeric part makes an array,
so `tags.0`, `tags.1` become `{"tags": [ …, … ]}` (gaps are filled with `null`). A column that
is both a value and a parent (`a` and `a.b`) is an error rather than a silent overwrite.

With `infer_types` (the default), cells that are JSON numbers become numbers, `true`/`false`
booleans and `null` null; numbers with a leading zero (`01234`, zip codes, ids) stay text.
Empty cells become `null` (`empty: "null"`), are left out (`"omit"`) or stay `""` (`"string"`).
`format` is `array` (default), `jsonl` (one object per line, in a string) or `keyed` (an object
keyed by `key_column`, like `csvjson --key`).

## JSON to CSV

The input is an array of objects (a single object counts as one row; scalars go into a `value`
column) as `json`, or JSON or JSON Lines text as `json_text`. Nested objects are flattened to
dotted columns and arrays to numbered ones (`arrays: "json"` keeps each array as JSON text in
one cell); columns appear in first-seen order across all rows. `null` and missing values are
empty cells, booleans `true`/`false`.

## Round trips

JSON → CSV → JSON returns the original when every value is a string that does not look like a
number, a number, or a boolean, and arrays have the same length in every row. What is lost:
`null` versus missing versus `""`; numeric-looking strings (`"28001"` comes back as a number);
empty arrays and objects (written as `[]` and `{}`, read back as text). The CSV output carries
these notes in `round_trip_notes`.

## Tests

A round trip worked by hand (two nested records with an object two levels deep and an array,
to CSV with eight dotted columns, and back to the identical JSON), flat csvkit-style
conversion, leading zeros, empty cells, padded arrays, JSON Lines with another separator and
arrays as JSON text, keyed and jsonl output, and the errors.

## Limits

The whole input is held in memory (20 MB); key names containing the separator must use
another separator; CSV dialects other than one-character delimiters with double quotes are
not written.

Column order: JSON objects carry no order, and the registry and the console pass inline objects on
with their keys sorted, so JSON given as `json` gets its columns in name order. Give the JSON as
text (`json_text`) or a file to keep the order it was written in.
