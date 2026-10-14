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

The first launch, with no `settings.json` yet and no `--model`, `--session` or `--repair`, runs the setup assistant before the console (decision D9 of the [lifecycle integration design](plans/2026-10-13-lifecycle-integration-design.md)). It asks two things at most and decides the rest; press Enter to take the option marked with `*`.

1. **Model.** With `OPENROUTER_API_KEY` set nothing is asked: the lightest recommended model becomes the default (change it with `/model`). Otherwise the assistant looks for OpenAI-compatible servers on this machine's usual ports (8080–8082, 8000, 11434 and 1234) and offers three routes: **OpenRouter** (it asks for a key without echo, checks it against OpenRouter's model list, saves it in `credentials.json`, a private file beside `settings.json` that the variable overrides, and recommends light models able to call tools), a **local server** (pick a model it found or type a base URL; it reads the context window llama.cpp or vLLM report and writes a `local_models` entry), or **skip** (`/model` later).
2. **Jev (TypeSafe), optional.** With `TYPESAFE_API_KEY` set, or a key already saved, Jev is on; otherwise one hidden question, Enter skips. Jev judges final answers, answers `axlr_judge` and settles each turn's memory.
3. **KMP and MADE.** Installed and configured without a question unless both are connected: the official releases are downloaded and verified, MADE's embedded setup creates its store and authorization bootstrap, the console's work grant and the ten pinned definitions are published, and both entries are registered at `auto`, so neither the console's bookkeeping nor the model's reads raise a card. KMP's store is not chosen here: at every launch the console opens the checkout's own `.kernel` inside a git repository, or `$XDG_DATA_HOME/kmp/default` outside one, creating and filling it when needed.

The language comes from `AXLR_LANG` or the locale. The assistant then writes `settings.json` complete, every section present with its value: autonomy on except the [destructive list](configuration.md#approvals), engines shared and required, forged tools on, Jev on when its key exists, the context capped at 75 000 tokens, and the defaults of jobs, plans, ceremonies and trace retention; `max_session_usd` has no default and stays absent.

Run `axlr-tui setup` at any time to repeat it, for example to add a local model or a Jev key. Without a terminal (a script, a service, a pipe) it asks nothing: the key comes from the environment or the model stays unset, and the engines install and configure themselves all the same (decision B12). A console whose settings require the engines (`engines.required`) installs a missing one at launch and refuses to start when that fails.

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
