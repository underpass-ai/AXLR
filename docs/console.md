# Interactive console

`axlr-tui` is AXLR's terminal interface. It streams OpenRouter output, lets you review tool calls, and keeps resumable local sessions. [Getting started](getting-started.md) covers the initial build and launch.

## Work in a session

The console uses the current directory unless `--root` names another existing workspace. Type a prompt and press Enter. Use Shift+Enter for a newline. While AXLR is running, sending another message steers the turn: an active model stream is interrupted and the new user message starts the next turn. If a tool is executing, AXLR waits for that effect to finish before steering. Up and Down in the composer browse only this session's user messages and return to the current draft. The first model choice is made through `/model` or `--model`; selecting a model in `/model` also saves it as the default for future new sessions.

The model picker filters to text models that support tools. Tab changes provider, arrows and PgUp/PgDn move, and Enter selects. A catalog failure offers Retry. A `--model` value bypasses the catalog for that launch. The selected model can change later only when the turn is idle, complete or interrupted without pending tool calls; the transcript remains in the session.

| Control | Action |
|:--|:--|
| `Enter` / `Shift+Enter` | Send / insert a newline |
| `/model` | Choose a model |
| `/mcp` | Inspect connected MCP servers, tools and approvals |
| `/approvals` | Show saved always-allow tools and autonomy mode |
| `/autonomy on` / `/autonomy off` | Automatically approve all known tools, or restore normal policy |
| `/plugin` | Browse packages installed in AXLR and available sources |
| `/theme` | Preview and save appearance |
| `/changes` / `/diff` / `Ctrl+D` | Review this session's file changes |
| `Ctrl+P` | Open the action palette |
| `Ctrl+O` | Open a saved session |
| `Ctrl+F` | Search the current conversation |
| `Ctrl+R` | Explicitly continue an interrupted turn |
| `Tab` | Switch between transcript and activity views |
| `F1` | Show in-app help |
| `Esc` | Close an overlay or cancel the active turn |
| `Ctrl+C` | Cancel active work; quit when idle |

The interface supports mouse controls where the terminal supplies them. In an approval dialog, inspect the exact target and arguments with arrows, PgUp/PgDn or the wheel; `A` approves once, `L` executes this call and always allows the exact tool identity on future calls, `F` activates full autonomy and executes this call, and `D` denies. `/approvals` lists saved choices. The autonomy switch approves every known local or registered tool without a dialog; it persists until turned off. Unknown tool names cannot be approved. A turn is limited to 32 tool calls. Cancellation does not reverse an effect that already happened.

## Review file changes

Open `/changes` (or `/diff`, `Ctrl+D`, or **File changes** in the action palette) to review completed local `write` and `edit` calls, newest first. A Changes button with a count appears after the first change. Each entry represents one recorded edit, so repeated edits to a file remain individually reviewable. The preview shows the exact before and after text, line numbers, three lines of context, and added/removed counts. It works without Git and survives reopening the session, even if the workspace file has since changed.

![File changes in the running console, with Ink and Spanish labels; fixture session](assets/console-changes.png)

Use Up/Down to select an entry and Tab to focus its diff. PgUp/PgDn and the wheel scroll; Left/Right pan long lines; Home/End jump to the beginning/end; `[` and `]` select another entry while reading. Esc returns to the conversation and preserves the draft. At 90 columns or more the list and diff appear together; narrower terminals switch between them with Tab. File rows and Close also support the mouse.

Previews retain at most 64 KiB and fewer than 2,000 newlines per version. Larger files and unavailable text previews show an explicit notice, without a partial or guessed diff. Failed, denied, cancelled and unchanged edits do not appear. Empty file creation does. File snapshots are stored privately with tool activity, outside the messages sent to the model. Existing sessions without snapshots remain readable; old edits cannot be reconstructed. `exec` commands and MCP/plugin effects do not produce file snapshots.

## Appearance and language

`/theme` offers Auto, Ink, Aurora, Paper and Phosphor. Press `I` for Safe, Nerd Mono or ASCII icons; `A` toggles reduced motion; Enter saves. Nerd Mono needs a Nerd Font configured in the terminal. `NO_COLOR=1` disables color.

English is the default. `--lang es` or `AXLR_LANG=es` changes interface labels only. Prompts, tool results and stored conversation text are not translated.

## Model context

The private session store retains the full transcript. Model requests receive a bounded projection of it: a 96 KiB history ceiling, 64 KiB low watermark, at most 16 KiB per tool result and an 8 KiB extractive checkpoint. The checkpoint quotes historical inputs and marks omissions. The current prompt and tool arguments are never silently shortened; an oversized active turn produces an error.

The model initially sees four local tools and three host controls: `axlr_tools` for discovering exact plugin schemas, `axlr_call_tool` for a registered plugin target and `axlr_history` for paged access to saved messages. AXLR validates plugin arguments against the discovered schema before execution. [Context policy research](research/2026-09-30-context-policy.md) explains the measured tradeoffs.

## Saved sessions and recovery

Sessions live under `$XDG_STATE_HOME/axlr/sessions` or `$HOME/.local/state/axlr/sessions`. The selected model and UI settings are stored separately in `model-preference.json` and `ui-preference.json`. Directories use owner-only permissions; snapshots and preferences are private files. An open session has an exclusive writer lock.

Open a session with `Ctrl+O` or start with:

```bash
/tmp/axlr-tui --root "$PWD" --session 0123456789abcdef0123456789abcdef
```

The workspace must match the saved one. The saved model is restored without a catalog request; an accompanying `--model` must match it. An interrupted session loads without executing pending calls. Use `Ctrl+R` to continue explicitly; pending tools follow the current approval policy. Partial output remains an interrupted draft outside valid model history.

## Diagnostics and privacy

Each launch writes a private JSONL timing trace under `$XDG_STATE_HOME/axlr/logs`, falling back to `$HOME/.local/state/axlr/logs`. The path is printed at startup. Trace records cover startup, requests, streaming, tools, rendering and session saves; they omit prompts, responses, tool arguments, model IDs and credentials.

By default, a separate private directory beside the trace captures OpenRouter HTTP request and response bodies, including model catalog and errors. Those bodies can contain prompts and tool results. Authorization headers are excluded; configured API keys and recognizable credential patterns are redacted. Capture is bounded to 8 MiB per file, and a launch can create multiple files. Use `--trace-payloads=false` to disable body capture while retaining timing records. `--trace-file /path/trace.jsonl` selects another trace path and appends to an existing file.

The [payload audit](diagnostics/2026-09-30-tui-payloads.md) and [MCP diagnostic](diagnostics/2026-09-30-tui-mcp.md) contain measured examples. [Troubleshooting](troubleshooting.md) gives a symptom-first path.
