# AXLR catalog (reference entries)

Reference content for the [pre-forged tools and downloadable ceremonies design](../docs/plans/2026-10-16-tool-registry-design.md). It proves the manifest shapes; the catalog itself is meant to live in its own repository and be served as a static, signed site.

- `tools/<category>/<name>/`: `manifest.json` (a forged tool plus a `catalog` block), the tool's files, `tests.json` and `README.md`. Run the tests with `python3 tools/catalog/run_tests.py`.
- `ceremonies/<domain>/<name>-<version>.yaml`: MADE definitions the generic `/ceremony` loop can drive; validate and walk them with `python3 tools/catalog/walk_ceremonies.py --made-bin <made-mcp>` (a disposable store; no user store is touched).
- `index.json`: built by `python3 tools/catalog/build_index.py`; `--check` verifies it is current.

Nothing here is registered in a console by being checked out: a tool becomes a tool only through the forge or install card, as the [console documentation](../docs/console.md#forged-tools) describes.
