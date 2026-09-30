# AXLR console module

`tui/` contains the interactive `axlr-tui` application. It is a separate Go module that uses the root AXLR runtime from this checkout. The console streams OpenRouter responses, reviews tool calls, connects explicitly selected MCP servers and saves resumable local sessions.

## Build and start

From the repository root, with Go 1.26:

```bash
GOWORK=off go -C tui test ./...
GOWORK=off CGO_ENABLED=0 go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
/tmp/axlr-tui --root "$PWD"
```

Supply `OPENROUTER_API_KEY` through the host environment before launch. Type `/model` to select a model, then send a prompt. `--model provider/model` selects one directly; `--lang es` changes interface labels to Spanish. `F1` opens the keyboard guide.

## Read next

| Task | Guide |
|:--|:--|
| First launch and prerequisites | [Getting started](../docs/getting-started.md) |
| Models, controls, sessions, themes and diagnostics | [Console](../docs/console.md) |
| Codex packages, MCP registration and approval | [Plugins and MCP](../docs/plugins.md) |
| Startup or session problems | [Troubleshooting](../docs/troubleshooting.md) |
| Package responsibilities and execution boundary | [Architecture](../docs/architecture.md) |

The console targets trusted-local Linux and uses your account's OS authority. It is not a sandbox. Private sessions and diagnostics can contain conversation content; [the console guide](../docs/console.md#diagnostics-and-privacy) explains their storage and capture controls.
