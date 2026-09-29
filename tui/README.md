# AXLR agent console

`axlr-tui` is a streaming OpenRouter console for a local workspace. It offers AXLR read, write, edit and exec tools, explicitly registered MCP plugins, per-call approval, transcript search and resumable sessions. It targets trusted-local Linux and runs with your account's OS access.

## Build and start

Use Go 1.26 from this repository checkout:

```bash
go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
# Supply OPENROUTER_API_KEY through your environment or secret manager.
/tmp/axlr-tui --root "$PWD" --model 'your-provider/your-model'
```

Both `--root` and `--model` are required. Choose an OpenRouter model that supports tools. `OPENROUTER_API_KEY` is required at startup; it is never placed in session snapshots or plugin environments. Starting the console does not make a model request. Submit a prompt to begin. `--help` lists all flags.

The separate module uses the repository's root AXLR library. `go.work` supports development, and `tui/go.mod` has a local `replace` so `GOWORK=off go -C tui build ./cmd/axlr-tui` also works without fetching an unreleased AXLR version. Build from a full repository checkout.

## Controls

| Key | Action |
| --- | --- |
| Ctrl+S | Send prompt |
| Enter | Newline |
| A / D | Approve / deny the displayed tool call |
| Esc | Close overlay or cancel the current turn |
| Ctrl+C | Cancel active work; quit when idle |
| Ctrl+R | Explicitly continue an interrupted session |
| Ctrl+F | Search the current session |
| Enter / Shift+Enter in search | Next / previous hit |
| Ctrl+P | Action palette |
| Ctrl+O | Saved sessions; arrows select, Enter opens |
| F1 | Help |
| Tab | Transcript / activity at narrow widths |
| PgUp / PgDn | Scroll |

Mouse controls match the keyboard actions. Tool arguments and exact targets appear before approval; scroll approval details with arrows, PgUp/PgDn or the wheel. Every tool call requires its own decision. A turn allows up to 32 tool calls. Unknown calls are rejected. Cancelled or uncertain effects are recorded and never automatically retried.

Use a terminal of at least 50 columns by 15 rows. Wide terminals show activity beside the transcript; narrower terminals use tabs. Set `NO_COLOR=1` for monochrome output.

## Plugins

Register each plugin explicitly with an absolute path to a [version 1 AXLR manifest](../README.md#external-tool-plugins). There is no directory scanning. A manifest specifies an absolute executable, arguments and allowed tool names:

```json
{
  "manifest_version": 1,
  "id": "notes",
  "command": "/absolute/path/to/notes-mcp",
  "args": [],
  "allow_tools": ["search"]
}
```

```bash
/tmp/axlr-tui --root "$PWD" --model 'your-provider/your-model' \
  --plugin /absolute/path/to/notes.json \
  --plugin-env-from 'notes:TOKEN=NOTES_API_TOKEN'
```

`--plugin` and `--plugin-env-from ID:KEY=HOST_ENV_VAR` are repeatable. The latter copies only the selected host variable into that plugin's `KEY`. Values stay out of command-line arguments. Plugin processes receive only explicit selections; local exec starts with an empty environment. Use absolute executable paths. Forwarding `OPENROUTER_API_KEY` to plugins is rejected. Plugin connections start when a turn discovers tools and close when the console exits.

## Local sessions and recovery

Sessions contain prompts, model output, tool arguments and results: treat them as local user data. They live under `$XDG_STATE_HOME/axlr/sessions`, or `$HOME/.local/state/axlr/sessions` if XDG state home is unset or relative. Directories are owner-only (0700), snapshots are 0600, updates use atomic replacement, and each open session has an exclusive writer lock.

The header identifies the current session. Open another through Ctrl+O, or start with:

```bash
/tmp/axlr-tui --root "$PWD" --model 'your-provider/your-model' \
  --session 0123456789abcdef0123456789abcdef
```

The startup workspace and model must match the saved session. The picker refuses a different workspace. Restored in-progress sessions appear interrupted and do not execute anything on load. Ctrl+R explicitly continues; pending calls reopen for individual decisions. Partial output is kept as an interrupted draft, outside valid model history. SIGTERM or terminal failure cancels active work and waits for its stable save before releasing resources.

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
