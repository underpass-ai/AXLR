# AXLR documentation

AXLR has three entry points. Start with the console if you want to work interactively; use the JSON worker if a host needs one local operation per process; use the Go library if the host owns the agent loop.

| Entry point | First page | Deeper reference |
|:--|:--|:--|
| Interactive console | [Getting started](getting-started.md) | [Console](console.md), [plugins and MCP](plugins.md), [troubleshooting](troubleshooting.md) |
| JSON worker | [Worker contract](worker.md) | [Architecture](architecture.md) |
| Go library | [Go library](library.md) | [MCP client](../mcpclient/README.md), [architecture](architecture.md) |

## Concepts

- [Architecture and boundaries](architecture.md) explains which component owns execution, model calls, policy and persistence.
- [Plugins and MCP](plugins.md) distinguishes AXLR-managed packages from active MCP connections and documents both transport types.
- [KMP runbook](runbooks/kmp.md) and [MADE runbook](runbooks/made.md) cover connection, verification and removal of each engine.
- [Release plan](releasing.md) describes the proposed cross-platform packages and checksums; tagged release CI is not implemented yet.
- [Brand assets](brand.md) contains the logo, palette and usage guidance.

## Development record

The documents under [`plans/`](plans/), [`superpowers/specs/`](superpowers/specs/) and [`superpowers/plans/`](superpowers/plans/) are dated design and implementation records. [TUI design studies](design/2026-09-30-tui/README.md), [research](research/2026-09-30-context-policy.md), [diagnostics](diagnostics/2026-09-30-tui-mcp.md) and [provenance](provenance.md) capture the evidence behind decisions. They may describe an earlier implementation stage; the guides above describe the current interface.

[Documentation audit](documentation-audit.md) records the gaps this reorganization addressed.

## Planned service

The current worker is a one-request process API. The [service API specification](specs/axlr-service-api.md) and [implementation plan](plans/axlr-service-helm-release.md) define a future HTTP API with mTLS, agent sessions, tool execution, remote KMP/MADE and a Helm chart. These documents are plans, not a claim that `axlr-serve` or its chart exists today.
