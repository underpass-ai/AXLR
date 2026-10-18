# AXLR catalog

The source of the official catalog of pre-forged tools, designed in the [pre-forged tools and downloadable ceremonies design](../docs/plans/2026-10-16-tool-registry-design.md). Its 159 tools are published through the registry (the sibling project [axlr-registry](https://github.com/underpass-ai/axlr-registry)), which tests each version in a sandbox and serves the approved ones in the signed index `https://registry.underpassai.com/index.json` that the console installs from. The [tool catalog](../docs/plans/2026-10-16-prebuilt-tools-catalog.md#status) lists each tool with its version. The ceremonies here are reference definitions: the console does not install ceremonies from a catalog yet.

- `tools/<category>/<name>/`: `manifest.json` (a forged tool plus a `catalog` block), the tool's files, `tests.json` and `README.md`. Run the tests with `python3 tools/catalog/run_tests.py`.
- `ceremonies/<domain>/<name>-<version>.yaml`: MADE definitions the generic `/ceremony` loop can drive; validate and walk them with `python3 tools/catalog/walk_ceremonies.py --made-bin <made-mcp>` (a disposable store; no user store is touched).
- `index.json`: built by `python3 tools/catalog/build_index.py`; `--check` verifies it is current.

Nothing here is registered in a console by being checked out: a tool becomes a tool only through the forge or install card, as the [console documentation](../docs/console.md#forged-tools) describes.
