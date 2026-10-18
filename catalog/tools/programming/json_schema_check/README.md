# json_schema_check

Validates a JSON instance against a JSON Schema with the rules of draft 2020-12, for the
subset of keywords listed below, and lists every error rather than stopping at the first.
Each error carries `instance_path` (a JSON Pointer into the instance, `""` for the root),
`schema_path` (the keyword location, a JSON Pointer into the schema that goes through `$ref`),
the `keyword` and a message. Schema and instance can be given as JSON values or as JSON text
(`schema_text`, `instance_text`).

Supported keywords:

- any type: `type` (a name or a list), `enum`, `const`, `allOf`, `anyOf`, `oneOf`, `not`,
  `if`/`then`/`else`, `$ref`;
- numbers: `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum` (numbers, as in
  2020-12), `multipleOf` (exact, with fractions, so 0.3 is a multiple of 0.1);
- strings: `minLength`, `maxLength` (in code points), `pattern` (a Python regular expression,
  unanchored, as the spec says), `format`;
- arrays: `prefixItems`, `items` (the items after `prefixItems`), `contains`, `minContains`,
  `maxContains`, `minItems`, `maxItems`, `uniqueItems`;
- objects: `properties`, `patternProperties`, `additionalProperties` (boolean or schema),
  `required`, `dependentRequired`, `propertyNames`, `minProperties`, `maxProperties`.

Types follow the data model of the spec: an integer is any number with a zero fractional
part (`1.0` is an integer), booleans are never numbers, and `enum`, `const` and
`uniqueItems` compare with JSON equality (`1` equals `1.0`, `true` does not equal `1`).

`$ref` resolves JSON Pointers inside the same document (`#`, `#/$defs/…`, `#/definitions/…`,
with `~0`/`~1` and percent escapes); a `$ref` that loops without consuming the instance is an
error. A `$ref` to another document or to an anchor is not fetched: it is listed under
`unsupported` and the rest is still checked.

`format` checks `date`, `date-time`, `time` (RFC 3339), `email`, `uri`, `uuid` and `ipv4`.
In 2020-12 `format` is an annotation, so by default a failure is a warning; `formats:
"assert"` turns it into an error. Other format names give a warning that they were not
checked.

Keywords outside the subset are never ignored silently: known 2020-12 keywords the tool does
not implement (`unevaluatedProperties`, `unevaluatedItems`, `dependentSchemas`,
`$dynamicRef`, `$anchor`, the content keywords, the draft 2019-09 array form of `items`,
draft 4's boolean `exclusiveMinimum`…) go to `unsupported` and make `complete` false, and
unknown keywords go to `unknown_keywords` (the spec treats them as annotations). A `$schema`
naming another draft gives a warning. Annotations (`title`, `description`, `default`,
`examples`, `$comment`, `$id`…) are accepted.

The tests use the product schema of json-schema.org's "Getting started" guide: its valid
instance passes, its invalid instance (no `productName`, `price` as the string `"100"`)
gives exactly those two errors, the final version with `dimensions` checks the nested
`required`, and its external `$ref` for `warehouseLocation` is reported as unsupported. Also
the spec's integer rule, local `$ref`, `prefixItems`, `oneOf`, `patternProperties`,
`additionalProperties`, formats and the unsupported-keyword report.

Limits: no remote references, `$id`-based resolution or anchors; no `unevaluated*`; the
`email` and `uri` checks are syntactic approximations of RFC 5321 and RFC 3986; for the full
specification use the `jsonschema` package (a tier 2 tool). Validation stops after 10 s.
