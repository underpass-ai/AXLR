# Getting started

AXLR is built from source today. You need a full checkout, Go 1.26, Linux, a terminal at least 50 × 15 cells, and an OpenRouter API key for the interactive console. The JSON worker and local library tests do not need a model account.

## 1. Build

From the repository root:

```bash
GOWORK=off go test ./...
GOWORK=off go -C tui test ./...
GOWORK=off CGO_ENABLED=0 go build -trimpath -o /tmp/axlr ./cmd/axlr
GOWORK=off CGO_ENABLED=0 go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
```

The root and `tui/` directories are separate Go modules. The checked-in workspace and TUI replace directive let the console build from this repository.

AXLR can start without KMP or MADE, but its intended product layers are KMP-governed memory and MADE-governed orchestration. Connect them with the [KMP runbook](runbooks/kmp.md) and [MADE runbook](runbooks/made.md), then confirm both in `/mcp`. Their built-in `/plugin` catalogue entries do not establish a connection.

## 2. Start a console

Provide `OPENROUTER_API_KEY` through your shell's environment or secret manager, then run:

```bash
/tmp/axlr-tui --root "$PWD"
```

Without a saved model, the console waits for you to choose one; startup alone does not send a model request or create a session. Type `/model`, choose a tool-capable text model, then send a prompt. Your account must have access to the selected OpenRouter model.

You can edit model, language and appearance in `$XDG_CONFIG_HOME/axlr/settings.json` (or `$HOME/.config/axlr/settings.json`). See the [settings format](console.md#edit-settings-as-json). The console reads changes on its next launch.

Useful launch options:

| Option | Effect |
|:--|:--|
| `--root /absolute/workspace` | Choose an existing workspace; default is the current directory |
| `--model provider/model` | Select a model for this launch without opening the catalog |
| `--lang es` | Show Spanish interface labels; `AXLR_LANG=es` also works |
| `--session ID` | Restore a saved session in the same workspace |
| `--trace-payloads=false` | Keep timing diagnostics without saved HTTP bodies |

Press `F1` for keyboard help. `Ctrl+P` opens the action palette. AXLR asks before local tool calls unless a connected MCP server has an explicit automatic approval policy. Review the target and arguments shown in the approval dialog.

## 3. Try the one-request worker

The worker is useful even without an API key:

```bash
printf '%s\n' '{"protocol_version":1,"request_id":"hello-1","tool":"read","arguments":{"path":"README.md"}}' |
  /tmp/axlr --root "$PWD" --profile trusted-local
```

It reads one JSON document from stdin and writes one JSON response to stdout. The [worker contract](worker.md) covers every tool, limit and status. The [Go library](library.md) covers hosts that need a long-lived executor or model client.

## Where data goes

The console sends model requests to OpenRouter and saves sessions under `$XDG_STATE_HOME/axlr/sessions`, falling back to `$HOME/.local/state/axlr/sessions`. Diagnostics use a private `logs/` directory in the same state base. HTTP request and response payload capture is enabled by default and contains conversation content, with credentials redacted; set `--trace-payloads=false` if you only need timings. [Console guide](console.md#diagnostics-and-privacy) has the full behavior.

Local tools act on the chosen workspace. `exec` and MCP servers use host account permissions, so run AXLR in an account and workspace you trust.
