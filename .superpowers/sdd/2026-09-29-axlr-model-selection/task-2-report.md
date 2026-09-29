# Task 2 report: OpenRouter model catalog adapter

## Delivered

- Added `ModelCatalog{APIKey, HTTPClient}.List(ctx)` in the TUI adapter. It sends one bearer-authenticated GET to `/api/v1/models` with `supported_parameters=tools` and `output_modalities=text`, never follows redirects, applies a 10-second context deadline, and caps the response at 8 MiB.
- Added a response mapper that validates the envelope and each item's ID, name, tool support, and text output. Invalid items are skipped. Invalid optional context or pricing fields are treated as unknown while retaining a valid model. Errors omit provider bodies, transport detail, and keys.
- Added HTTP fixture tests; no live provider call or completion request occurs.

## TDD evidence

**RED:** After writing `model_catalog_test.go` and before production code, `go -C tui test ./adapters/openrouter -count=1` failed to compile with seven `undefined: ModelCatalog` errors from the new test file. This was the expected missing adapter symbol.

**GREEN:** After adding the adapter and DTO mapping, the same focused command passed: `ok .../tui/adapters/openrouter 10.016s`.

**Race gate:** `go -C tui test -race ./adapters/openrouter ./application -count=1` passed both packages (`11.096s`, `1.017s`).

**Full TUI suite:** `go -C tui test ./... -count=1` passed all tested packages. `tui/dto` has no test files.

## Self-review

- Confirmed only the three adapter files are staged for the implementation commit; the concurrently edited root `README.md` belongs to another task.
- Reviewed request construction, cancellation, body closure, size boundary, per-entry validation, optional metadata handling, and error redaction. The adapter does not call the completion endpoint.
- The timeout fixture waits for the real ten-second deadline, which makes each adapter test command take about ten seconds. This checks the timeout behavior rather than a shortened test-only setting.
- The API may add metadata fields; unknown fields are ignored. Required capability fields must retain the documented array shape for an entry to appear.

## Source

OpenRouter [models endpoint documentation](https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties) for the wire shape and query parameter meanings.
