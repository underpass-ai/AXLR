# format_convert

Converts configuration and data between JSON, TOML, INI and YAML (a subset). Give `text` with
`from`, or `data` (a JSON value), and `to`. The output has the converted `text` (and `data` when
converting to JSON), `round_trip_ok` (the output read back gives the same data) and
`round_trip_notes` (what the conversion changed or could not keep).

## Formats

- **JSON**: Python's `json` (RFC 8259).
- **TOML**: read with `tomllib` (Python 3.11+; on 3.10 reading TOML is refused with a clear
  error). Dates and times become ISO 8601 strings, `inf` and `nan` strings. Written by this
  program: scalars and inline arrays first, then `[tables]`, then `[[arrays of tables]]` for
  lists of objects, keys quoted when they are not bare (`"google.com"`), floats always with a
  decimal point. TOML has no null, so a null is an error that names its path.
- **INI**: `configparser` with interpolation off, key case kept and a `[DEFAULT]` section read as
  an ordinary section (configparser would otherwise copy its keys into every section). A file
  without a section header is read as `[DEFAULT]`. Every value is a string. Writing needs an
  object of sections whose values are scalars; top-level scalars go to `[DEFAULT]`.
- **YAML subset**, read and written here (YAML 1.2.2): block mappings and sequences (including
  `- key: value` items and sequences at the same indentation as their key), plain scalars
  resolved with the YAML 1.2 core schema (null, `~`, true/false, integers, `0o` and `0x`, floats,
  `.inf`, `.nan`; anything else is a string, so `yes` and `no` are strings, unlike PyYAML's
  YAML 1.1 reading, and a note says so), single- and double-quoted strings with their escapes,
  multi-line plain scalars, literal `|` and folded `>` block scalars with `-` chomping,
  single-line flow collections `[a, b]` and `{k: v}`, comments, one leading `---`. Refused with a
  message: anchors `&` and aliases `*`, tags `!`, a second document, directives, complex keys
  `?`, multi-line flow collections and quoted strings, tabs in indentation. Written in block
  style, with strings double-quoted whenever they could be read as another type.

## Tests

The TOML v1.0.0 example of toml.io (owner, database with an inline table and nested arrays,
servers.alpha and servers.beta, an offset date-time), the specification's array-of-tables
`[[products]]` example (with its empty middle table) and its dotted-keys example; JSON written
to TOML and read back by `tomllib`; a YAML file with every supported construct, whose result was
checked against PyYAML 6 (identical except `yes`, which YAML 1.1 reads as true), written back to
YAML and reread; INI both ways; and the refusals (anchors, tags, a second document, null in
TOML, lists in INI, invalid TOML).

## Limits

The YAML side is a subset, not a full YAML parser: anchors, tags and multi-document streams
are refused, block scalars drop comment-like lines and trailing blank lines (`|+`). TOML local
dates and times lose their type through JSON. Comments are not carried across formats.
