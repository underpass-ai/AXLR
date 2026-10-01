# Documentation audit — 30 September 2026

This audit began against AXLR at `8faf100` and was reconciled with `08f617a`, which replaced the Codex-owned `/plugin` flow with AXLR-owned package installation. KMP and MADE provided structural references: a clear promise, a quick start, a route table and separate guides for detailed contracts. AXLR's current claims were checked against its own code.

| Finding | Evidence in the prior docs | Resolution |
|:--|:--|:--|
| The first task was hard to find | The root README opened with the JSON worker; the interactive console appeared later | The README now starts with the console and a tested launch path |
| No documentation map | Current guides, historical plans, research and diagnostics shared `docs/` without an index | [Documentation home](index.md) separates current guides from dated records |
| One TUI page mixed several jobs | `tui/README.md` contained controls, models, MCP setup, sessions, diagnostics and developer gates | Separate console, plugin, troubleshooting and architecture pages; the module README points to them |
| MCP transport wording had aged | The root README described external plugins as stdio only | The plugin guide documents stdio and Streamable HTTP from `plugins/manifest.go` |
| Package installation and server connection were easy to conflate | `/plugin` changed ownership after the first audit: it now installs into AXLR and can register MCP servers | The plugin guide describes AXLR-owned packages, generated manual MCP registration and `/mcp` as the connection view |
| Codex plugin compatibility was undersold | The guide mentioned a compatible manifest without stating the intended reuse path | The plugin guide now maps Codex package components to AXLR behavior and gives a concrete install-and-verify route |
| Product responsibility was underspecified | Generic "local execution runtime" wording hid which layer owns memory and orchestration | The README and architecture identify AXLR execution, KMP memory and MADE orchestration without claiming their engines are automatically connected |
| Engine setup/removal had no safe path | A built-in `/plugin` row could be mistaken for an active engine | Separate [KMP](runbooks/kmp.md) and [MADE](runbooks/made.md) runbooks describe connection, verification and disconnection while preserving stores |
| HTTP service and cross-platform releases were aspirations, not current features | The worker is a one-request stdin/stdout process; CI runs on Linux only | The [service spec](specs/axlr-service-api.md) and [implementation plan](plans/axlr-service-helm-release.md) mark planned API, Helm and release work explicitly |
| Operational behavior was buried | Session recovery, private payload captures and execution authority were below long feature descriptions | The quick start and console guide put those facts beside the action they affect |
| No shared visual entry point | AXLR had no logo in the repository | Added the project-supplied pixel-art wordmark, documented in [Brand](brand.md) |

Historical design notes remain available as records of decisions. Current behavior should be checked in the [current guides](index.md) and, for exact schemas and limits, in the running binary and source.

## Review — 1 October 2026

| Finding | Evidence | Correction |
|:--|:--|:--|
| Logos had no reproducible shared source | AXLR was a flattened raster; KMP and MADE had separate SVG exports | Added the [pixel renderer](../tools/pixelart/README.md), editable terminal-art definitions, six transparent exports and a CI drift check. Before recoloring, MADE's generated lettering matched the current upstream `made-cuatro-voces.png` pixel for pixel at 2× scale. The later palette change preserves that geometry. |
| Product palettes changed after the shared exports | The user supplied four distinct copper, gold, brown and sand colors per product | Applied the exact [product palettes](assets/brand/README.md) to lettering and bars, added per-logo bar color configuration and regenerated the documentation, website and GitHub profile assets. |
| The bars should form a square | The user requested vertical stacking after the palette update | Added the reusable square layout: four slanted bars stacked with transparent gaps, aligned below each wordmark's right edge. Updated export dimensions and website image metadata; lettering and palettes are unchanged. |
| One-launch KMP setup could select another store | The example passed a manifest without its configured `KMP_MCP_DATA_DIR` | The runbook now forwards an explicit absolute store for that launch and pins embedded mode in persistent configuration. |
| The quick start understated saved approvals | `tui/application/automatic_tool_approval_test.go` covers exact-tool grants and full autonomy | The first-run guide now names those saved policies and `/approvals`. |
| Symlink wording overstated the read restriction | `FileAdapter.Read` opens through `os.Root`; `Load` separately rejects symlinks for editing | The worker guide distinguishes an in-root read target from an escape and a mutable regular file. |
| Loopback probes were not connected to a Kubernetes probe mechanism | A kubelet `httpGet` targets the pod IP, not container loopback | The plan specifies the service's native `probe` subcommand and exec probes; readiness cannot generate paid completions or mutate engine state. |
| Six-platform releases lacked a prerequisite portability step | Unix process groups, `Flock`, `O_NOFOLLOW` and shell fixtures appear in untagged code | The plan adds native process, private-file and lock adapters before enabling Windows and the release matrix. |
| Some proposed HTTP states and preconditions were ambiguous | `uncertain` is not a current worker status; conflicting revisions/cursors had no rule | The plan separates service states and fixes revision, cursor and error-code conventions. |
| MADE's public status lagged its release | GitHub Release and Cargo both publish `made-mcp` 0.9.1 | The runbook and public surfaces now identify 0.9.1 while linking the upstream setup contract. |
