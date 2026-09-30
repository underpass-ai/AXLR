# AXLR agent console

`axlr-tui` is a streaming OpenRouter console for a local workspace. It offers AXLR read, write, edit and exec tools, explicitly registered MCP plugins, persistent plugin approval policies, transcript search and resumable sessions. It targets trusted-local Linux and runs with your account's OS access.

## Build and start

Use Go 1.26 from this repository checkout:

```bash
go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
# Supply OPENROUTER_API_KEY through your environment or secret manager.
/tmp/axlr-tui
```

`--root` defaults to the current directory. Type `/model` and press Enter to choose a model before sending your first prompt. The searchable catalog lists text models that support tools, with context limits and pricing when available. Arrow keys and PgUp/PgDn navigate, Enter selects, and Esc closes; failed requests offer Retry. The action palette also offers Models and preserves a drafted prompt. The model selected through `/model` becomes the default for future new sessions.

Use `--model your-provider/your-model` to override the default for one launch without fetching the catalog. Bare startup uses the saved default, if one exists; otherwise it does not create a session snapshot or lock file until a model is chosen. Quitting or a catalog failure before the first selection leaves no session. Changing models is allowed only while idle, complete, or interrupted without pending tool calls. Selection applies to subsequent requests and preserves the transcript. If saving the default fails after a session model change, the new session model remains active and the TUI shows a warning.

`OPENROUTER_API_KEY` is required at startup; it is never placed in session snapshots or plugin environments. Starting the console does not make a model request. Submit a prompt to begin. `--help` lists all flags.

The separate module uses the repository's root AXLR library. `go.work` supports development, and `tui/go.mod` has a local `replace` so `GOWORK=off go -C tui build ./cmd/axlr-tui` also works without fetching an unreleased AXLR version. Build from a full repository checkout.

## Controls

| Key | Action |
| --- | --- |
| Enter | Send prompt; run `/model`, `/mcp` or `/plugin` |
| A in `/plugin` | Toggle automatic/manual approval for the selected plugin |
| R in `/mcp` or `/plugin` | Refresh server/tool inventory |
| Shift+Enter | Newline in the prompt |
| A / D | Approve / deny the displayed tool call |
| Esc | Close overlay or cancel the current turn |
| Ctrl+C | Cancel active work; quit when idle |
| Ctrl+R | Explicitly continue an interrupted session |
| Ctrl+F | Search the current session |
| Enter / Shift+Enter in search | Next / previous hit |
| Ctrl+P | Action palette |
| Ctrl+O | Saved sessions; arrows select, Enter opens |
| F1 | Help |
| Tab | Transcript / activity |
| PgUp / PgDn | Scroll |

Mouse controls match the keyboard actions. Tool arguments and exact targets appear before approval; scroll approval details with arrows, PgUp/PgDn or the wheel. Local tools and plugins default to an individual decision. A configured plugin may opt into automatic approval; `/plugin` changes and persists that policy for its exact ID. A turn allows up to 32 tool calls. Unknown calls are rejected. Cancelled or uncertain effects are recorded and never automatically retried.

Use a terminal of at least 50 columns by 15 rows. The transcript occupies the full terminal width; Tab opens the activity view. User and assistant turns are separate, full-width rows with subtle backgrounds selected for the terminal's light or dark theme. KMP tool requests and results use distinct memory rows; a memory indicator appears while KMP executes. Tool rows show compact previews; Actions → Info retains the full saved results in a scrollable view. Before the first text delta, the status shows the elapsed wait for the model. Rows wrap to their content, and the transcript scrolls as turns accumulate. Set `NO_COLOR=1` for monochrome output. The terminal controls the font, including Nerd Fonts.

## Plugins

Register each plugin explicitly with an absolute path to a [version 1 AXLR manifest](../README.md#external-tool-plugins). There is no directory scanning. A manifest specifies an absolute executable, arguments and allowed tool names. `"*"` opts in to every tool advertised by that server; use exact names to restrict it:

```json
{
  "manifest_version": 1,
  "id": "notes",
  "command": "/absolute/path/to/notes-mcp",
  "args": [],
  "allow_tools": ["search"]
}
```

For persistent connections, create `$XDG_CONFIG_HOME/axlr/mcp.json` (or `$HOME/.config/axlr/mcp.json`) with mode `0600`:

```json
{
  "version": 1,
  "plugins": [
    {
      "manifest": "/absolute/path/to/notes.json",
      "name": "Notes",
      "description": "Workspace knowledge",
      "purpose": "memory",
      "approval": "auto",
      "env_from": {"TOKEN": "NOTES_API_TOKEN"}
    }
  ]
}
```

Optional `purpose` is `tools` (default), `memory` or `ceremony`. Optional `approval` is `manual` (default) or `auto`. Automatic approval applies to tools allowed by that plugin's manifest, including future tools if `allow_tools` is `"*"`. Policies are persisted with a process-shared lock and atomic private-file replacement. `/mcp` shows each server and its tool inventory; `/plugin` shows policies and supports changing them. Command-line-only registrations remain manual unless placed in the persistent config.

AXLR loads this file at every start, including launches without `--plugin`. `env_from` copies only named host variables; optional `env` supplies literal values for a plugin. Both maps become that plugin's complete child environment. Use `--mcp-config /absolute/path/config.json` to select another file. The config lists manifests explicitly, so tools from unrelated applications are not silently started.

```bash
/tmp/axlr-tui --root "$PWD" --model 'your-provider/your-model' \
  --plugin /absolute/path/to/notes.json \
  --plugin-env-from 'notes:TOKEN=NOTES_API_TOKEN'
```

`--plugin` and `--plugin-env-from ID:KEY=HOST_ENV_VAR` are repeatable and add one-launch registrations. The latter copies only the selected host variable into that plugin's `KEY`. Values stay out of command-line arguments. Plugin processes receive only explicit selections; local exec starts with an empty environment. Use absolute executable paths. Forwarding `OPENROUTER_API_KEY` to plugins is rejected. Plugin connections start when a turn discovers tools and close when the console exits. A configured server that fails discovery reports an error before the model request.

## Local sessions and recovery

Sessions contain prompts, model output, tool arguments and results: treat them as local user data. They live under `$XDG_STATE_HOME/axlr/sessions`, or `$HOME/.local/state/axlr/sessions` if XDG state home is unset or relative. The default model is stored separately at `$XDG_STATE_HOME/axlr/model-preference.json`, or the equivalent path under `$HOME/.local/state`. Directories are owner-only (0700), snapshots and the preference file are 0600, updates use atomic replacement, and each open session has an exclusive writer lock.

The header identifies the current session. Open another through Ctrl+O, or start with:

```bash
/tmp/axlr-tui --root "$PWD" \
  --session 0123456789abcdef0123456789abcdef
```

The startup workspace must match the saved session. `--session` restores its saved model without fetching the catalog. If you also supply `--model`, it must match the saved model; use `/model` after loading to change it. The picker refuses a different workspace. Restored in-progress sessions appear interrupted and do not execute anything on load. Ctrl+R explicitly continues; pending calls resume under the current plugin policy; local tools and manual plugins still require individual decisions. Partial output is kept as an interrupted draft, outside valid model history. SIGTERM or terminal failure cancels active work and waits for its stable save before releasing resources.

## Diagnostics

Pass `--trace-file /absolute/path/axlr-tui.jsonl` to record startup, model requests, streaming progress, event delivery, rendering dimensions and time, session saves, and completion. The file is owner-only (`0600`). Records contain event names, request byte counts, timings, dimensions and error classes; they omit prompts, responses, model IDs, tool arguments and credentials. The trace is optional and appends to an existing file. `provider_headers.bytes` measures the outgoing request; the first `provider_progress.elapsed_ms` in each stream measures time to first text. Tool completion and render timings distinguish MCP latency from provider waits. See [the measured MCP diagnostic](../docs/diagnostics/2026-09-30-tui-mcp.md).

## Development checks

Run from the repository root:

```bash
gofmt -l .
GOWORK=off go vet ./...
GOWORK=off go test -race -count=1 -coverpkg=./... -coverprofile=coverage-root.out ./...
go -C tui vet ./...
go -C tui test -race -count=1 -coverpkg=./... -coverprofile=coverage-tui.out ./...
go tool cover -func=coverage-root.out
go -C tui tool cover -func=coverage-tui.out
GOWORK=off CGO_ENABLED=0 go build -trimpath -o /tmp/axlr ./cmd/axlr
CGO_ENABLED=0 go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
```

CI enforces aggregate coverage above 80% for each module. Tests use local fixtures and simulated model streams; no live OpenRouter account is required. The root `cmd/axlr` remains the one-request JSON worker.
