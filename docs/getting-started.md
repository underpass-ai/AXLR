# Getting started

This guide builds AXLR from a full checkout with Go 1.26. You need a terminal at least 50 × 15 cells and, for the interactive console, an OpenRouter API key or a [local OpenAI-compatible server](console.md#local-models). The commands below use a POSIX shell; the release workflow also targets macOS and Windows on amd64 and arm64. On Windows, use native executable paths and PowerShell environment syntax, and supply an absolute `HOME` or the required XDG directories. The JSON worker and local library tests do not need a model account.

## 1. Build

From the repository root:

```bash
GOWORK=off go test ./...
GOWORK=off go -C tui test ./...
GOWORK=off CGO_ENABLED=0 go build -trimpath -o /tmp/axlr ./cmd/axlr
GOWORK=off CGO_ENABLED=0 go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
```

The root and `tui/` directories are separate Go modules. The checked-in workspace and TUI replace directive let the console build from this repository.

AXLR is the default execution entry point in the [product stack](product.md). KMP gives it memory and MADE tracked procedures; the setup assistant below installs and connects both, and `/mcp` confirms them. Their built-in `/plugin` catalogue entries do not establish a connection. The console can start without either engine; `/debug` and `/delivery` require prepared MADE definitions.

## 2. Start a console

```bash
/tmp/axlr-tui --root "$PWD"
```

The first launch in a terminal, with no `settings.json` yet and no `--model`, `--session` or `--repair`, runs the setup assistant before the console. Press Enter to take the option marked with `*`.

1. **Language.** English or Spanish, starting from `AXLR_LANG` or the locale.
2. **Model.** The assistant looks for OpenAI-compatible servers on this machine's usual ports (8080–8082, 8000, 11434 and 1234) and offers three routes:
   - **OpenRouter.** It uses `OPENROUTER_API_KEY` when set; otherwise it asks for a key, without echo, checks it against OpenRouter's model list and saves it in `credentials.json`, a private file beside `settings.json` that the variable overrides. It then recommends light models: fast, cheap and able to call tools, with their window and price. GLM 5.3 Flash and Claude Haiku 5.5 come first when available, then the cheapest flash, mini, lite or haiku models with at least 128k tokens of context.
   - **Local server.** Pick a model it found or type a base URL. It reads the context window llama.cpp or vLLM report, which you confirm, and writes a `local_models` entry.
   - **Skip.** Choose later with `/model`.
3. **KMP and MADE.** Unless both are already connected, it offers to install them. It downloads the official releases, verifies their checksums and versions, and runs MADE's embedded setup with its store and authorization bootstrap. KMP's store is not chosen here: at every launch the console opens the checkout's own `.kernel` inside a git repository, or `$XDG_DATA_HOME/kmp/default` outside one, creates it with the repository's committed memory and the guide when it is new, and says which in its startup line (see the [KMP runbook](runbooks/kmp.md#let-axlr-install-it)). It then adds both to `mcp.json`, issues AXLR's MADE work grant and publishes the driven ceremonies. You choose whether KMP may record memories without a card each time (recommended); `/mcp` changes it later. Official packages exist for Linux x86_64/arm64 and macOS on Apple silicon; elsewhere follow the [KMP](runbooks/kmp.md) and [MADE](runbooks/made.md) runbooks.

Run `axlr-tui setup` at any time to repeat it, for example to add a local model or install the engines you skipped. Without a terminal (a script, a service) the assistant never runs, and a launch without a provider stops with the ways to configure one.

Without a saved model, the console waits for you to choose one; startup alone does not send a model request or create a session. Type `/model`, choose a tool-capable text model, then send a prompt. Your account must have access to the selected OpenRouter model; a local model needs its server running.

You can edit model, language and appearance in `$XDG_CONFIG_HOME/axlr/settings.json` (or `$HOME/.config/axlr/settings.json`). The [configuration reference](configuration.md) describes every key of it, `mcp.json` and `credentials.json`. The console reads changes on its next launch.

Useful launch options:

| Option | Effect |
|:--|:--|
| `--root /absolute/workspace` | Choose an existing workspace; default is the current directory |
| `--model provider/model` | Select a model for this launch without opening the catalog |
| `--lang es` | Show Spanish interface labels; `AXLR_LANG=es` also works |
| `--session ID` | Restore a saved session in the same workspace |
| `--trace-payloads` | Also save redacted HTTP request and response bodies for this launch (off by default; `trace_payloads` in settings turns it on for every launch) |

Press `F1` for keyboard help. `Ctrl+P` opens the action palette. AXLR asks before tool calls by default; saved per-tool choices, a server's automatic policy or full autonomy can approve them automatically. `/approvals` shows the saved choices. Review the target and arguments shown in the approval dialog. New sessions start in `/normal`. Use `/writer` for documentation, `/review` for findings, or `/research` for evidence gathering. For a checked implementation or repair, follow the [agent workflow](runbooks/agent-workflow.md) and choose `/delivery` or `/debug` after preparing MADE.

## 3. Try the one-request worker

The worker is useful even without an API key:

```bash
printf '%s\n' '{"protocol_version":1,"request_id":"hello-1","tool":"read","arguments":{"path":"README.md"}}' |
  /tmp/axlr --root "$PWD" --profile trusted-local
```

It reads one JSON document from stdin and writes one JSON response to stdout. The [worker contract](worker.md) covers every tool, limit and status. The [Go library](library.md) covers hosts that need a long-lived executor or model client.

## Where data goes

The console sends model requests to OpenRouter, or to the configured local server for a local model, and saves sessions under `$XDG_STATE_HOME/axlr/sessions`, falling back to `$HOME/.local/state/axlr/sessions`. Diagnostics use a private `logs/` directory in the same state base. HTTP request and response payload capture is off by default; `--trace-payloads` (or `trace_payloads` in settings) turns it on, and then it contains conversation content, with credentials redacted. [Console guide](console.md#diagnostics-and-privacy) has the full behavior.

Local tools act on the chosen workspace. `exec` and MCP servers use host account permissions, so run AXLR in an account and workspace you trust.
