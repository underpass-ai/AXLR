# Documentation audit — 30 September 2026

This audit compares the prior AXLR public README and package guides with the source at `8faf100`, the latest local AXLR implementation when this documentation branch began. KMP and MADE provided structural references: a clear promise, a quick start, a route table and separate guides for detailed contracts. AXLR's claims were checked against its own code.

| Finding | Evidence in the prior docs | Resolution |
|:--|:--|:--|
| The first task was hard to find | The root README opened with the JSON worker; the interactive console appeared later | The README now starts with the console and a tested launch path |
| No documentation map | Current guides, historical plans, research and diagnostics shared `docs/` without an index | [Documentation home](index.md) separates current guides from dated records |
| One TUI page mixed several jobs | `tui/README.md` contained controls, models, MCP setup, sessions, diagnostics and developer gates | Separate console, plugin, troubleshooting and architecture pages; the module README points to them |
| MCP transport wording had aged | The root README described external plugins as stdio only | The plugin guide documents stdio and Streamable HTTP from `plugins/manifest.go` |
| Package installation and server connection were easy to conflate | `/plugin` and `/mcp` appeared together without a boundary map | The plugin guide explains the two inventories and their independent effects |
| Operational behavior was buried | Session recovery, private payload captures and execution authority were below long feature descriptions | The quick start and console guide put those facts beside the action they affect |
| No shared visual entry point | AXLR had no logo in the repository | Added the project-supplied pixel-art wordmark, documented in [Brand](brand.md) |

Historical design notes remain available as records of decisions. Current behavior should be checked in the [current guides](index.md) and, for exact schemas and limits, in the running binary and source.
