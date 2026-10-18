# openapi_outline

Reads an OpenAPI 3.0 or 3.1 document, or a Swagger 2.0 one, given inline as JSON or YAML, and
outlines it:

- the spec version, title, API version, first line of the description and servers (2.0:
  `schemes`, `host` and `basePath`);
- every operation under `paths` (and 3.1 `webhooks`): method, path, `operationId`, summary
  (or the first line of the description), tags, `deprecated`;
- its parameters, with the path-level ones merged in (an operation's own parameter with the
  same name and location wins), each with location, whether it is required (path parameters
  always are) and a compact type;
- the request body (3.x `requestBody`; 2.0 `in: body` or `formData` parameters with
  `consumes`) by media type, and the responses by status code with description, media types
  and header names (2.0 `schema` with `produces`);
- the component schemas (3.x `components.schemas`, 2.0 `definitions`) with their required
  properties and property types;
- operations per tag, and warnings: operations without `operationId`, duplicate
  `operationId`s, circular or external references.

`$ref`s are resolved inside the document (JSON Pointers with `~0`, `~1` and percent escapes,
chains followed, cycles stopped). A reference to another file or URL is not fetched; it is
shown as written and listed in the warnings. Types are shown compactly: a referenced schema by
its name (`Pet`), `array<Pet>`, `string(date-time)`, `string enum[a,b]`, `object{id,name}`,
`map<string,Pet>`, `oneOf<Cat|Dog>`.

The output has `outline` (JSON) and `markdown` (an endpoint table, one section per operation
and the schema list); `format` picks one or both.

## YAML

JSON is parsed with the standard library. YAML is read by a built-in parser (no PyYAML) that
covers what OpenAPI files use: block mappings and sequences (including sequences at the same
indentation as their key and `- key: value` items), any root indentation, plain scalars
resolved with the YAML 1.2 core schema (null, booleans, integers, floats; `200:` keys become
strings), single- and double-quoted scalars with escapes and line folding, plain scalars
continued on more-indented lines, literal `|` and folded `>` block scalars with chomping and
indentation indicators, one-line or multi-line flow collections (`[a, b]`, `{k: v}`),
comments and a leading `---`. It refuses, with the line number, what it does not implement:
anchors (`&a`), aliases (`*a`) and merge keys, tags (`!tag`, `!!str`), complex keys (`? `),
directives, several documents in one text, and tabs in indentation. It was checked against
PyYAML on all 23 YAML examples of the OpenAPI Initiative's learn.openapis.org repository (the
only difference: PyYAML turns unquoted timestamps into dates, YAML 1.1 style; here they stay
strings).

## Tests

The OpenAPI Initiative's `petstore.yaml` example (learn.openapis.org, `examples/v3.0`): paths
`/pets` and `/pets/{petId}`, operations `listPets`, `createPets` and `showPetById` with their
parameters, request body and responses, and the `Pet`, `Pets` and `Error` schemas through
`$ref`; the same from `petstore.json`; the Markdown table; the Swagger 2.0
`petstore-expanded.yaml` example; and a YAML document exercising block scalars, flow
collections, path-level parameters and an external `$ref`, plus the refusals.

Limits: no external references; `callbacks`, `links`, `security` and examples are not
outlined; YAML outside the subset is refused rather than guessed.
