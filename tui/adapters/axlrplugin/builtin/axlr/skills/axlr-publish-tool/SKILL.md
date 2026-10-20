---
name: axlr-publish-tool
description: Publish a tool forged in this workspace to a catalog's registry (axlr_publish_tool) so others can install it. Use when the person asks to publish, upload or share a forged tool, or to put it in a catalog or the AXLR registry.
---

# Publish a forged tool

The registry takes a tool only in one shape. Prepare it, check it, then call `axlr_publish_tool`. The person approves the upload on a card; an operator of the registry approves the version later.

## 1. The tool's directory

Everything sits at the top of `.axlr/tools/<name>/`, with bare file names (no subdirectories). Forge it again with `axlr_forge_tool` when a file is missing; files written any other way are not published.

| File | What it is |
|:--|:--|
| the program files | what `program` and `args` run (`args` names them by `.axlr/tools/<name>/<file>`) |
| fixtures | any input file a test reads, as text (a PGM image as `P2`, a CSV, JSON) |
| `tests.json` | the registry's test cases, never part of the tool itself |
| `catalog.json` | the catalog block, never part of the tool itself |

Forged files are text. A fixture that is binary cannot be published: write it in a text format, or build the input inside the test.

## 2. tests.json

A JSON list of cases with distinct names. The registry runs each one in a sandbox from the tool's directory, sends `input` on stdin and checks `expect`:

```json
[
  {"name": "counts three words", "input": {"text": "uno dos tres"}, "expect": {"exit": 0, "stdout_json": {"words": 3}}},
  {"name": "rejects a missing text", "input": {}, "expect": {"exit": 2}},
  {"name": "reads the fixture", "input": {"path": "fixture.csv"}, "expect": {"exit": 0, "stdout_contains": "rows"}}
]
```

- `input` is an object, as the tool's `input_schema` describes it.
- `expect` has `exit` and, when it matters, `stdout_json` (exact JSON) or `stdout_contains` (a substring).
- Cover the main result and at least one refusal. A file a case reads must be one of the tool's files.

## 3. catalog.json

```json
{"version": "1.0.0", "category": "data", "tags": ["image", "dct"], "license": "MIT",
 "requires": {"program_min_version": "3.10", "packages": ["numpy", "Pillow"]}, "network": false,
 "example": {"input": {"mode": "forward"}, "output": {"blocks": 1}}, "source": "where the method comes from"}
```

- `version` is major.minor.patch. A published version is never replaced: a change is a new version.
- `category` and `tags` are lowercase words; `license` is an SPDX id.
- `requires.packages` lists every package the program imports beyond the standard library.
- `network` is true only when the tool needs the network.

## 4. Check before publishing

1. Run every case of `tests.json` with `axlr_run_tool` and compare with its `expect`. Fix the tool or the case until all match.
2. The tool's name must fall under a namespace the person's publisher is approved for. When it does not, the registry refuses it: tell the person, who enrols it with `/tools enroll <publisher> <namespace>… --contact <address>`, and an operator approves it.

## 5. Publish

Call `axlr_publish_tool` with `name`, optionally `version` and `catalog`, and a `why`. The console refuses before the card when `tests.json` or a field of `catalog.json` is missing, and says which.

After the upload, tell the person in a few lines: the version, the catalog, the registry's status and any findings. The registry then runs the tests; the person follows the result with `/tools`.

When the registry answers `403 pending`, the publisher is not approved yet; `401 stale`, the clock is off; a namespace refusal, step 4.2. Report these to the person; do not try another way to upload.
