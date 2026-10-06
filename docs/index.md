# AXLR documentation

AXLR is Underpass's default engine for agentic execution, supported by KMP memory and MADE procedures and extended through compatible MCP servers and OpenAI/Codex plugin packages. Start with the [product contract](product.md) for responsibilities and the exact capabilities of each interface.

## Start and use

| Task | Guide |
|:--|:--|
| Build and launch the console | [Getting started](getting-started.md) |
| Use models, work modes, approvals, sessions and diagnostics | [Console](console.md) |
| Understand driven and explicitly requested procedures | [Ceremonies](ceremonies.md) |
| Install packages or connect MCP tools | [Plugins and MCP](plugins.md) |
| Resolve a visible failure | [Troubleshooting](troubleshooting.md) |

## Operational runbooks

| Outcome | Runbook |
|:--|:--|
| Take a task from context recovery to verified delivery | [Agent workflow](runbooks/agent-workflow.md) |
| Connect, verify and disconnect memory | [KMP](runbooks/kmp.md) |
| Prepare identity, definitions and tracked execution | [MADE](runbooks/made.md) |
| Qualify, install and remove an extension | [Extensions](runbooks/extensions.md) |
| Update engines or recover interrupted work and backups | [Recovery and maintenance](runbooks/recovery.md) |
| Check service health and reconcile API operations | [Service operations](runbooks/service.md) |
| Keep documentation aligned with a code revision | [Documentation maintenance](runbooks/documentation.md) |

## Integration and deployment

The [HTTP/gRPC/MCP parity ledger](transport-parity.md) maps all 18 service capabilities and their shared request, approval and event contracts.

| Interface | Contract |
|:--|:--|
| HTTP service | [Service API](api.md), [OpenAPI v1](../api/openapi/axlr-v1.yaml) |
| Kubernetes | [Helm deployment](helm.md) |
| One-request process | [JSON worker](worker.md) |
| Go host | [Library](library.md), [MCP client](../mcpclient/README.md) |
| Implementation and distribution | [Architecture](architecture.md), [releasing](releasing.md) |

## Documentation status and design record

The current guides were reconciled with `main` at `559a1b8` on 4 October 2026. The [audit](documentation-audit.md) records source evidence, corrections, validation and remaining product limits. [Brand](brand.md) documents the visual assets.

Documents under [plans](plans/), [design studies](design/2026-09-30-tui/README.md), [superpowers specs](superpowers/specs/) and [superpowers plans](superpowers/plans/) preserve dated design and implementation work. The [service design](specs/axlr-service-api.md), [context research](research/2026-09-30-context-policy.md), [diagnostics](diagnostics/2026-09-30-tui-mcp.md) and [provenance](provenance.md) explain decisions; they are not installation or current API references. Use the current guides above for operations.
