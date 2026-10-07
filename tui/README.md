# AXLR console module

`tui/` contains the interactive `axlr-tui` application. It is a separate Go module that uses the root AXLR runtime from this checkout. The console streams OpenRouter responses, reviews tool calls, connects explicitly selected MCP servers and saves resumable local sessions.

## Build and start

From the repository root, with Go 1.26:

```bash
GOWORK=off go -C tui test ./...
GOWORK=off CGO_ENABLED=0 go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
/tmp/axlr-tui --root "$PWD"
```

Supply `OPENROUTER_API_KEY` through the host environment before launch, or configure [local models](../docs/console.md#local-models). Type `/model` to select a model, then send a prompt. `--model provider/model` selects one directly; `--lang es` changes interface labels to Spanish. `F1` opens the keyboard guide.

## Read next

| Task | Guide |
|:--|:--|
| Product responsibilities and daily workflow | [Product contract](../docs/product.md), [agent workflow](../docs/runbooks/agent-workflow.md) |
| First launch and prerequisites | [Getting started](../docs/getting-started.md) |
| Models, work modes, approvals, sessions and diagnostics | [Console](../docs/console.md) |
| AXLR packages, MCP registration and approval | [Plugins and MCP](../docs/plugins.md) |
| KMP memory and MADE orchestration | [KMP runbook](../docs/runbooks/kmp.md), [MADE runbook](../docs/runbooks/made.md) |
| Startup or session problems | [Troubleshooting](../docs/troubleshooting.md) |
| Package responsibilities and execution boundary | [Architecture](../docs/architecture.md) |

The console uses your account's OS authority. The release matrix targets Linux, macOS and Windows on amd64 and arm64; the commands above use a POSIX shell. It is not a sandbox. Private sessions and diagnostics can contain conversation content; [the console guide](../docs/console.md#diagnostics-and-privacy) explains their storage and capture controls.
