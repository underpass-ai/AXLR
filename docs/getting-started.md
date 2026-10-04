# Getting started

This guide builds AXLR from a full checkout with Go 1.26. You need a terminal at least 50 × 15 cells and an OpenRouter API key for the interactive console. The commands below use a POSIX shell; the release workflow also targets macOS and Windows on amd64 and arm64. On Windows, use native executable paths and PowerShell environment syntax, and supply an absolute `HOME` or the required XDG directories. The JSON worker and local library tests do not need a model account.

## 1. Build

From the repository root:

```bash
GOWORK=off go test ./...
GOWORK=off go -C tui test ./...
GOWORK=off CGO_ENABLED=0 go build -trimpath -o /tmp/axlr ./cmd/axlr
GOWORK=off CGO_ENABLED=0 go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
```

The root and `tui/` directories are separate Go modules. The checked-in workspace and TUI replace directive let the console build from this repository.

AXLR is the default execution entry point in the [product stack](product.md). Connect [KMP](runbooks/kmp.md) for memory and [MADE](runbooks/made.md) for tracked procedures, then confirm both in `/mcp`. Their built-in `/plugin` catalogue entries do not establish a connection. The console can start without either engine; `/debug` and `/delivery` require prepared MADE definitions.

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

Press `F1` for keyboard help. `Ctrl+P` opens the action palette. AXLR asks before tool calls by default; saved per-tool choices, a server's automatic policy or full autonomy can approve them automatically. `/approvals` shows the saved choices. Review the target and arguments shown in the approval dialog. New sessions start in `/normal`. Use `/writer` for documentation, `/review` for findings, or `/research` for evidence gathering. For a checked implementation or repair, follow the [agent workflow](runbooks/agent-workflow.md) and choose `/delivery` or `/debug` after preparing MADE.

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
