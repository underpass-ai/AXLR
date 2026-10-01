# AXLR documentation

AXLR has three entry points. Start with the console if you want to work interactively; use the JSON worker if a host needs one local operation per process; use the Go library if the host owns the agent loop.

| Entry point | First page | Deeper reference |
|:--|:--|:--|
| Interactive console | [Getting started](getting-started.md) | [Console](console.md), [plugins and MCP](plugins.md), [troubleshooting](troubleshooting.md) |
| JSON worker | [Worker contract](worker.md) | [Architecture](architecture.md) |
| Go library | [Go library](library.md) | [MCP client](../mcpclient/README.md), [architecture](architecture.md) |

## Concepts

- [Architecture and boundaries](architecture.md) explains which component owns execution, model calls, policy and persistence.
- [Plugins and MCP](plugins.md) explains Codex package compatibility, AXLR-managed installation, active MCP connections and both standard transports.
- [KMP runbook](runbooks/kmp.md) and [MADE runbook](runbooks/made.md) cover connection, verification and removal of each engine.
- [Release process](releasing.md) describes the tagged build, native test matrix, packages and checksums.
- [Brand assets](brand.md) contains the logo, palette and usage guidance.

## Development record

The documents under [`plans/`](plans/), [`superpowers/specs/`](superpowers/specs/) and [`superpowers/plans/`](superpowers/plans/) are dated design and implementation records. [TUI design studies](design/2026-09-30-tui/README.md), [research](research/2026-09-30-context-policy.md), [diagnostics](diagnostics/2026-09-30-tui-mcp.md) and [provenance](provenance.md) capture the evidence behind decisions. They may describe an earlier implementation stage; the guides above describe the current interface.

[Documentation audit](documentation-audit.md) records the gaps this reorganization addressed.

## Service and deployment

The worker remains a one-request process API. [`axlr-serve`](api.md) implements HTTP `/v1` with mTLS, sessions, SSE and tool calls. The [Helm chart](helm.md) connects to existing KMP and MADE engines. The [service specification](specs/axlr-service-api.md) and [implementation plan](plans/axlr-service-helm-release.md) record the contract and acceptance work; they do not imply that a tagged release has been published.
