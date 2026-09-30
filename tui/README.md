# AXLR agent console

`axlr-tui` is a streaming OpenRouter console for a local workspace. It offers AXLR read, write, edit and exec tools, explicitly registered MCP servers, persistent server approval policies, an AXLR plugin package catalog compatible with Codex package formats, transcript search and resumable sessions. It targets trusted-local Linux and runs with your account's OS access.

## Build and start

Use Go 1.26 from this repository checkout:

```bash
go -C tui build -trimpath -o /tmp/axlr-tui ./cmd/axlr-tui
# Supply OPENROUTER_API_KEY through your environment or secret manager.
/tmp/axlr-tui
```

`--root` defaults to the current directory. Type `/model` and press Enter to choose a model before sending your first prompt. The searchable catalog lists text models that support tools, with provider filters and a detail pane showing context limits and pricing when available. Arrow keys and PgUp/PgDn navigate, Tab changes provider, Enter selects, and Esc closes; failed requests offer Retry. The action palette also offers Models and preserves a drafted prompt. The model selected through `/model` becomes the default for future new sessions.

The interface defaults to English. Start with `--lang es` for Spanish, or set `AXLR_LANG=es`; `--lang` takes precedence. Language changes affect interface labels only, preserving prompts, session history and tool results exactly as stored. Both catalogs use stable labels in `adapters/terminal/i18n.go`.

Use `--model your-provider/your-model` to override the default for one launch without fetching the catalog. Bare startup uses the saved default, if one exists; otherwise it does not create a session snapshot or lock file until a model is chosen. Quitting or a catalog failure before the first selection leaves no session. Changing models is allowed only while idle, complete, or interrupted without pending tool calls. Selection applies to subsequent requests and preserves the transcript. If saving the default fails after a session model change, the new session model remains active and the TUI shows a warning.

`OPENROUTER_API_KEY` is required at startup; it is never placed in session snapshots or plugin environments. Starting the console does not make a model request. Submit a prompt to begin. `--help` lists all flags.

The separate module uses the repository's root AXLR library. `go.work` supports development, and `tui/go.mod` has a local `replace` so `GOWORK=off go -C tui build ./cmd/axlr-tui` also works without fetching an unreleased AXLR version. Build from a full repository checkout.

## Controls

| Key | Action |
| --- | --- |
| Enter | Send prompt; run `/model`, `/mcp`, `/plugin` or `/theme` |
| A or Enter in `/mcp` | Review server approval; Enter confirms it, Esc cancels |
| I in `/mcp` | Install a third-party MCP from a manifest path or `ID https://host/mcp` |
| Tab in `/plugin` | Switch between installed packages and the available catalog |
| Enter in `/plugin` | Review and install the selected available plugin |
| M in `/plugin` | Add a Codex-compatible plugin package or marketplace from an HTTPS Git URL or absolute local path |
| R in `/mcp` or `/plugin` | Refresh the current inventory |
| / in `/mcp` or `/plugin` | Search servers/tools or plugin packages |
| Tab in `/model` | Cycle provider filters |
| I / A in `/theme` | Cycle icon profiles / toggle reduced motion |
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

Mouse controls match the keyboard actions. Tool arguments and exact targets appear before approval; scroll approval details with arrows, PgUp/PgDn or the wheel. Local tools and MCP servers default to an individual decision. A configured MCP server may opt into automatic approval; `/mcp` shows the exact ID and policy scope before confirming and persisting a change. Local catalog/history reads are automatically resolved; they do not grant approval to MCP calls. A turn allows up to 32 tool calls. Unknown calls are rejected. Cancelled or uncertain effects are recorded and never automatically retried.

Use a terminal of at least 50 columns by 15 rows. The transcript occupies the full terminal width; Tab opens the activity view. The composer grows with multiline drafts. User and assistant turns are separate, full-width rows. `/theme` previews Auto, Ink, Aurora, Paper and Phosphor; Enter saves the choice under `$XDG_STATE_HOME/axlr/ui-preference.json`, and Esc restores the previous theme. Icon profiles are Safe, Nerd Mono and ASCII. The terminal controls the font; Nerd Mono requires a Nerd Font installed in your terminal. `NO_COLOR=1` disables color. KMP tool requests and results use distinct memory rows; a memory indicator appears while KMP executes. Tool rows show compact previews; Actions → Info retains the full saved results in a scrollable view. Waiting and tool execution show an activity indicator and elapsed time, with a static indicator when reduced motion is enabled. Rows wrap to their content, and the transcript scrolls as turns accumulate.


## Model context

The full conversation and tool outcomes remain in the private session store. Model requests use a separate bounded projection: 96 KiB of history, a 64 KiB low watermark, up to 16 KiB per tool result and an 8 KiB extractive checkpoint. Checkpoints quote historical inputs and mark omissions explicitly. The current prompt and tool arguments are never silently shortened; an oversized active turn produces an explicit error. Restoring a session reproduces the projection without deleting history or calling a summarization model.

The model initially receives the four local tools and three host controls: `axlr_tools` searches or retrieves exact plugin schemas; `axlr_call_tool` invokes the registered plugin target; `axlr_history` reads original messages in pages. Discovery uses `offset` and returned `next_offset`; history uses `message_index`, `offset_bytes` and returned `next_offset_bytes`. Retrieved schemas can be reused for the frozen catalog. Exact schema results are limited to 32 KiB. Plugin arguments are validated locally against that schema before execution, and unsupported schema assertions produce an explicit tool error.

Provider reasoning and tool preparation have separate status indicators. They show activity without displaying hidden reasoning text. See the [implementation research and measurements](../docs/research/2026-09-30-context-policy.md).

## Plugin packages and MCP servers

`/plugin` manages packages in AXLR's own data directory (`$XDG_DATA_HOME/axlr/plugins` or `~/.local/share/axlr/plugins`). KMP and MADE are built in. Press M to add a local package or marketplace path, or an HTTPS Git repository; AXLR reads `.codex-plugin/plugin.json` and `.agents/plugins/marketplace.json`. The packages appear in the available tab, where Enter installs one into AXLR. Installed package skills are indexed for the model to read on demand, and declared MCP servers are registered in AXLR with manual approval. `/mcp` shows server connections. This does not change the Codex app's plugins. AXLR currently supports plugin skills and MCP servers; other Codex plugin components are retained in the package but not activated.

`/mcp` shows AXLR's connected servers, discovered tools, and their approval policies. KMP and MADE remain connected with their existing policies when present in `mcp.json`. Press I to install an additional server during the running session. Enter an absolute path to an AXLR manifest for a local stdio server, or `ID https://host/mcp` for a Streamable HTTP endpoint. The initial approval policy is manual; the connection is persisted in `mcp.json` and becomes available for the next tool discovery. An unreachable server is shown as unavailable after discovery.

Register a local server with an absolute path to a [version 1 AXLR manifest](../README.md#external-tool-plugins). A manifest specifies an absolute executable, arguments and allowed tool names. `"*"` opts in to every tool advertised by that server; use exact names to restrict it:

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

Optional `purpose` is `tools` (default), `memory` or `ceremony`. Optional `approval` is `manual` (default) or `auto`. Automatic approval applies to tools allowed by that server's manifest, including future tools if `allow_tools` is `"*"`. Policies are persisted with a process-shared lock and atomic private-file replacement. MADE ceremony tools use the same exact-server policy: set MADE to `auto` in `/mcp` or its persisted entry to execute allowed ceremony calls without individual approval. Command-line-only registrations remain manual unless placed in the persistent config.

AXLR loads this file at every start, including launches without `--plugin`. `env_from` copies only named host variables; optional `env` supplies literal values for a plugin. Both maps become that plugin's complete child environment. Use `--mcp-config /absolute/path/config.json` to select another file. The config lists manifests explicitly, so tools from unrelated applications are not silently started.

```bash
/tmp/axlr-tui --root "$PWD" --model 'your-provider/your-model' \
  --plugin /absolute/path/to/notes.json \
  --plugin-env-from 'notes:TOKEN=NOTES_API_TOKEN'
```

`--plugin` and `--plugin-env-from ID:KEY=HOST_ENV_VAR` are repeatable and add one-launch registrations. The latter copies only the selected host variable into that plugin's `KEY`. Values stay out of command-line arguments. Plugin processes receive only explicit selections; local exec starts with an empty environment. Use absolute executable paths. Forwarding `OPENROUTER_API_KEY` to plugins is rejected. Plugin connections start when a turn discovers tools and close when the console exits. A configured server that fails discovery reports an error before the model request.

## Local sessions and recovery

Sessions contain prompts, model output, tool arguments and results: treat them as local user data. They live under `$XDG_STATE_HOME/axlr/sessions`, or `$HOME/.local/state/axlr/sessions` if XDG state home is unset or relative. The default model and interface settings are stored separately as `model-preference.json` and `ui-preference.json` in that `axlr` directory. Directories are owner-only (0700), snapshots and preference files are 0600, updates use atomic replacement, and each open session has an exclusive writer lock.

The header identifies the current session. Open another through Ctrl+O, or start with:

```bash
/tmp/axlr-tui --root "$PWD" \
  --session 0123456789abcdef0123456789abcdef
```

The startup workspace must match the saved session. `--session` restores its saved model without fetching the catalog. If you also supply `--model`, it must match the saved model; use `/model` after loading to change it. The picker refuses a different workspace. Restored in-progress sessions appear interrupted and do not execute anything on load. Ctrl+R explicitly continues; pending calls resume under the current plugin policy; local tools and manual plugins still require individual decisions. Partial output is kept as an interrupted draft, outside valid model history. SIGTERM or terminal failure cancels active work and waits for its stable save before releasing resources.

## Diagnostics

Every launch records startup, model requests, streaming progress, event delivery, rendering dimensions and time, session saves, and completion. Traces are written to a unique file under `$XDG_STATE_HOME/axlr/logs`, or `$HOME/.local/state/axlr/logs`. The path is printed on startup. Directories are private (`0700`), and files are owner-only (`0600`). Each measured action has `action_start` and `action_end`, a span ID and parent ID, with microsecond durations. Records include a run ID to distinguish launches appended to one trace, correlated request IDs, endpoint categories, byte counts, timings, dimensions and error classes; they omit prompts, responses, model IDs, tool arguments and credentials. `--trace-file /path/axlr-tui.jsonl` overrides the location and appends to an existing file; relative paths are also accepted.

All OpenRouter API requests and response bodies, including the model catalog and HTTP errors, are captured by default in a separate private directory beside the trace. Each launch gets a unique payload directory, including when the trace file is reused. Its path is printed on startup. Authorization headers are never captured, and the configured API key and recognizable credential patterns are redacted from bodies. Payloads contain conversation content and tool results; captures are limited to **8 MiB per file**. There is no per-launch byte budget that stops later requests from being captured. HTTP error bodies are read on close with an 8 MiB / 500 ms bound; partial captures, unavailable responses and I/O failures have an explicit `payload_failed` record. Use `--trace-payloads=false` to keep timing logs without body captures.

The `context_projected` record reports original/projected message counts and bytes, dropped messages and the cut index. `request_sent` reports message count, tool count, separate message/schema bytes and outgoing bytes. Plugin argument validation has its own timed `tool_validation` action. `provider_headers.elapsed_ms` measures time to HTTP headers; `provider_content.elapsed_ms` measures arrival of visible answer text. `provider_reasoning`, `provider_tool_delta`, `provider_heartbeat` and `provider_wire_bytes` distinguish reasoning, tool requests, keepalives and received bytes from visible text. Tool completion and render timings distinguish MCP latency from provider waits. See [the measured MCP diagnostic](../docs/diagnostics/2026-09-30-tui-mcp.md) and [the payload audit and live generation probe](../docs/diagnostics/2026-09-30-tui-payloads.md).

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
