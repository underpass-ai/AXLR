# Documentation audit — 30 September 2026

This audit began against AXLR at `8faf100` and was reconciled with `08f617a`, which replaced the Codex-owned `/plugin` flow with AXLR-owned package installation. KMP and MADE provided structural references: a clear promise, a quick start, a route table and separate guides for detailed contracts. AXLR's current claims were checked against its own code.

| Finding | Evidence in the prior docs | Resolution |
|:--|:--|:--|
| The first task was hard to find | The root README opened with the JSON worker; the interactive console appeared later | The README now starts with the console and a tested launch path |
| No documentation map | Current guides, historical plans, research and diagnostics shared `docs/` without an index | [Documentation home](index.md) separates current guides from dated records |
| One TUI page mixed several jobs | `tui/README.md` contained controls, models, MCP setup, sessions, diagnostics and developer gates | Separate console, plugin, troubleshooting and architecture pages; the module README points to them |
| MCP transport wording had aged | The root README described external plugins as stdio only | The plugin guide documents stdio and Streamable HTTP from `plugins/manifest.go` |
| Package installation and server connection were easy to conflate | `/plugin` changed ownership after the first audit: it now installs into AXLR and can register MCP servers | The plugin guide describes AXLR-owned packages, generated manual MCP registration and `/mcp` as the connection view |
| Product responsibility was underspecified | Generic "local execution runtime" wording hid which layer owns memory and orchestration | The README and architecture identify AXLR execution, KMP memory and MADE orchestration without claiming their engines are automatically connected |
| Engine setup/removal had no safe path | A built-in `/plugin` row could be mistaken for an active engine | Separate [KMP](runbooks/kmp.md) and [MADE](runbooks/made.md) runbooks describe connection, verification and disconnection while preserving stores |
| HTTP service and cross-platform releases were aspirations, not current features | The worker is a one-request stdin/stdout process; CI runs on Linux only | The [service spec](specs/axlr-service-api.md) and [implementation plan](plans/axlr-service-helm-release.md) mark planned API, Helm and release work explicitly |
| Operational behavior was buried | Session recovery, private payload captures and execution authority were below long feature descriptions | The quick start and console guide put those facts beside the action they affect |
| No shared visual entry point | AXLR had no logo in the repository | Added the project-supplied pixel-art wordmark, documented in [Brand](brand.md) |

Historical design notes remain available as records of decisions. Current behavior should be checked in the [current guides](index.md) and, for exact schemas and limits, in the running binary and source.
