# Interactive console

`axlr-tui` is AXLR's terminal interface. It streams output from OpenRouter or from [local OpenAI-compatible servers](#local-models), lets you review tool calls, and keeps resumable local sessions. [Getting started](getting-started.md) covers the initial build and launch.

## Work in a session

The console uses the current directory unless `--root` names another existing workspace. Type a prompt and press Enter. Use Shift+Enter for a newline. While AXLR is running, sending another message queues it: the transcript shows it as queued and the footer adds “message queued”. The current model step is not interrupted, so a reasoning model keeps its work. When the step ends in tool calls, the queued message joins the running turn after their results and the next model request carries it, marked for the model as sent while it was working, so it answers both; when the step ends in an answer, the queued message starts the next turn. A ceremony step is never joined: the console stops it at its next model request and starts a turn with the message, without reporting that stop as an error. Several messages sent meanwhile are joined. Esc still cancels the running step. Up and Down in the composer browse only this session's user messages and return to the current draft. The first model choice is made through `/model` or `--model`; selecting a model in `/model` also saves it as the default for future new sessions.

The model picker filters to text models that support tools. Tab changes provider, arrows and PgUp/PgDn move, and Enter selects. A catalog failure offers Retry. A `--model` value bypasses the catalog for that launch. The selected model can change later only when the turn is idle, complete or interrupted without pending tool calls; the transcript remains in the session.

| Control | Action |
|:--|:--|
| `Enter` / `Shift+Enter` | Send / insert a newline |
| `/model` | Choose a model |
| `/normal`, `/review`, `/writer`, `/research` | Choose direct work and its local tool policy |
| `/stop-ceremony` | Cancel the running ceremony in MADE and return to normal mode (alias `/parar`); see [a step the model will not hand back](ceremonies.md#a-step-the-model-will-not-hand-back) |
| `/plan` | Select planning for the next prompt: the brief becomes verified atomic tasks you approve, run by workers; shows the plans panel while a plan runs. See [plans](ceremonies.md#plans-atomic-tasks-for-small-models) |
| `/debug`, `/delivery`, `/incident`, `/repair`, `/improve` | Select a console-driven MADE procedure for the next prompt; `/incident`, `/repair` and `/improve` also open a pending approval card; `/repair` opens the self-repairs and improvements panel when the agent requested one from this session, and `/improve` does when one of them is an improvement |
| `/update` | Update explicitly configured local KMP and MADE engines |
| `/mcp` | Inspect connected MCP servers, tools and approvals |
| `/approvals` | Show saved always-allow tools and autonomy mode |
| `/autonomy on` / `/autonomy off` | Automatically approve permitted known tools, or restore normal policy |
| `/plugin` | Browse packages installed in AXLR and available sources |
| `/theme` | Preview and save appearance |
| `/changes` / `/diff` / `Ctrl+D` | Review this session's file changes |
| `Ctrl+P` | Open the action palette |
| `Ctrl+O` | Open a saved session |
| `Ctrl+F` | Search the current conversation |
| `Ctrl+R` | Explicitly continue an interrupted turn |
| `Tab` | Switch between transcript and activity views |
| `F1` | Show in-app help |
| `Esc` | Drop a selection, close an overlay or cancel the active turn |
| `Ctrl+C` | Cancel active work; quit when idle |
| Drag over the conversation | Select text and copy it on release; double click takes a word, triple click a row, Shift+click extends |
| `/copy` (alias `/copiar`) | Copy the last reply as the model wrote it, or the current selection |

The interface supports mouse controls where the terminal supplies them. Buttons, footer hints, catalog rows and the file-change list respond to a left click; the wheel scrolls the conversation and every scrolling panel. The console keeps mouse reporting on while it runs, so a plain drag is seen by AXLR rather than by the terminal; see [copy and paste](#copy-and-paste). In an approval dialog, inspect the exact target and arguments with arrows, PgUp/PgDn or the wheel; `A` approves once and `D` denies. When offered, `L` executes this call and always allows the exact tool identity on future calls, `F` activates full autonomy and executes this call. `/approvals` lists saved choices. The autonomy switch approves known tools that the work mode permits; it persists until turned off. Restricted modes still require a decision for every local `exec`, and a new ceremony check command always needs approval. Their dialogs offer only decisions that apply to that call. Unknown tool names cannot be approved. An ordinary turn is limited to 32 tool calls; driven ceremonies reset the allowance as a step advances, and under the [compact profile](ceremonies.md#the-compact-profile-for-small-models) a step has 16. Cancellation does not reverse an effect that already happened.

## Copy and paste

Drag over the conversation to select text: the selection is painted while the button is held and copied when it is released, with a short confirmation in the footer. Double click selects a word, triple click a row, and Shift+click extends the selection to another cell. Esc drops it. The copied text follows the wrapped lines as shown, without the gutter. `/copy` (or **Copy last reply** in the action palette, `Y`) copies the last reply exactly as the model wrote it, including Markdown and code fences, or the selection if one exists.

The console writes the clipboard with the OSC 52 terminal sequence, so copying works over SSH, and mirrors it to the X11 primary selection and to the host clipboard tools (`wl-copy`, `xclip`, `xsel`, `pbcopy`) when they are present. Terminals that refuse OSC 52 by default need it enabled: kitty, foot, WezTerm, Alacritty, iTerm2 and Windows Terminal accept writes, xterm needs `allowWindowOps`, and older VTE terminals (GNOME Terminal, Tilix) ignore them and rely on the host fallback; inside tmux add `set -g set-clipboard on`. The console cannot tell whether the terminal accepted a write.

Hold Shift while dragging to bypass the console and let the terminal select natively (Alt in VS Code on Linux and Windows, Option in some macOS terminals). That selection includes the gutter and wraps lines where the screen wraps them. Paste with the terminal's shortcut (`Ctrl+Shift+V`, `Shift+Insert`, `Cmd+V`) or Shift+middle click; bracketed paste keeps newlines in the composer without sending the message.

## Work modes

Select a mode with its slash command between turns, after choosing a model. The mode is saved with the session and applies to subsequent work; switching while busy or with pending calls is refused. `normal` is the default for a new session.

| Command | Purpose | Local tool policy |
|:--|:--|:--|
| `/normal` | General agentic work | All four local tools, under saved approval policy |
| `/review` | Findings with locations, failure scenarios and evidence | Read; write/edit hidden and refused; every exec needs approval |
| `/writer` | Brief, outline, draft, critique and at most two revisions | Write/edit only `.md`, `.mdx`, `.txt`, `.rst` or paths under `docs/`; every exec needs approval |
| `/research` | Recover memory, read primary sources and produce a supported decision | Same document write policy as writer; every exec needs approval |
| `/plan` | Decompose a brief into atomic tasks you approve; workers run them | No edits while planning; workers (`task` sessions) run autonomous local tools within each task's scope; console drives `axlr_plan`, `axlr_task` and `axlr_sync` 1.0 |
| `/debug` | Reproduce, diagnose, repair and integrate | All local tools; console drives MADE `axlr_debug` 2.0 |
| `/delivery` | Brief, build/check and integrate | All local tools; console drives MADE `axlr_delivery` 2.0 |
| `/incident` | Blameless postmortem with review and person's approval | All local tools; model KMP outcome writes refused; console drives MADE `axlr_incident` 1.0 |
| `/repair` | Repair the configured repository in a fresh clone; the console opens, watches and merges the pull request | All local tools; model KMP writes refused; console drives MADE `axlr_repair` 1.0; start it with `axlr-tui --repair "<brief or #issue>"`, or let the agent request it with `axlr_request_repair` from any session |
| `/improve` | Improve the configured repository in a fresh clone against a check that fails first; the console opens and watches the pull request, and the person decides the merge | All local tools; model KMP writes refused; console drives MADE `axlr_improve` 1.0; start it with `axlr-tui --improve "<brief or #issue>"`, or let the agent request it with `axlr_request_improvement` from any session |

`review`, `writer` and `research` refuse any local exec with nonempty `stdin`, including when full autonomy is enabled. Arguments remain visible for per-call review. These restrictions apply to AXLR's local tools: MCP calls retain their own approval policy and may have external side effects. An approved process is not sandboxed by the mode.

Direct modes do not start ceremonies. For debug/delivery/incident, [prepare MADE](runbooks/made.md#prepare-the-driven-ceremonies), select the mode, then send the task. AXLR starts and claims the pinned definition; the model performs the work and hands results to `axlr_step_done`. A new debug/delivery check command is approved once, then reused unchanged for repair/build verification. Incident drafts receive a fresh-context review, then the person's approval through the `/incident` card. On `COMPLETED` or `BLOCKED`, the session returns to normal mode. Missing MADE or definitions prevents this start; there is no silent substitute ceremony. [Ceremonies](ceremonies.md) describes the two execution paths and recovery limits.

The local process environment contains only a sanitized absolute `PATH` and the host's `HOME` when it is absolute, so tools find their usual configuration (`~/.gitconfig`, `~/.cargo`, caches). Other shell variables and provider credentials are not inherited by console exec/check commands. A check that works in your login shell may therefore need explicit paths or a workspace script with documented prerequisites.

## Review file changes

Open `/changes` (or `/diff`, `Ctrl+D`, or **File changes** in the action palette) to review completed local `write` and `edit` calls, newest first. A Changes button with a count appears after the first change. Each entry represents one recorded edit, so repeated edits to a file remain individually reviewable. The preview shows the exact before and after text, line numbers, three lines of context, and added/removed counts. It works without Git and survives reopening the session, even if the workspace file has since changed.

![File changes in the running console, with Ink and Spanish labels; fixture session](assets/console-changes.png)

Use Up/Down to select an entry and Tab to focus its diff. PgUp/PgDn and the wheel scroll; Left/Right pan long lines; Home/End jump to the beginning/end; `[` and `]` select another entry while reading. Esc returns to the conversation and preserves the draft. At 90 columns or more the list and diff appear together; narrower terminals switch between them with Tab. File rows and Close also support the mouse.

Previews retain at most 64 KiB and fewer than 2,000 newlines per version. Larger files and unavailable text previews show an explicit notice, without a partial or guessed diff. Failed, denied, cancelled and unchanged edits do not appear. Empty file creation does. File snapshots are stored privately with tool activity, outside the messages sent to the model. Existing sessions without snapshots remain readable; old edits cannot be reconstructed. `exec` commands and MCP/plugin effects do not produce file snapshots.

## Appearance and language

`/theme` offers Auto, Ink, Aurora, Paper, Phosphor and Editorial. Editorial is light and also changes the layout: speaker labels, consecutive tool calls summarised in one line, and the approval as a bottom sheet. Press `I` for Safe, Nerd Mono or ASCII icons; `A` toggles reduced motion; Enter saves. Nerd Mono needs a Nerd Font configured in the terminal. `NO_COLOR=1` disables color.

The input's text, background, placeholder, selection and cursor follow the active theme, including live previews and cancellation. Auto also updates the input after detecting the terminal's background.

English is the default. `--lang es` or `AXLR_LANG=es` changes interface labels only. Prompts, tool results and stored conversation text are not translated.

## Edit settings as JSON

The console reads `$XDG_CONFIG_HOME/axlr/settings.json`, or `$HOME/.config/axlr/settings.json` when `XDG_CONFIG_HOME` is unset. Create or edit it with any text editor:

```json
{
  "model": "provider/model",
  "language": "es",
  "theme": "auto",
  "icons": "safe",
  "reduce_motion": false,
  "approvals": {
    "autonomous": false,
    "allowed": []
  },
  "repair": {
    "repository": "underpass-ai/AXLR",
    "directory": "/absolute/repairs",
    "auto_merge": false,
    "watch_minutes": 45,
    "about": "project:axlr",
    "autonomous": true,
    "max_attempts": 2
  },
  "context_tokens": 65536,
  "models": {
    "z-ai/glm-5.3-flash": {
      "provider": {"sort": "throughput"}
    }
  },
  "local_models": [
    {
      "id": "local/qwen3.8-27b",
      "name": "Qwen3.8-27B (llama.cpp)",
      "url": "http://127.0.0.1:8080/v1",
      "context_tokens": 65536
    },
    {
      "id": "local/gemma-4-31b",
      "name": "Gemma 4 31B (vLLM)",
      "url": "http://127.0.0.1:8082/v1",
      "model": "gemma-4-31b",
      "context_tokens": 65536
    }
  ]
}
```

All keys are optional. `repair` configures `/repair`, the agent's self-repair and `/improve`: `repository` is the `owner/name` the console may repair (default `underpass-ai/AXLR`), `directory` where it clones (default `<data>/axlr/repairs`), `auto_merge` whether green pull requests merge without the person (default `false`), `watch_minutes` how long one check round may take (default 45), `about` the KMP project scope (default `project:<name>`), `autonomous` whether an agent-requested repair session runs local tools in its clone without a card (default `true`; the reproduction command and the merge always wait for the person) and `max_attempts` how many repair sessions one failure may start (default 2, at most 5). `/improve` uses `repository`, `directory`, `watch_minutes` and `about`; `auto_merge` never applies to an improvement, whose merge always waits for the person. See [ceremonies](ceremonies.md#self-repair-the-console-drives-the-pull-request) and [self-repair from a running session](ceremonies.md#self-repair-from-a-running-session). `model` may be empty to choose a model in the console. `language` accepts `en` or `es`; `theme` accepts `auto`, `ink`, `aurora`, `paper`, `phosphor` or `editorial`; `icons` accepts `safe`, `nerd-mono` or `ascii`. In `approvals`, `autonomous` enables automatic approval for every known tool; `allowed` contains exact tool identities saved by the approval dialog. Edits take effect on the next launch. `--model` overrides the JSON model for one launch; `--lang` overrides `AXLR_LANG`, which overrides the JSON language. Selecting `/model`, saving `/theme`, or changing `/autonomy` or always-allow choices updates the corresponding JSON keys while retaining other settings, including keys from newer AXLR versions. The console writes the file with owner-only permissions and rejects invalid JSON without replacing it.

`prompt_tokens` (the prompt a request to a model without a known window may reach; default 64,000, at least 16,384) is described under [model context](#model-context), `context_tokens` and `local_models` under [local models](#local-models), `models` under [per-model request options](#per-model-request-options), `jev` under [Jev](#jev-an-external-judge-off-by-default), and `ceremonies.profile` (`auto`, `standard` or `compact`) under [the compact profile](ceremonies.md#the-compact-profile-for-small-models), `trace_retention_days` (default 30; `0` keeps every trace) and `trace_payloads` (default `false`) under [diagnostics and privacy](#diagnostics-and-privacy). `plan.model` names the model that decomposes a brief (default `session`, the session's model) and `plan.auto_approve` starts a verified plan without the person (default `false`); see [plans](ceremonies.md#plans-atomic-tasks-for-small-models).

Existing `model-preference.json` and `ui-preference.json` files under the state directory are read until `settings.json` exists. Existing `approvals.json` choices are read until `settings.json` has an `approvals` section. The next related change writes those values into `settings.json`; the old files are left in place. MCP server connections and their own approval policies remain in the separate `$XDG_CONFIG_HOME/axlr/mcp.json` file.

## Model context

The private session store retains the full transcript. Model requests receive a bounded projection whose four limits are bytes, not the model's advertised token window: a message-history ceiling, a low watermark the history is cut back to once it crosses the ceiling, a limit per tool result and an extractive checkpoint. Which limits apply depends on what the console knows about the model:

- A model whose window is unknown, which is every OpenRouter model, gets the **prompt budget**: `prompt_tokens` in [settings](#edit-settings-as-json) (default 64,000) is the prompt a request may reach. At the 2.44 bytes per token measured on Claude Haiku 5.5, minus 17 KiB for the guidance and the tool schemas, that is a 138,752-byte ceiling, a 69,376-byte low watermark, 17,344 bytes per tool result and an 11,562-byte checkpoint. The default stays under the 100K prompt tokens above which Haiku 5.5 costs five times as much; a session of 198 requests that reached 315K tokens per request and $14.13 in total under the earlier 1 MiB ceiling replays at 62K tokens at most and $0.90 under this budget ([research](research/2026-10-08-bounded-remote-context.md)). The budget applies to every remote model, the plan model included; raise `prompt_tokens` for a cheaper model with a larger window, and a global `context_tokens` smaller than the prompt budget still bounds it. Tokenizers differ: z-ai/glm-5.3-flash held about 4.2 bytes per prompt token on 8 October 2026, so at 2.44 its requests stopped near 37K tokens. The console therefore measures each remote request, the size of the body it sent against the prompt tokens the provider counted, and keeps a moving average per model (each ratio clamped to 1.5–6 bytes; prompts under 2,048 tokens are not measured; past 50 requests a new one weighs a fiftieth) in `bytes-per-token.json` under the state directory, shared by every console. After five measured requests the model's prompt budget uses its own ratio: 251,392 bytes of history for glm-5.3-flash at 4.2, which brings its requests to the 64,000 tokens the budget aims at. A console fixes the ratio it applies to a model the first time it uses it, so the ceiling does not move under a running session; the next launch picks up what was measured since. Local models keep the window budget.
- A [local model](#local-models) gets the **window budget**, since its server holds the window for free: three quarters of its window at three bytes per token (147,456 bytes for 65,536 tokens), a low watermark three quarters of the ceiling, a tool result at most an eighth of the ceiling and the checkpoint at most an eighth of the low watermark. A window that holds more than the 1 MiB ceiling keeps the ceiling: 1 MiB, 768 KiB, 64 KiB per tool result and a 16 KiB checkpoint.

When the history crosses the ceiling, whole earlier turns are dropped until it is back under the low watermark, and one checkpoint message takes their place. It quotes each omitted request of the person exactly, newest first (an excerpt when they no longer fit), and next to each the files the turn wrote, which are on disk, and the memory it recorded in KMP (`about` and `idempotency_key` of each accepted `kmp_write_memory`); a turn that wrote files or made five or more local calls other than file reads in a session that uses KMP, and recorded nothing, is marked `unrecorded`. Console messages (`[AXLR…]`) belong to the request before them. The checkpoint keeps the KMP agent and context identities and the index of the last guide and protocol result, and tells the model to recover what the session settled from KMP, a file from disk and one exact message with `axlr_history`. When the map no longer fits, older turns keep only their request and `records_omitted_at_or_before` says from which one. The checkpoint quotes no assistant prose.

In a closed turn, the arguments of a `local_write` or `local_edit` call of 512 bytes or more keep their path and mode, and the written text is replaced by its size (`content_omitted_bytes`, `old_text_omitted_bytes`, `new_text_omitted_bytes`) and a note to `local_read` the file: the file on disk holds it. The saved transcript keeps them whole, and the turn in progress keeps them exact.

Host results such as exact tool-discovery schemas are retained up to 64 KiB, and `axlr_history` pages hold up to 32 KiB, or what the model's budget keeps per tool result when that is less, so a recovered page is never excerpted again.

A `local_read` page is bounded the same way: without `max_bytes`, or with a larger one, it holds what the budget keeps per tool result less 512 bytes for the result's offsets, status and digest (16,832 bytes under the default prompt budget), and a page whose JSON escaping (newlines, quotes, `<`, `>`, `&`) still overflows the budget is read again, shorter, until it fits. Its `next_offset_bytes` is therefore where the file continues. Under the default prompt budget, a 30 KB `docs/console.md` read without `max_bytes` used to come back as an excerpt whose `next_offset_bytes` was the end of the file; the model then tried a `query` field `local_read` does not have and re-read overlapping ranges. A `local_read` result excerpted all the same, such as one saved under a larger budget, names the offset of the first byte its excerpt omits and the largest `max_bytes` to read it with, and says that its `next_offset_bytes` follows the whole page; other excerpts suggest a narrower query, filter or page.

### Per-model request options

`models` in [settings](#edit-settings-as-json) adds fields to the requests OpenRouter receives for one model. The key is the exact model id a request names, suffix included: an entry for `z-ai/glm-5.3-flash` does not apply to `z-ai/glm-5.3-flash:nitro`. The entry applies to every request for that model, the session's, a plan's, its workers' and the reviewer's. Without an entry, the default, a request carries none of these fields and OpenRouter chooses the endpoint and the defaults. An entry may hold:

| Key | Meaning |
|:--|:--|
| `provider` | OpenRouter's [provider routing](https://openrouter.ai/docs/features/provider-routing), sent as written: `order`, `only` and `ignore` (lists of provider slugs), `allow_fallbacks` (OpenRouter's default `true`), `require_parameters`, `data_collection` (`allow` or `deny`), `zdr`, `quantizations`, `sort` (`price`, `throughput` or `latency`, or an object with `by` and `partition`), `preferred_min_throughput` (tokens per second) and `preferred_max_latency` (seconds) as numbers, and `max_price` with `prompt`, `completion`, `request` and `image` (dollars; per million tokens for `prompt` and `completion`) |
| `reasoning` | OpenRouter's [reasoning](https://openrouter.ai/docs/use-cases/reasoning-tokens) object: `effort` (`max`, `xhigh`, `high`, `medium`, `low`, `minimal` or `none`) or `max_tokens`, not both, plus `exclude` and `enabled` |
| `max_tokens` | Caps the completion, from 1 to 1,000,000 tokens |

The console refuses to launch with an entry that has an unknown key (a misspelt one would otherwise leave the routing as it was), a value of the wrong type or outside these values, or that names a model of `local_models`: local servers receive none of these fields, and their own keys are under [local models](#local-models).

The levers on an endpoint's price and speed are `provider.sort: "throughput"`, `provider.max_price.completion`, `provider.ignore` and the `:nitro` suffix. On 8 October 2026 OpenRouter routed `z-ai/glm-5.3-flash`, requested without preferences, to OpenInference: $0.032 per million input tokens, $3.109 per million output tokens, about 10 seconds to the first byte and about 19 tokens per second. Relace and InferenceNet served the same model at $0.50 per million output tokens, 1 to 2 seconds to the first byte and 78 to 177 tokens per second. Output was 75 % of that session's cost, and their endpoints would have cut the session's cost by about 59 %. `z-ai/glm-5.3-flash:nitro`, which sorts by throughput, went to InferenceNet: about 1.1 seconds to the first byte and 120 to 180 tokens per second. `"provider": {"sort": "throughput"}` asks for the same order under the plain id. `"max_price": {"completion": 1}` holds out endpoints whose output costs more than $1 per million tokens, a ceiling that would have held out the $3.109 of 8 October but not the $0.937 OpenInference listed on 9 October; `ignore` holds out providers by the slug OpenRouter lists them under. A ceiling or an exclusion that leaves no endpoint leaves OpenRouter nothing to route to.

`sort: "price"` is accepted but is not the lever it seems: OpenRouter does not document which price it sorts by (checked on 9 October 2026), and the endpoint list of the model is ordered by input price, where OpenInference, the cheapest input, comes first. An order weighted by input price would choose the endpoint of the case above. No sort orders by output price; `max_price.completion` is the only control over it, as a ceiling.

### Prompt caching

A request to an `anthropic/*` model through OpenRouter asks for Anthropic's prompt cache, which OpenRouter forwards to Anthropic, Claude Platform on AWS, Bedrock and Vertex: a top-level `cache_control` places the automatic breakpoint on the last block of the conversation and moves it forward as the conversation grows, and an explicit breakpoint on the system prompt keeps the fixed prefix (schemas and guidance) readable after a cut of the history, which rewrites everything behind the checkpoint. A cached prefix is read at a tenth of the input price and written at 1.25 times it; the entry lives five minutes and every request renews it, and a prefix under 512 tokens is not cached. The projection keeps the prefix stable between requests: a clipped tool result reads the same before and after its turn closes, and the history is cut only when it crosses the ceiling. The `provider_done` trace event reports `prompt_tokens`, `completion_tokens`, `cached_tokens` and `cache_write_tokens`, and the footer shows the cached share of the last request's prompt next to its tokens. Other models and local servers receive the request unchanged. Replaying the session of the [research note](research/2026-10-08-bounded-remote-context.md) with the prompt budget and this cache gives 87 % of prompt tokens read from the cache and $0.28 instead of $0.90.

When the active turn alone exceeds the budget, AXLR progressively reduces its tool-result excerpts to 8, 4, 2 and then 1 KiB. It asks the model to finish from the evidence already gathered. The current prompt and tool arguments are not silently shortened; context that still cannot fit produces an explicit error. Full saved results remain intact. `axlr_history` can recover earlier-turn messages, but refuses tool results from the current turn to avoid a rereading loop.

The model normally sees four local tools plus `axlr_tools` (exact plugin schema discovery), `axlr_call_tool` (registered plugin invocation), `axlr_history` (paged saved messages), `axlr_skill` (paged skill resources), `axlr_session` (current-session title and exact memory scope), `axlr_request_repair` (ask the console to repair a defect of AXLR in a separate session, with evidence the console validates), `axlr_request_improvement` (ask it to improve AXLR the same way, citing at least two calls that show the friction) and `axlr_repair_status` (the repairs and improvements linked to the session), plus `axlr_judge` when [Jev](#jev-an-external-judge-off-by-default) is enabled. Session bookkeeping and repair and improvement requests run without a separate approval, because the request itself starts nothing the person has not agreed to; plugin calls retain their configured policy. Review mode hides local write/edit; an active driven ceremony adds `axlr_step_done`; a repair or improvement session never sees `axlr_request_repair`, and repair, improvement, plan and task sessions never see `axlr_request_improvement`. Plugin arguments are validated against the discovered schema before execution; `x-*` keywords are treated as annotations. `axlr_tools` returns a schema without its `x-*` annotations; a schema that still does not fit one host result comes back as an outline of paths and sizes, and `path` (a JSON pointer such as `/properties/stages`) returns that part exactly, including an annotation such as `/x-made-pattern-catalog`. Read only the required schema and follow resource page cursors. [Context policy research](research/2026-09-30-context-policy.md) records the original design; [the audit](documentation-audit.md) covers the later active-turn compaction change.

### Memory writes

When KMP is connected with `kmp_write_memory`, the normal-mode prompt names it among KMP's entry tools and tells the model to record what a task settles (a decision, constraint, fix or outcome worth reusing) before its final answer, with source evidence and a stable idempotency key under the session's exact about. The console then checks each request: one that changed a file or made five or more local tool calls (a command counts, since a check or a test can settle an outcome; `local_read`, session bookkeeping and KMP reads do not), ran no ceremony and attempted no `kmp_write_memory` gets one visible `[AXLR · memory]` message, and the model records the result or says in one sentence why there is nothing durable. A request is reminded at most once; Jev's final check still runs at most once per request and always judges the person's request, not a console message. Modes that refuse the model's memory writes, and driven ceremonies, whose outcome the console records, get neither the sentence nor the reminder. See the [KMP runbook](runbooks/kmp.md#use-memory-in-a-task) for the approval policy of the writes.

## Local models

`local_models` lists OpenAI-compatible servers, such as `llama-server` or vLLM, that `/model` lists together with OpenRouter's catalog; favorites keep a model on top. Each entry has:

| Key | Meaning |
|:--|:--|
| `id` | The model id the session saves and `/model` shows; a `local/` prefix groups the entries and keeps them apart from OpenRouter ids |
| `name` | Display name; default `id` |
| `url` | The server's OpenAI base URL, such as `http://127.0.0.1:8080/v1`; the console posts to `<url>/chat/completions` |
| `model` | The name sent to the server; default `id`. vLLM needs its `--served-model-name`; llama.cpp ignores it |
| `api_key_env` | The environment variable holding the server's key. A loopback URL (`localhost`, `127.0.0.0/8`, `::1`) needs no key and receives no `Authorization` header; any other host must use `https` and a key |
| `context_tokens` | Required, at least 4096: the window the console respects for this model. It may be smaller than the server's own `-c` or `--max-model-len` |
| `tools` | Default `true`. AXLR needs native tool calls, so `false` hides the model from `/model` instead of degrading it |
| `stream` | Default `true`. `false` asks the server for whole replies: the text appears at once and the "reasoning" status is not shown, but a server whose streaming tool-call parser leaks calls as text (vLLM 0.22.1 with `gemma4` did, once in three replays of the same request, while its non-streaming parser never did) returns real `tool_calls` |
| `thinking` | `false` sends `chat_template_kwargs: {"enable_thinking": false}`, which llama.cpp and vLLM pass to the chat template; plan workers want short replies, and Qwen3.8-27B spent 4,000 tokens thinking before one worker call when it was on. Unset leaves the server's default |
| `stream_idle_seconds`, `stream_max_minutes` | Stream limits; defaults 600 seconds without a byte and 60 minutes in total, since a cold prefill of a long prompt on a local GPU takes minutes before the first token |

The top-level `context_tokens` caps the window for every model, local or OpenRouter; a local model's own `context_tokens` applies when it is smaller. The console turns a local model's window into the byte budget of the [model context](#model-context): the message history may use three quarters of the window at three bytes per token, which leaves room for the guidance, the tool schemas and the reply. A model without a known window gets the prompt budget of `prompt_tokens` instead, bounded by the cap when there is one.

With at least one local model, `OPENROUTER_API_KEY` is optional: without it `/model` lists only the local models, and a session whose model is not local is refused at its first request. Model keys, both `OPENROUTER_API_KEY` and every `api_key_env`, are never passed to plugins, and they are redacted from diagnostics. Local requests are traced like OpenRouter's when their scheme, host, port and path match a configured `url`.

Serving is outside AXLR. A server must return native `tool_calls`: run llama.cpp with `--jinja` and a template that declares tools, and vLLM with `--enable-auto-tool-choice` and the parser that matches the model (`qwen3_coder` for Qwen, `gemma4` for Gemma 4, `mistral` for Devstral, `openai` for gpt-oss). Ollama truncates the prompt silently from the beginning unless `OLLAMA_CONTEXT_LENGTH` is set. [Troubleshooting](troubleshooting.md#local-models) has a smoke test.

## Jev, an external judge (off by default)

[TypeSafe Jev](https://docs.typesafe.ai/api.md) is a judgement model: given a state and a typed question, it returns the probability of yes, or a choice among options with a probability for each and its confidence. It neither generates text nor acts. The `jev` section lets a model, a small local one in particular, lean on it:

```json
"jev": {
  "tool": true,
  "final_check": true,
  "final_threshold": 0.5,
  "model": "jev-1.13.0",
  "timeout_ms": 20000
}
```

- `tool` offers the model `axlr_judge`, so it can ask Jev at a real fork: which approach or next step, which candidate fix, whether a result satisfies the task. The model writes the state Jev sees; Jev sees nothing else. The call runs without a card, like the other host bookkeeping. Without `tool`, the tool is not in the request at all, even in a session that used it before.
- `final_check` asks Jev, once per request, whether the model's final answer completes it. Jev sees the request, the final answer and one line per tool call. When the probability is below `final_threshold` (default 0.5), the console adds one visible `[AXLR · Jev]` message with the estimate, and the model continues: it either finishes the work or explains why the answer is complete. The check is skipped during a driven ceremony, and a failed or slow Jev call never blocks the turn. The diagnostics trace records each check's duration and failure, never its content.
- `model` is the pinned Jev version (default `jev-1.13.0`, the one KMP uses); `-latest` aliases are refused. `timeout_ms` bounds one request, from 1000 to 60000.

Either switch sends text to TypeSafe: the model's questions with their state, and, with `final_check`, the request and the final answer. The console says so on stderr at launch. The key is read from `TYPESAFE_API_KEY`, which must be set when either switch is on; it is redacted from diagnostics, and it stays available to plugins because KMP reads it for its own Jev features.

## Saved sessions and recovery

Sessions live under `$XDG_STATE_HOME/axlr/sessions` or `$HOME/.local/state/axlr/sessions`. User settings live in the [editable JSON file](#edit-settings-as-json). Directories use owner-only permissions; snapshots and settings written by AXLR are private files. An open session has an exclusive writer lock.

Open a session with `Ctrl+O` or start with:

```bash
/tmp/axlr-tui --root "$PWD" --session 0123456789abcdef0123456789abcdef
```

The workspace must match the saved one. The saved model is restored without a catalog request; an accompanying `--model` must match it. An interrupted session loads without executing pending calls. Use `Ctrl+R` to continue explicitly; pending tools follow the current approval policy. Partial output remains an interrupted draft outside valid model history.

## Diagnostics and privacy

Each launch writes a private JSONL timing trace under `$XDG_STATE_HOME/axlr/logs`, falling back to `$HOME/.local/state/axlr/logs`. The path is printed at startup. Trace records cover startup, requests, streaming, tools, rendering and session saves; they omit prompts, responses, tool arguments, model IDs and credentials.

Request and response bodies are not saved unless you ask. With `--trace-payloads`, or `trace_payloads: true` in [settings](#edit-settings-as-json), a separate private directory beside the trace (or in the state `logs/` directory when the trace is inside the workspace, so the model's own searches never read earlier requests) captures OpenRouter and configured local-model HTTP request and response bodies, including model catalog and errors, and the launch says so in one line with the directory. Those bodies contain prompts, tool results and the file contents the model read; on 9 October 2026 the default `logs/` directory held 295 MB of them, 74 MB from a single run, which is why capture became opt-in. `--trace-payloads=false` turns it off for one launch when the setting is on. Authorization headers are excluded; configured API keys and recognizable credentials (bearer values, `sk-` keys, GitHub, Slack and AWS access tokens, AWS secret keys assigned by name, and private key blocks) are redacted, in a streamed response also when the stream splits one across chunks. Capture is bounded to 8 MiB per file, and a launch can create multiple files. The trace and every capture are owner-only files (`0600`) in owner-only directories (`0700`). `--trace-file /path/trace.jsonl` selects another trace path and appends to an existing file.

A launch with the default trace deletes, without printing anything, what earlier launches left in that `logs/` directory once it has not changed for `trace_retention_days` (a `settings.json` key; default 30, `0` keeps everything): default `trace-<pid>-<n>.jsonl` files, and the request and response captures of every `*.jsonl.payloads-<n>` directory there, which is removed when nothing else remains in it. The current launch's files, a trace file chosen with `--trace-file` and anything outside that directory are never deleted, and links are not followed.

The [payload audit](diagnostics/2026-09-30-tui-payloads.md) and [MCP diagnostic](diagnostics/2026-09-30-tui-mcp.md) contain measured examples. [Troubleshooting](troubleshooting.md) gives a symptom-first path.
