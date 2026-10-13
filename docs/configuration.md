# Configuration reference

The interactive console, `axlr-tui`, reads its configuration from a few JSON files in one private directory. This page describes each file, every key it accepts, its default, its limits, what refuses it and what writes it. The [console guide](console.md) explains the behaviour behind most keys in context; this page is the exhaustive list. The JSON worker (`axlr`) and the HTTP service read none of these files: see the [worker contract](worker.md) and the [service configuration](api.md).

Everything below was read from the code on 13 October 2026. The source of truth is [user_settings.go](../tui/adapters/storage/user_settings.go) and its siblings for `settings.json`, [mcp_config.go](../tui/adapters/storage/mcp_config.go) for `mcp.json`, [manifest.go](../plugins/manifest.go) for MCP manifests and [credentials.go](../tui/adapters/storage/credentials.go) for `credentials.json`.

## Where the files live

Each base directory is the XDG variable when it holds an **absolute** path; a relative or empty value is ignored and the fallback under `$HOME` applies. Without an absolute `HOME` either, the launch stops (`absolute HOME or XDG_CONFIG_HOME is required for settings`, and the same for the state and data bases). On Windows supply an absolute `HOME`, or the XDG variables, as [getting started](getting-started.md) says.

| Base | Variable | Fallback | AXLR directory (`<config>`, `<state>`, `<data>` below) |
|:--|:--|:--|:--|
| Configuration | `XDG_CONFIG_HOME` | `$HOME/.config` | `<base>/axlr` |
| State | `XDG_STATE_HOME` | `$HOME/.local/state` | `<base>/axlr` |
| Data | `XDG_DATA_HOME` | `$HOME/.local/share` | `<base>/axlr` |
| Runtime | `XDG_RUNTIME_DIR` | `<state>` | `<base>/axlr/engines` (sockets of [shared engines](console.md#shared-engines) only) |

Files in `<config>`:

| File | Holds | Written by |
|:--|:--|:--|
| [`settings.json`](#settingsjson) | Every user setting: model, language, appearance, approvals, local models, budgets, ceremonies, sandbox, Jev, diagnostics | You, and the console for the keys listed in [who writes settings.json](#who-writes-settingsjson) |
| `settings.json.lock` | Empty lock file that serializes writers of `settings.json` | The console (`0600`) |
| [`credentials.json`](#credentialsjson) | The OpenRouter key and the TypeSafe (Jev) key the setup assistant saved; `OPENROUTER_API_KEY` and `TYPESAFE_API_KEY` win when set | The setup assistant |
| [`mcp.json`](#mcpjson) | MCP server registrations and their approval policy | You, `/mcp`, `/plugin`, `/update`, `P` on MADE |
| `mcp.json.lock` | Lock file that serializes writers of `mcp.json` | The console (`0600`) |
| [`approvals.json`](#approvalsjson-legacy) | Legacy approval choices, read until `settings.json` has an `approvals` section | Older consoles only |
| `engines/<id>-<random>.json` | [MCP manifests](#mcp-manifests) for the KMP and MADE engines AXLR installed | `/update`, the setup assistant |
| `plugins/<id>.json` | MCP manifests for Streamable HTTP servers added by URL in `/mcp` | `/mcp` |

`approvals.json` is looked up beside the MCP configuration in use, so `--mcp-config /other/dir/mcp.json` reads `/other/dir/approvals.json`. `engines/` and `plugins/` are created beside the MCP configuration as well. `settings.json` and `credentials.json` always come from `<config>`: `--mcp-config` selects connections, not another settings file.

Files in `<state>` and `<data>` are AXLR's own records, not configuration, but they are part of the picture:

| Path | Contents |
|:--|:--|
| `<state>/sessions/` | Saved sessions, their usage ledgers (`<id>.usage.json`) and side files, among them `<id>.instance`, the session's MADE instance with the live claim's fence, the last turn's record, the KMP entries still to write (see [the session ceremony](ceremonies.md#the-session-ceremony)), the session's KMP agent and context and the prompt its memory was last recalled for (see [what the model sees](runbooks/kmp.md#what-the-model-sees)) |
| `<state>/install-id` | This install's id, drawn once: the console registers with KMP as `axlr:<install id>` and reports to MADE with it as its incarnation |
| `<state>/made-grant` | The id of the MADE work grant this console last issued (or found it could not issue: a remote or operator-managed engine), so a changed grant is issued once |
| `<state>/kmp-agents.json` | The agent the console registered in each KMP store, keyed by the store's path; a lost entry is registered again |
| `<state>/logs/` | Diagnostics traces (`trace-<pid>-<n>.jsonl`), payload captures (`*.jsonl.payloads-<n>/`) and the app log `axlr.log`; see [diagnostics and privacy](console.md#diagnostics-and-privacy) |
| `<state>/session-labels.json` | Session titles, abouts and archive marks |
| `<state>/bytes-per-token.json` | Measured bytes per prompt token per remote model, shared by every console |
| `<state>/plans.json`, `<state>/repairs.json` | The plan and the repair/improvement registries |
| `<state>/runs/` | One lock per running console, so registries can tell a stopped console from a live one |
| `<state>/forged-tools/` | The registry of [forged tools](console.md#forged-tools), one per workspace |
| `<state>/model-preference.json`, `<state>/ui-preference.json` | Legacy preferences, read only while `settings.json` does not exist |
| `<data>/plugins/` | Packages installed with `/plugin` |
| `<data>/repairs/` | Default clone directory of `/repair` and `/improve` (`repair.directory` overrides it) |
| `<data>/engines/` | KMP and MADE packages downloaded by `/update` or the setup assistant |
| `<git top level>/.kernel`, `$XDG_DATA_HOME/kmp/default` | The KMP store the console opens for a workspace: the checkout's own inside a git repository, the per-user default outside one, unless the `kmp` entry's `env` pins another directory (KMP's own data; see the [KMP runbook](runbooks/kmp.md#let-axlr-install-it)) |
| `$HOME/.local/state/underpass-made/`, `$HOME/.config/underpass-made/embedded/` | MADE's default embedded store and its private configuration, created by the package's setup (MADE's own files). The launcher receives only `HOME` and `PATH`, so these paths ignore the XDG variables |

## settings.json

Path: `<config>/settings.json`, that is `$XDG_CONFIG_HOME/axlr/settings.json`, else `$HOME/.config/axlr/settings.json`. The launch prints it (`axlr-tui: settings: <path>`). There is no flag to choose another one.

### File requirements

The console opens the file without following a symbolic link and checks the open descriptor, so a path swapped for a link is refused rather than followed. When the file is read:

- A missing file is not an error: the console uses the defaults, or the [legacy preferences](#legacy-preference-files).
- It must be a regular file, not writable by group or others, and at most 64 KiB (65,536 bytes). Mode `0644` is accepted for reading; `0664` or `0666` is not. Otherwise: `settings.json must be a regular, non-writable-by-others file of at most 64 KiB`. On Windows the permission bits are not checked; the ACLs govern access.
- Its bytes must be valid UTF-8 (`invalid settings.json size or UTF-8`).
- It must hold exactly one JSON object. `null`, a JSON value of the wrong type for a key, or a second value after the first (`{"theme":"ink"} {"theme":"paper"}`) give `invalid settings.json JSON`.
- The object must then pass validation; each refusal names the key, as listed per key below.

Any of these stops the launch before the terminal opens. The console never rewrites a file it cannot read: every save reads the current file first and fails if it is invalid, so the file stays exactly as you left it.

A save (from `/model`, `/theme`, `/autonomy`, an always-allow choice or the setup assistant):

1. creates `<config>` with mode `0700` if needed;
2. opens `settings.json.lock` (created `0600`), which must itself be a private regular file, otherwise `settings lock <path> has mode … and must not be readable or writable by other accounts; run chmod 600 <path>`, and takes an exclusive lock on it, waiting while another console holds it;
3. reads the current file, applies the one change, validates the result;
4. writes it with two-space indentation and a final newline to a temporary `.axlr-settings-*` file of mode `0600` in the same directory, syncs it, renames it over `settings.json` and syncs the directory.

Two consoles saving different keys at the same moment keep both changes. A result over 64 KiB is refused (`settings.json exceeds 64 KiB`).

Edits you make by hand take effect on the next launch. The exception is `favorite_models`, which `/model` reads again each time it opens.

### Key names and unknown keys

- **Top level.** Key names match case-insensitively when read (`"PROMPT_TOKENS"` sets `prompt_tokens`) and are written back in lower case, once. A key this version does not know is kept, with its value untouched, and written back on every save, so a setting added by a newer AXLR survives an older console saving the theme.
- **Inside `approvals`.** The same: unknown keys are kept.
- **Inside `models`.** Every key is checked, case-sensitively: an unknown key (`"providr"`, or `"Provider"`) stops the launch. See [models](#models-per-model-openrouter-options).
- **Inside every other section** (`repair`, `jev`, `plan`, `jobs`, `engines`, `exec_sandbox`, `ceremonies`, each `local_models` entry). Field names match case-insensitively, and an unknown or misspelt field is **ignored without a message and dropped on the next save**. `"repair": {"repositry": "me/fork"}` repairs `underpass-ai/AXLR`. Check spelling against this page.

A `null` value for a whole section means the section is absent. Every key is optional.

### Who writes settings.json

| Action | Keys written |
|:--|:--|
| `/model`: Ctrl+Enter (or Ctrl+D) on a model, or the first model chosen while no default is saved | `model` |
| `/model`: Ctrl+S on a model | `favorite_models` |
| `/theme`: Enter | `theme`, `icons`, `reduce_motion` |
| `/autonomy on`, `/autonomy off`, `F` on an approval card | `approvals.autonomous` |
| `L` on an approval card | `approvals.allowed` |
| The setup assistant (`axlr-tui setup`, or the first launch in a terminal) | `language`, `model`, `local_models` |

Nothing else writes the file; every other key is yours to edit. Each save rewrites the whole file from its parsed form: your formatting and key order are not kept, keys are written in alphabetical order, and `model`, `language`, `theme`, `icons` and `reduce_motion` are always written, with their defaults when you left them out.

### Legacy preference files

Older consoles kept the model in `<state>/model-preference.json` and the theme, icons and motion in `<state>/ui-preference.json`. While `settings.json` does not exist, the console reads those two files and starts from their values; an invalid one is ignored with a line on stderr (`ignoring invalid saved UI preference`, `ignoring invalid saved model preference`). The first save creates `settings.json` with them, and the old files are left in place and never read again. `approvals.json` follows the same pattern; see [approvals.json (legacy)](#approvalsjson-legacy).

### Model identifiers

Wherever a key takes a model id (`model`, `reviewer_model`, `favorite_models`, `plan.model`, the keys of `models`, `local_models[].id` and `local_models[].model`), the value must be valid UTF-8, contain no control characters and not be empty or only spaces. The console does not check that the model exists: OpenRouter refuses an unknown id at the first request.

### Keys at a glance

| Key | Type | Default | Section |
|:--|:--|:--|:--|
| `model` | string | `""` (choose in `/model`) | [Model and appearance](#model) |
| `favorite_models` | array of strings | `[]` | [favorite_models](#favorite_models) |
| `reviewer_model` | string | `""` (the session's model) | [reviewer_model](#reviewer_model) |
| `language` | `"en"` \| `"es"` | `"en"` | [language](#language) |
| `theme` | string | `"auto"` | [theme](#theme) |
| `icons` | string | `"safe"` | [icons](#icons) |
| `reduce_motion` | boolean | `false` | [reduce_motion](#reduce_motion) |
| `notify` | `"bell"` \| `"osc"` \| `"off"` | `"bell"` | [notify](#notify) |
| `approvals` | object | absent (manual approval); a clean install writes `autonomous: true` with `always_ask` | [approvals](#approvals) |
| `context_tokens` | integer | `0` (no cap) | [context_tokens](#context_tokens) |
| `prompt_tokens` | integer | `0` (75,000); a clean install writes it | [prompt_tokens](#prompt_tokens) |
| `turn_tool_calls` | integer | `0` (32) | [turn_tool_calls](#turn_tool_calls) |
| `max_session_usd` | number | `0` (no limit) | [max_session_usd](#max_session_usd) |
| `local_models` | array of objects | `[]` | [local_models](#local_models) |
| `models` | object | `{}` | [models](#models-per-model-openrouter-options) |
| `exec_sandbox` | object | absent (`mode: off`) | [exec_sandbox](#exec_sandbox) |
| `forged_tools` | boolean | `true` | [forged_tools](#forged_tools) |
| `engines` | object | absent (`shared: false`, `required: false`); a clean install writes both true | [engines](#engines) |
| `ceremonies` | object | absent (`profile: auto`) | [ceremonies](#ceremonies) |
| `plan` | object | absent (session model, manual approval) | [plan](#plan) |
| `repair` | object | absent (repairs `underpass-ai/AXLR`) | [repair](#repair) |
| `jobs` | object | absent (`max_active: 2`) | [jobs](#jobs) |
| `jev` | object | absent (off) | [jev](#jev) |
| `trace_retention_days` | integer | absent (30) | [trace_retention_days](#trace_retention_days) |
| `trace_payloads` | boolean | `false` | [trace_payloads](#trace_payloads) |

Several integer keys use `0` for "the default": `prompt_tokens`, `turn_tool_calls`, `repair.watch_minutes`, `repair.max_attempts`, `jobs.max_active`, `jev.timeout_ms`, `local_models[].stream_idle_seconds` and `local_models[].stream_max_minutes`. Their error messages give the range of an explicit value (`between 1 and 720`), but `0` is accepted and means the default. For `context_tokens` and `max_session_usd`, `0` means no limit.

### Model and appearance

#### model

String, a [model id](#model-identifiers). Default empty: the console opens without a model and you choose one with `/model`; startup sends no model request and creates no session until then. An OpenRouter id (`z-ai/glm-5.3-flash`) or the `id` of a [local model](#local_models) (`local/qwen3.8-27b`).

Refused: `invalid settings model`. Written by `/model` (Ctrl+Enter) and the setup assistant. `--model` overrides it for one launch, and a resumed session (`--session`, `-c`) keeps its own saved model.

```json
"model": "z-ai/glm-5.3-flash"
```

A model whose provider is unavailable at launch is not refused here: with no `OPENROUTER_API_KEY` and no `credentials.json` key, a session on an OpenRouter model is refused at its first request.

#### favorite_models

Array of [model ids](#model-identifiers), at most 50. `/model` lists them first. Ctrl+S in `/model` adds the selected model or removes it, and at 50 further additions are ignored. Refused: `invalid favorite model in settings.json`, `settings.json lists too many favorite models`.

```json
"favorite_models": ["z-ai/glm-5.3-flash", "local/qwen3.8-27b"]
```

#### reviewer_model

String, a [model id](#model-identifiers). Default empty: the session's model. The model of the fresh-context review that judges an `/incident` postmortem. Refused: `invalid reviewer_model in settings.json`.

```json
"reviewer_model": "anthropic/claude-haiku-5.5"
```

#### language

`"en"` (default) or `"es"`, exactly; `"es-ES"` or `"ES"` are refused here (`settings language must be en or es`). Changes interface labels only; prompts, tool results and saved conversations are not translated. Written by the setup assistant.

Precedence: `--lang` over `AXLR_LANG` over `language`. The flag and the variable are more lenient than the file: they are lower-cased, a `.UTF-8`-style suffix is dropped and `_` read as `-`, so `es_ES.UTF-8`, `es-MX` and `en-GB` work; anything that is neither English nor Spanish stops the launch with `unsupported language "…" (use en or es)`.

#### theme

One of `"auto"` (default), `"ink"`, `"aurora"`, `"paper"`, `"phosphor"`, `"editorial"`. Editorial is a light theme that also changes the layout (speaker labels, tool runs summarised in one line, the approval as a bottom sheet). Refused: `unknown TUI theme`. Written by `/theme`. `NO_COLOR` (any non-empty value) or `TERM=dumb` turn colour off whatever the theme says; see [appearance and language](console.md#appearance-and-language).

#### icons

`"safe"` (default), `"nerd-mono"` (needs a Nerd Font in the terminal) or `"ascii"`. Refused: `unknown TUI icon profile`. Written by `/theme` (`I`).

#### reduce_motion

Boolean, default `false`. Turns off animation. Written by `/theme` (`A`). A string such as `"yes"` is a type error (`invalid settings.json JSON`).

#### notify

`"bell"` (default; also what an absent or empty value means), `"osc"` or `"off"`. How the console calls you back when a turn of ten seconds or more ends (not one you cancelled) and whenever an approval, plan, postmortem or repair card appears: `bell` rings the terminal bell, `osc` also sends an OSC 9 desktop notification (inside tmux it needs `allow-passthrough`), `off` stays silent. The terminal title is maintained regardless, unless `TERM` is `dumb`. Refused: `settings notify must be "bell", "osc" or "off"`.

### approvals

Object; absent means every tool call that needs a card asks for one, except the tools of an MCP server whose [entry](#plugin-entries) says `"approval": "auto"`. Read and written by the approval policy ([approval_settings.go](../tui/adapters/storage/approval_settings.go)).

| Key | Type | Default | Meaning |
|:--|:--|:--|:--|
| `autonomous` | boolean | `false` | Full autonomy: every known tool the work mode permits runs without a card. Set by `/autonomy on` and `F` on a card, cleared by `/autonomy off` |
| `allowed` | array of tool identities, or `null` | `null` | Exact tools that always run without a card, added by `L` on a card. `/approvals` lists them |
| `always_ask` | array of strings | absent (every rule) | The destructive list (decision K5a of the [lifecycle integration design](plans/2026-10-13-lifecycle-integration-design.md)): the rules that keep a card for a `local_exec` even under autonomy or a saved rule. Rule names: `delete-outside-workspace` (`rm`, `rmdir`, `mv`, `cp`, `dd`… with a path outside the workspace, `~` included, also inside a shell string), `git-push`, `git-reset-hard`, `git-clean`, `git-branch-delete`, `git-rebase`, `network-write` (`curl`/`wget` with a method other than GET or a body, `gh pr create|merge`, `gh release`, `npm`/`cargo` publish, `docker push`, `kubectl apply|delete`, `terraform apply|destroy`, `helm install|upgrade`, deploys). The card says which rule asked. An empty list disables them all |

Unknown keys inside `approvals` are kept, as at the top level. Work modes still restrict or refuse a call whatever these say; see [work modes](console.md#work-modes).

A tool identity is written with the Go field names, capitalised, exactly as the console saves it:

```json
"approvals": {
  "autonomous": false,
  "allowed": [
    {"Kind": "local", "LocalOperation": "read", "Plugin": {"PluginID": "", "ToolName": ""}},
    {"Kind": "plugin", "LocalOperation": "", "Plugin": {"PluginID": "kmp", "ToolName": "kmp_write_memory"}}
  ]
}
```

- `Kind` `local` takes a `LocalOperation` of `read`, `write`, `edit`, `exec`, `search` or `list`, and an empty `Plugin`.
- `Kind` `plugin` takes a `Plugin` with a `PluginID` (`^[A-Za-z][A-Za-z0-9_-]*$`, the MCP registration id) and a `ToolName` (`^[A-Za-z0-9_.-]+$`, at most 128 characters), and an empty `LocalOperation`.
- `Kind` `host` (AXLR's own bookkeeping tools) cannot be always-allowed and is refused.

The empty fields may be omitted when you write the file by hand. Refused: `invalid approved tool identity in settings.json`.

#### approvals.json (legacy)

`<dir of mcp.json>/approvals.json` held these choices before they moved into `settings.json`. It is read only while `settings.json` has no `approvals` section; the next change (`/autonomy`, `L`, `F`) writes the section into `settings.json`, and `approvals.json` is left in place and not read again. Its format is strict: `{"version": 1, "autonomous": false, "allowed": [ … ]}` with the same identities, no other key, version 1, a regular file (not a link) of at most 64 KiB. An invalid one stops the launch (`invalid approval settings file`, `invalid approved tool identity`).

### Context and budget

#### context_tokens

Integer. Absent or `0`: no cap. Otherwise from 4,096 to 16,777,216 (`settings context_tokens must be between 4096 and 16777216`). The context window the console assumes for **every** model: a local model's own `context_tokens` applies when it is smaller; a remote model, whose window is otherwise unknown, is treated as having this window. It therefore also bounds the remote [prompt budget](#prompt_tokens) and, with [`ceremonies.profile: auto`](#ceremonies), a cap of 65,536 or less makes remote sessions compact too. See [local models](console.md#local-models) and [model context](console.md#model-context).

```json
"context_tokens": 65536
```

#### prompt_tokens

Integer. Absent or `0`: 75,000. Otherwise from 16,384 to 16,777,216 (`settings prompt_tokens must be between 16384 and 16777216`). The cap, in tokens, of a request's whole context (guidance, tool schemas and messages) for a model without a known window (every OpenRouter model), counted by the console with one tokenizer (tiktoken's `o200k`) for every model (decision D10 of the [lifecycle integration design](plans/2026-10-13-lifecycle-integration-design.md)); a local model's cap is three quarters of its window. Under the cap nothing is dropped or compacted, so the provider's prompt cache keeps its prefix; the per-result excerpt bounds still derive from this value at the bytes per token measured for the model (2.44 until five requests are measured). A request that would exceed the cap stops the turn, closes the session's MADE instance and continues the prompt in a successor session whose first prompt recalls the predecessor's memory (`ws:<predecessor>`); the footer says so and the previous session stays readable. 75,000 stays under the 100K prompt tokens above which Claude Haiku 5.5 costs five times as much. Local models use their window instead. See [model context](console.md#model-context).

#### turn_tool_calls

Integer. Absent or `0`: 32. Otherwise from 8 to 256 (`settings turn_tool_calls must be between 8 and 256`): below 8 a turn cannot read, edit and check; above 256 one turn could spend a session's budget without you. The tool calls one ordinary turn may make before it pauses; ceremony steps reset the allowance as they advance. Applies to repair, improvement and plan worker sessions as well.

#### max_session_usd

Number, in dollars. Absent or `0`: no limit. Otherwise up to 10,000 (`settings max_session_usd must be between 0 and 10000 dollars`; a larger value is almost certainly a typo). Once a session's known cost reaches it, the console sends no further request. Each session has its own account, worker sessions included. `+` in `/usage` grants one more `max_session_usd` to the current session only, without touching the file. See [cost and budget](console.md#cost-and-budget).

```json
"max_session_usd": 5
```

### local_models

Array of at most 16 objects (`settings.json lists too many local models`): OpenAI-compatible servers (llama.cpp, vLLM, Ollama, LM Studio) that `/model` lists beside OpenRouter's catalog. With at least one entry `OPENROUTER_API_KEY` is optional. Written by the setup assistant, which adds or replaces the entry with the same `id`.

| Key | Type | Default | Limits and meaning |
|:--|:--|:--|:--|
| `id` | string | required | A [model id](#model-identifiers), unique in the list (`settings local_models lists <id> twice`). What the session saves and `/model` shows; the `local/` prefix is a convention that keeps it apart from OpenRouter ids. Must not also be a key of [`models`](#models-per-model-openrouter-options) |
| `name` | string | the `id` | Display name: valid UTF-8, at most 200 bytes, no line breaks |
| `url` | string | required | The server's OpenAI base URL, such as `http://127.0.0.1:8080/v1`. Absolute `http` or `https`, with a host, without user info, query or fragment. Trailing slashes are dropped and `/chat/completions` appended |
| `model` | string | the `id` | A [model id](#model-identifiers): the name sent to the server. vLLM needs its `--served-model-name`; llama.cpp ignores it |
| `api_key_env` | string | none | Name of the environment variable holding the server's key (`^[A-Za-z_][A-Za-z0-9_]*$`). The key is sent as a bearer token when the variable is set |
| `context_tokens` | integer | required | From 4,096 to 16,777,216: the window the console respects for this model, which may be smaller than the server's own. The top-level `context_tokens` lowers it further |
| `tools` | boolean | `true` | `false` hides the model from `/model`: AXLR needs native tool calls and does not degrade without them |
| `stream` | boolean | `true` | `false` asks for whole replies instead of a stream, for servers whose streaming tool-call parser leaks calls as text |
| `thinking` | boolean | unset | `false` sends `chat_template_kwargs: {"enable_thinking": false}`. Unset and `true` both leave the server's default |
| `stream_idle_seconds` | integer | `0` (600) | Seconds without a byte before the stream is abandoned; `0` to 3,600 |
| `stream_max_minutes` | integer | `0` (60) | Minutes a whole reply may take; `0` to 720 |

The long stream defaults are deliberate: a cold prefill of a long prompt on a local GPU takes minutes before the first token, and decoding runs at a few tokens per second.

Refusals while reading the file: `settings local_models entry has an invalid id`, `settings local_models <id> has an invalid name`, `… url must be an absolute http(s) URL without credentials, query or fragment`, `… has an invalid model`, `… api_key_env must be an environment variable name`, `… context_tokens must be between 4096 and 16777216` (also when it is missing), `… stream_idle_seconds must be between 1 and 3600`, `… stream_max_minutes must be between 1 and 720`.

Two more rules are checked when the launch connects to the server, and stop it:

- **Plain `http` only on loopback.** A URL whose host is `localhost`, an address in `127.0.0.0/8` or `::1` may use `http`; any other host must use `https` (`model endpoint must use https unless it is a loopback address`), so a key never crosses the network unencrypted.
- **A key off loopback.** A non-loopback server needs a key: an `api_key_env` naming a variable that is set and not blank. Without one the launch fails, naming the variable when `api_key_env` is given (`local model <id>: … (set <VAR>)`). A loopback server needs no key and gets no `Authorization` header unless its variable holds one.

Every `api_key_env` variable, like `OPENROUTER_API_KEY`, is a **model key**: it is redacted from diagnostics and the app log, never passed to the local commands the model runs, and refused as a source for plugins (see [env and env_from](#env-and-env_from)).

```json
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
    "context_tokens": 65536,
    "stream": false,
    "thinking": false
  }
]
```

Serving is outside AXLR; [local models](console.md#local-models) lists the server flags native tool calls need, and [troubleshooting](troubleshooting.md#local-models) has a smoke test.

### models (per-model OpenRouter options)

Object mapping an exact OpenRouter model id to request options, at most 64 entries (`settings models lists more than 64 models`). The key is the exact id a request names, suffix included: an entry for `z-ai/glm-5.3-flash` does not apply to `z-ai/glm-5.3-flash:nitro`. The entry is added to every OpenRouter request for that model: the session's, a plan's, its workers' and the reviewer's. Without an entry a request carries none of these fields and OpenRouter chooses. Source: [user_settings_models.go](../tui/adapters/storage/user_settings_models.go).

Validation is strict, because a misspelt key would otherwise leave routing as it was without a word. Entries are checked in id order, so the first error is stable. Every refusal begins `settings models <id>` and names the key path (`provider.sort.by`):

- the id must be a [model id](#model-identifiers) and must not be the `id` of a `local_models` entry (`settings models <id> is a local model; provider, reasoning and max_tokens are sent only to OpenRouter`);
- the entry, and every nested object, must be a JSON object whose keys are all known; key names are case-sensitive here;
- no value may be `null` (`… must not be null`).

An entry holds up to three keys:

| Key | Type | Meaning |
|:--|:--|:--|
| `provider` | object | OpenRouter's [provider routing](https://openrouter.ai/docs/features/provider-routing), sent as written; keys below |
| `reasoning` | object | OpenRouter's [reasoning](https://openrouter.ai/docs/use-cases/reasoning-tokens) object; keys below |
| `max_tokens` | integer | Caps the completion: a whole number from 1 to 1,000,000 |

`provider` keys:

| Key | Accepted value |
|:--|:--|
| `order`, `only`, `ignore` | A list of 1 to 64 provider slugs, as openrouter.ai lists a model's endpoints. Each slug non-empty, at most 200 bytes, without surrounding spaces or control characters. An empty list is refused: it orders nothing, and in `only` it would refuse every provider |
| `allow_fallbacks`, `require_parameters`, `zdr` | `true` or `false` |
| `data_collection` | `"allow"` or `"deny"` |
| `quantizations` | A non-empty list of `int4`, `int8`, `fp4`, `mxfp4`, `nvfp4`, `fp6`, `fp8`, `mxfp8`, `fp16`, `bf16`, `fp32`, `unknown` |
| `sort` | `"price"`, `"throughput"` or `"latency"`; or an object `{"by": <one of those>, "partition": <name>}` where `by` is required and `partition` is a non-empty name passed through as OpenRouter defines it |
| `preferred_min_throughput` | A number ≥ 0 (tokens per second) |
| `preferred_max_latency` | A number ≥ 0 (seconds) |
| `max_price` | An object with any of `prompt`, `completion`, `request`, `image`, each a number ≥ 0 in dollars (per million tokens for `prompt` and `completion`): a ceiling an endpoint must not exceed |

`reasoning` keys:

| Key | Accepted value |
|:--|:--|
| `effort` | `"max"`, `"xhigh"`, `"high"`, `"medium"`, `"low"`, `"minimal"` or `"none"` |
| `max_tokens` | A whole number from 1 to 1,000,000; not together with `effort` (`… reasoning sets both effort and max_tokens; OpenRouter takes one of them`) |
| `exclude`, `enabled` | `true` or `false` |

The levers on price and speed are `provider.sort: "throughput"`, `provider.max_price.completion`, `provider.ignore` and the `:nitro` suffix. On 8 October 2026 `z-ai/glm-5.3-flash` without preferences went to OpenInference at $3.109 per million output tokens and about 19 tokens per second, while InferenceNet served it at $0.50 and up to 177 tokens per second. `sort: "price"` is accepted but OpenRouter does not document which price it sorts by. [Per-model request options](console.md#per-model-request-options) tells the whole case.

```json
"models": {
  "z-ai/glm-5.3-flash": {
    "provider": {"sort": "throughput", "ignore": ["openinference"], "max_price": {"completion": 1}},
    "reasoning": {"effort": "low", "exclude": true},
    "max_tokens": 8192
  },
  "anthropic/claude-haiku-5.5": {
    "reasoning": {"max_tokens": 2048}
  }
}
```

### Tools and safety

#### exec_sandbox

Object; absent means off. Confines every `local_exec`, and every check, forge and git command the console runs through the same runtime, under bubblewrap on Linux. Source: [user_settings_sandbox.go](../tui/adapters/storage/user_settings_sandbox.go), [run_sandbox.go](../tui/cmd/axlr-tui/run_sandbox.go).

| Key | Type | Default | Meaning |
|:--|:--|:--|:--|
| `mode` | `"off"` \| `"auto"` \| `"required"` | `"off"` (also for `""`) | `auto` confines when the platform can and runs commands unconfined otherwise, saying why on stderr and in the header; `required` refuses every local command when it cannot confine |
| `network` | boolean | `true` | `false` gives confined commands an empty network namespace, which also stops `git push` and `gh` in `/repair` and `/improve` |
| `writable` | array of strings | `[]` | Paths outside the workspace that confined commands may write, such as build caches. At most 32; each absolute, already clean (`/tmp/../etc` is refused) and not `/` |

Refused while reading: `settings exec_sandbox.mode must be off, auto or required`, `settings exec_sandbox.writable lists more than 32 paths`, `settings exec_sandbox.writable "<path>" must be a clean absolute path other than the root`. When the mode is not `off`, the launch also checks that each `writable` path exists (`exec_sandbox.writable <path>: …`). The default is `off` because a read-only home breaks common checks until their caches are listed. See [exec sandbox](console.md#exec-sandbox).

```json
"exec_sandbox": {
  "mode": "auto",
  "network": true,
  "writable": ["/home/you/.cache/go-build", "/home/you/go/pkg/mod"]
}
```

#### forged_tools

Boolean, default `true` (absent means on; an explicit `false` is kept when other keys are saved). `false` offers the model none of `axlr_forge_tool`, `axlr_run_tool` and `axlr_remove_tool`. See [forged tools](console.md#forged-tools).

### Engines, ceremonies and jobs

These keys matter only when the MCP registrations with the ids [`kmp` and `made`](#the-kmp-and-made-ids) are connected.

#### engines

| Key | Type | Default | Meaning |
|:--|:--|:--|:--|
| `required` | boolean | `false` | Refuse to start without KMP and MADE connected (decision A2): a launch that lacks one installs and configures it first, without a question, and stops with a message naming `axlr-tui setup` when that fails. A clean install writes `true` |
| `shared` | boolean | `false` | Run one KMP and one MADE process per identical launch (same command, arguments and whole environment) for every console of the user, through a small daemon and a private unix socket, instead of one per console. Unix only |

See [shared engines](console.md#shared-engines). Source: [user_settings_engines.go](../tui/adapters/storage/user_settings_engines.go).

#### ceremonies

| Key | Type | Default | Meaning |
|:--|:--|:--|:--|
| `profile` | `"auto"` \| `"standard"` \| `"compact"` | `"auto"` | The profile of the driven ceremonies. `auto` is compact when the session model's known window is at most 65,536 tokens: a local model's window, or the top-level `context_tokens` cap for any model. An OpenRouter model without a cap has no known window and gets `standard` |

Refused: `settings ceremonies.profile must be auto, standard or compact`. See [the compact profile](ceremonies.md#the-compact-profile-for-small-models).

#### plan

| Key | Type | Default | Meaning |
|:--|:--|:--|:--|
| `model` | string | `""` (the session's model) | The model that decomposes a brief in `/plan`: a [model id](#model-identifiers), or `"session"`, which means the session's model as well |
| `auto_approve` | boolean | `false` | Start a verified plan without your approval |

Refused: `settings plan.model must be a model id or session`. If `plan.model` is an OpenRouter model and there is no OpenRouter key, the launch says so on stderr (`plan.model <id> needs OPENROUTER_API_KEY; /plan uses the session model`) and uses the session's model. The default was `z-ai/glm-5.3-flash` until 9 October 2026, when its decompose request took 138 seconds. See [plans](ceremonies.md#plans-atomic-tasks-for-small-models).

#### repair

Configures `/repair`, the agent's self-repair (`axlr_request_repair`), `/improve` and `--repair`/`--improve`. `/improve` uses `repository`, `directory`, `watch_minutes` and `about`; `auto_merge` never applies to an improvement.

| Key | Type | Default | Limits and meaning |
|:--|:--|:--|:--|
| `repository` | string | `"underpass-ai/AXLR"` | `owner/name`, each part made of letters, digits, `-`, `_` and `.`. The repository the console may repair |
| `directory` | string | `<data>/repairs` | Absolute path where clones are made |
| `auto_merge` | boolean | `false` | Merge a green repair pull request without you |
| `watch_minutes` | integer | `0` (45) | How long one round of checks may take; `0` to 720 |
| `about` | string | `project:<name>` | The KMP project scope of repairs: the repository name with its case (`underpass-ai/AXLR` gives `project:AXLR`). Not validated |
| `autonomous` | boolean | `true` | Let a repair session the agent started run local tools in its clone without a card. The reproduction command and the merge always wait for you; an explicit `false` is kept on save |
| `max_attempts` | integer | `0` (2) | Repair sessions one failure may start; `0` to 5 |
| `install` | boolean | `false` | Replace the running `axlr-tui` with the repaired build once its pull request merges, without the panel's `i` |

Refused: `settings repair.repository must be owner/name`, `settings repair.directory must be an absolute path`, `settings repair.watch_minutes must be between 1 and 720`, `settings repair.max_attempts must be between 1 and 5`. See [self-repair](ceremonies.md#self-repair-the-console-drives-the-pull-request).

```json
"repair": {
  "repository": "underpass-ai/AXLR",
  "directory": "/home/you/repairs",
  "auto_merge": false,
  "watch_minutes": 45,
  "about": "project:AXLR",
  "autonomous": true,
  "max_attempts": 2,
  "install": false
}
```

#### jobs

| Key | Type | Default | Meaning |
|:--|:--|:--|:--|
| `max_active` | integer | `0` (2) | Repair and improvement sessions active at once across every console that shares `<state>/repairs.json`, whoever started them; `0` to 8 |

Refused: `settings jobs.max_active must be between 1 and 8`. Each job runs a model session and its checks in its own clone; eight at once were already more than one machine held on 9 October 2026. See [jobs and the merge queue](ceremonies.md#jobs-and-the-merge-queue).

### jev

TypeSafe Jev, an external judgement model. Absent, or with both switches off, nothing is sent to TypeSafe. Source: [user_settings.go](../tui/adapters/storage/user_settings.go), [typesafe/client.go](../tui/adapters/typesafe/client.go).

| Key | Type | Default | Limits and meaning |
|:--|:--|:--|:--|
| `tool` | boolean | `false` | Offer the model `axlr_judge` |
| `final_check` | boolean | `false` | Ask Jev, once per request, whether the final answer completes it |
| `final_threshold` | number | `0` (0.5) | From 0 up to but not including 1: the probability below which a final answer goes back to the model. `0` means the default 0.5 |
| `model` | string | `"jev-1.13.0"` | The pinned Jev version: 1 to 64 letters, digits, `.`, `_` or `-`, and not a `-latest` alias |
| `timeout_ms` | integer | `0` (20,000) | One request's timeout; `0`, or 1,000 to 60,000 |

Refused while reading: `settings jev.final_threshold must be between 0 and 1`, `settings jev.timeout_ms must be between 1000 and 60000`. `model` is checked only when a switch is on, at launch (`jev: jev.model must be a pinned version such as jev-1.13.0`). With either switch on, `TYPESAFE_API_KEY` must be set or the launch stops (`jev: TYPESAFE_API_KEY is required for Jev`), and the launch says on stderr that questions and final answers are sent to TypeSafe. See [Jev](console.md#jev-an-external-judge-off-by-default).

```json
"jev": {"tool": true, "final_check": true, "final_threshold": 0.5, "model": "jev-1.13.0", "timeout_ms": 20000}
```

### Diagnostics

#### trace_retention_days

Integer. Absent: 30. `0` keeps every trace. Otherwise up to 36,500 (`settings trace_retention_days must be between 0 and 36500`); an explicit `0` is kept on save. A launch with the default trace silently deletes what earlier launches left in `<state>/logs/` once unchanged for this many days: `trace-<pid>-<n>.jsonl` files and payload captures. A trace chosen with `--trace-file`, the current launch's files and the app log are never deleted.

#### trace_payloads

Boolean, default `false`. `true` stores redacted HTTP request and response bodies, prompts and tool results included, in a private directory beside the trace on every launch. `--trace-payloads` or `--trace-payloads=false`, when given, decides for that launch either way. Capture became opt-in after the default `logs/` directory held 295 MB of bodies on 9 October 2026. See [diagnostics and privacy](console.md#diagnostics-and-privacy).

### Flags and environment

Command-line flags and environment variables win over `settings.json` for one launch and never change it.

| Flag | Overrides or adds |
|:--|:--|
| `--model ID` | `model`, for this launch. With `--session` it must equal the saved session's model |
| `--lang en\|es` | `AXLR_LANG` and `language` |
| `--trace-payloads[=true\|false]` | `trace_payloads`, either way, when present |
| `--trace-file PATH` | The default trace path; retention does not apply to it |
| `--mcp-config PATH` | The MCP configuration (absolute, must exist) and the directory of `approvals.json`, `engines/` and `plugins/`; not `settings.json` |
| `--plugin PATH` | Adds an MCP manifest for this launch, with purpose `tools` and approval `manual`; not saved |
| `--plugin-env-from ID:KEY=HOST_VAR` | Child environment for a `--plugin` server; refuses `OPENROUTER_API_KEY` and every `api_key_env` as `HOST_VAR`, and an `ID` no `--plugin` registers |
| `--root`, `--session`, `-c`/`--continue`, `--repair`, `--improve` | Choose the workspace and session; not configuration. `--repair` and `--improve` use the `repair` section |
| `--import FILE` | Save the session bundle `/session export` wrote on another console (or this one) in this store, bound to `--root`, and resume it; refused when the session is saved here already (resume it with `--session`) and when combined with `--session`, `--continue`, `--repair` or `--improve`. Its MADE instance continues in a successor and the handoff is recorded in KMP under `ws:<id>` |

| Variable | Effect |
|:--|:--|
| `OPENROUTER_API_KEY` | The OpenRouter key. Wins over [`credentials.json`](#credentialsjson). Never passed to plugins or local commands; redacted from diagnostics |
| each `local_models[].api_key_env` | A local server's key; treated like `OPENROUTER_API_KEY` |
| `TYPESAFE_API_KEY` | Jev's key, required when `jev` is on. Redacted, but still available to plugins: KMP reads it for its own Jev features |
| `AXLR_LANG` | Interface language; between `--lang` and `language` |
| `NO_COLOR` (non-empty), `TERM=dumb` | Monochrome output; `TERM=dumb` also leaves the terminal title alone. No settings equivalent |
| `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_DATA_HOME`, `XDG_RUNTIME_DIR`, `HOME` | [Where the files live](#where-the-files-live) |

Local commands (`local_exec`, checks, forged tools) receive only an absolute `PATH` and `HOME`, whatever the shell exports. MCP servers receive only what [`env` and `env_from`](#env-and-env_from) give them.

## credentials.json

`<config>/credentials.json`, beside `settings.json`, holds the OpenRouter key the setup assistant saved for a person who does not export `OPENROUTER_API_KEY`. Source: [credentials.go](../tui/adapters/storage/credentials.go).

```json
{
  "openrouter_api_key": "sk-or-v1-…"
}
```

| Key | Type | Meaning |
|:--|:--|:--|
| `openrouter_api_key` | string | The key: non-empty, at most 512 bytes, no spaces or control characters anywhere |

- **Precedence.** When `OPENROUTER_API_KEY` is set and non-empty, the file is not read at all. Otherwise its key is used as if the variable held it.
- **Privacy.** A regular file, not a link, with no permission bits for group or others (`0600` or `0400`). Otherwise the launch stops: `<path> holds a key and must be a regular file readable only by you; run chmod 600 <path>`. The key is redacted from the app log and diagnostics like the variable, and it is never given to plugins, local commands or the model: plugins receive only their explicit environment, and this key is not an environment variable.
- **Format.** At most 16 KiB (`credentials.json exceeds 16 KiB`); one JSON object with no other key and nothing after it (`invalid credentials.json JSON`); a malformed key gives `credentials.json has an invalid openrouter_api_key`. A missing file means no saved key.
- **Writes.** Only the setup assistant writes it: atomically, through a `.axlr-credentials-*` temporary file of mode `0600`, synced and renamed, the directory created `0700`. Delete the file, or the key in it, to forget the key; exporting `OPENROUTER_API_KEY` overrides it without deleting it.

### The setup assistant

On the first launch in an interactive terminal, when `settings.json` does not exist yet, no legacy `model-preference.json` names a model, and neither `--model`, `--session`, `-c`, `--repair` nor `--improve` was given, the console runs a short assistant before opening; `axlr-tui setup` runs it again on demand. It asks for the language, the OpenRouter key or a local server (it probes the usual loopback ports of llama.cpp, vLLM, Ollama and LM Studio) and the model, and can install KMP and MADE. It writes `language` as soon as it is chosen, so `settings.json` exists from then on and a later launch does not ask again; then `model` and a `local_models` entry, the key to `credentials.json` (only a key it typed and OpenRouter accepted, never one from the environment), and the engines' entries to [`mcp.json`](#mcpjson). Its source is [setup.go](../tui/cmd/axlr-tui/setup.go); [getting started](getting-started.md#2-start-a-console) walks through it.

## mcp.json

`mcp.json` lists the MCP servers the console connects at startup and the approval policy of each. Path: `<config>/mcp.json`, or the absolute path given with `--mcp-config` (which must exist; the default may be missing, meaning no persistent servers). Sources: [mcp_config.go](../tui/adapters/storage/mcp_config.go), [mcp_config_store.go](../tui/adapters/storage/mcp_config_store.go), [mcp_plugin_config.go](../tui/dto/mcp_plugin_config.go). [Plugins](plugins.md) explains how servers are added from the console.

### File requirements

Stricter than `settings.json`:

- Opened without following links; a regular file with no permission bits for group or others, at most 64 KiB. Each failure names the file and the fix: `MCP config <path> has mode 0664 and must not be readable or writable by other accounts; run chmod 600 <path>` (the case of a file copied under umask 002 on 10 October 2026), `… is not a regular file; replace it with a regular file of mode 0600`, `… is <n> bytes; the limit is 65536 bytes (64 KiB)`.
- Valid UTF-8 (`invalid MCP config size or UTF-8`); one JSON object with **no unknown field** at any level and nothing after it (`invalid MCP config JSON`).
- `version` must be `1`, and `plugins` may hold at most 32 entries (`invalid MCP config version or plugin count`).
- Each entry's manifest must load, and no two entries may resolve to the same manifest `id` (`duplicate MCP plugin ID`).

Writes (`/mcp` approval changes, URL installs, `/plugin` packages, `/update`, `P` on MADE) take an exclusive lock on `mcp.json.lock` (`0600`, private, like the settings lock), re-read the file, and replace it atomically through a `.axlr-mcp-*` temporary file of mode `0600`. The console rewrites the whole file, two-space indented.

### Top level

```json
{
  "version": 1,
  "plugins": [ ]
}
```

| Key | Type | Meaning |
|:--|:--|:--|
| `version` | integer | Must be `1` |
| `plugins` | array | At most 32 [entries](#plugin-entries); `null` or `[]` means none |

### Plugin entries

| Key | Type | Default | Meaning |
|:--|:--|:--|:--|
| `manifest` | string | required | Absolute path of an [MCP manifest](#mcp-manifests). The registration id is the manifest's `id`, not anything in this entry |
| `name` | string | the manifest `id` | Display name in `/mcp`; valid UTF-8 without NUL |
| `description` | string | `""` | Shown in `/mcp`; valid UTF-8 without NUL |
| `purpose` | `"tools"` \| `"memory"` \| `"ceremony"` | `"tools"` | The capability the server contributes, shown in `/mcp` (`invalid plugin purpose`). What the console drives depends on the [id](#the-kmp-and-made-ids), not on this field |
| `approval` | `"manual"` \| `"auto"` | `"manual"` | `manual` asks for every call of this server's tools; `auto` runs its allowed tools without a card, future tools included when the manifest allows `*`. Changed from `/mcp` (`A` or Enter, then confirm) (`invalid plugin approval mode`) |
| `env` | object of string → string | `{}` | Literal environment values for the server process |
| `env_from` | object of string → string | `{}` | Environment variables copied from AXLR's own environment: key is the name in the child, value the host variable's name |

`env` and `env_from` together hold at most 64 entries (`too many MCP plugin environment entries`).

### env and env_from

For a stdio server, `env` and `env_from` form the **complete** environment of the child process. Nothing is inherited: an entry with neither map starts the server with an empty environment, without even `PATH` or `HOME`. Add `"env_from": {"HOME": "HOME", "PATH": "PATH"}` when the server needs them, as most launchers do.

- Names, in both maps and as `env_from` sources, must match `^[A-Za-z_][A-Za-z0-9_]*$`; an `env` value must not contain NUL. Otherwise `invalid MCP plugin environment` or `invalid MCP plugin environment source`.
- A name may not appear in both maps (`duplicate MCP plugin environment key`).
- `env_from` is resolved at launch. A host variable that is not set gives the child an **empty** value, not an error.
- `OPENROUTER_API_KEY` is refused as an `env` name and as an `env_from` name or source. Every local model's `api_key_env` is refused as an `env_from` source: the launch stops with `model key <VAR> cannot be passed to plugins`. `TYPESAFE_API_KEY` is allowed.
- A Streamable HTTP server (a manifest with `url`) accepts no environment at all: any `env` or `env_from` entry fails its registration (`invalid MCP URL registration`).
- `P` on MADE persists `MADE_AUTH_TRUSTED_HOST_ID` as a literal `env` value. It refuses to replace a key that `env_from` resolves (`MCP environment key is resolved from the host environment`).

Prefer `env_from` for secrets, so the value stays in your secret manager rather than in the file. The app log redacts the values of plugin variables whose names look secret (`KEY`, `TOKEN`, `SECRET`, `PASS`, `AUTH`, `CREDENTIAL`, `COOKIE`).

### The kmp and made ids

Two registration ids, taken from the manifest's `id`, change what the console does:

| id | Turns on |
|:--|:--|
| `kmp` | `axlr_remember`, ceremony memory, the KMP guide root and the memory reminder; see the [KMP runbook](runbooks/kmp.md) |
| `made` | The driven ceremonies (`/debug`, `/delivery`, `/incident`, `/repair`, `/improve`), `/plan` with MADE plans, self-repair, `/jobs`, and `P` in `/mcp`; see the [MADE runbook](runbooks/made.md) |

Package-generated registrations are usually named `<package>-<server>` and do not count. `/update` considers only the entries whose manifest id is `kmp` or `made`, and updates one only when its manifest has a `command` and no `args`; a URL engine or a custom argument list is left to its owner. An update writes a new manifest to `<dir of mcp.json>/engines/<id>-<random>.json` pointing at the new launcher, keeps the original `allow_tools`, and switches only that entry's `manifest`; its name, purpose, approval and environment are kept. `/update` and the setup assistant also install an official package that `mcp.json` lacks: they prepare its store, write `<dir of mcp.json>/engines/<id>-<random>.json` and append the entries below; an existing registration is never replaced.

### Full examples

The KMP entry the installer writes (`purpose` `memory`, the embedded backend pinned, `HOME` and `PATH` passed through; no store, which the console chooses per workspace at launch and pins into the child environment itself). Its `approval` is `auto` from `/update` and from the assistant unless you answer no to recording memories without a card, which gives `manual`:

```json
{
  "manifest": "/home/you/.config/axlr/engines/kmp-1234567890.json",
  "name": "KMP",
  "description": "Underpass graph-temporal agent memory",
  "purpose": "memory",
  "approval": "auto",
  "env": {"KMP_MCP_BACKEND": "embedded"},
  "env_from": {"HOME": "HOME", "PATH": "PATH"}
}
```

An entry written before 13 October 2026 also carries `"KMP_MCP_DATA_DIR": "/home/you/.local/share/kmp/default"`; the console treats that value as the old pin and still opens a checkout's `.kernel`. Any other directory in `KMP_MCP_DATA_DIR` is yours and wins.

A MADE entry for the installed embedded launcher, which finds its private store configuration through `HOME`:

```json
{
  "manifest": "/home/you/.config/axlr/engines/made-0987654321.json",
  "name": "MADE",
  "description": "Underpass agentic ceremonies, structured reviews and human approval procedures",
  "purpose": "ceremony",
  "approval": "manual",
  "env_from": {"HOME": "HOME", "PATH": "PATH"}
}
```

After `P`, the MADE entry gains `"env": {"MADE_AUTH_TRUSTED_HOST_ID": "axlr-work-…"}`. A directly managed `made-mcp` needs its store, policy, identity and cursor key spelled out; the [MADE runbook](runbooks/made.md#direct-binary-or-remote-operator-setup) has that entry. A complete file with both engines and a third-party server:

```json
{
  "version": 1,
  "plugins": [
    {
      "manifest": "/home/you/.config/axlr/kmp.json",
      "name": "KMP",
      "purpose": "memory",
      "approval": "manual",
      "env": {"KMP_MCP_BACKEND": "embedded", "KMP_MCP_DATA_DIR": "/home/you/memory/axlr"},
      "env_from": {"HOME": "HOME", "PATH": "PATH"}
    },
    {
      "manifest": "/home/you/.config/axlr/made.json",
      "name": "MADE",
      "purpose": "ceremony",
      "approval": "manual",
      "env_from": {"HOME": "HOME", "PATH": "PATH"}
    },
    {
      "manifest": "/home/you/.config/axlr/plugins/notes.json",
      "name": "Notes",
      "description": "Team notes over HTTP",
      "purpose": "tools",
      "approval": "manual"
    }
  ]
}
```

## MCP manifests

An MCP manifest describes how to reach one server and which of its tools AXLR may call. `mcp.json` entries and `--plugin` point at manifests by absolute path. Source: [manifest.go](../plugins/manifest.go), [registration.go](../plugins/registration.go).

```json
{
  "manifest_version": 1,
  "id": "kmp",
  "command": "/home/you/.local/bin/kmp-mcp",
  "args": [],
  "allow_tools": ["*"]
}
```

| Key | Type | Required | Meaning |
|:--|:--|:--|:--|
| `manifest_version` | integer | yes | Must be `1` (`unsupported plugin manifest version`) |
| `id` | string | yes | The registration id: `^[A-Za-z][A-Za-z0-9_-]*$`. `kmp` and `made` are [special](#the-kmp-and-made-ids) |
| `command` | string | one of `command`, `url` | Absolute path of a stdio server's executable. No `PATH` lookup, no shell |
| `args` | array of strings | with `command` | The arguments, each a JSON string without NUL. Required when `command` is set, even if empty (`plugin args must be an array`) |
| `url` | string | one of `command`, `url` | A Streamable HTTP endpoint: absolute `http` or `https`, with a host, no user info and no fragment (a query is allowed). No `args` and no environment with it (`invalid MCP URL`) |
| `allow_tools` | array of strings | yes | The tools AXLR may call: `["*"]` for every tool the server advertises, or a list of distinct tool names (`^[A-Za-z0-9_.-]+$`, up to 128 characters). Empty is refused |

Exactly one of `command` and `url` (`plugin requires one MCP transport and nonempty allow_tools`). Any other key is refused (`unknown plugin manifest field "…"`), as is a key given twice, trailing JSON, or a file over 64 KiB or not UTF-8. There is no `limits` key or any other: timeouts and output limits are AXLR's own. The manifest's mode is not checked, but keep it private like the files that point to it.

The manifests AXLR writes itself: `engines/<id>-<random>.json` by `/update` (stdio, `"args": []`), and `plugins/<id>.json` by `/mcp` when you add a server by URL, created exclusively with mode `0600` and `"allow_tools": ["*"]`.

## Complete settings.json example

Every key with a value. The file passes validation as written; to launch with it, also export `TEAM_LLM_KEY` for the third local model, or drop that entry. Nothing is required; a real file usually holds a handful of these.

```json
{
  "model": "z-ai/glm-5.3-flash",
  "favorite_models": ["z-ai/glm-5.3-flash", "anthropic/claude-haiku-5.5", "local/qwen3.8-27b"],
  "reviewer_model": "anthropic/claude-haiku-5.5",
  "language": "es",
  "theme": "auto",
  "icons": "safe",
  "reduce_motion": false,
  "notify": "bell",
  "approvals": {
    "autonomous": false,
    "allowed": [
      {"Kind": "local", "LocalOperation": "read", "Plugin": {"PluginID": "", "ToolName": ""}},
      {"Kind": "plugin", "LocalOperation": "", "Plugin": {"PluginID": "kmp", "ToolName": "kmp_ask"}}
    ]
  },
  "context_tokens": 131072,
  "prompt_tokens": 64000,
  "turn_tool_calls": 32,
  "max_session_usd": 5,
  "local_models": [
    {
      "id": "local/qwen3.8-27b",
      "name": "Qwen3.8-27B (llama.cpp)",
      "url": "http://127.0.0.1:8080/v1",
      "context_tokens": 65536,
      "tools": true,
      "stream_idle_seconds": 600,
      "stream_max_minutes": 60
    },
    {
      "id": "local/gemma-4-31b",
      "name": "Gemma 4 31B (vLLM)",
      "url": "http://127.0.0.1:8082/v1",
      "model": "gemma-4-31b",
      "context_tokens": 65536,
      "stream": false,
      "thinking": false
    },
    {
      "id": "local/team-server",
      "name": "Team GPU server",
      "url": "https://llm.example.com/v1",
      "model": "qwen3.8-27b",
      "api_key_env": "TEAM_LLM_KEY",
      "context_tokens": 131072
    }
  ],
  "models": {
    "z-ai/glm-5.3-flash": {
      "provider": {"sort": "throughput", "ignore": ["openinference"], "max_price": {"completion": 1}},
      "reasoning": {"effort": "low", "exclude": true},
      "max_tokens": 8192
    },
    "anthropic/claude-haiku-5.5": {
      "provider": {"data_collection": "deny", "zdr": true},
      "reasoning": {"max_tokens": 2048}
    }
  },
  "exec_sandbox": {
    "mode": "auto",
    "network": true,
    "writable": ["/home/you/.cache/go-build", "/home/you/go/pkg/mod"]
  },
  "forged_tools": true,
  "engines": {"shared": true},
  "ceremonies": {"profile": "auto"},
  "plan": {"model": "session", "auto_approve": false},
  "repair": {
    "repository": "underpass-ai/AXLR",
    "directory": "/home/you/repairs",
    "auto_merge": false,
    "watch_minutes": 45,
    "about": "project:AXLR",
    "autonomous": true,
    "max_attempts": 2,
    "install": false
  },
  "jobs": {"max_active": 2},
  "jev": {"tool": false, "final_check": false, "final_threshold": 0.5, "model": "jev-1.13.0", "timeout_ms": 20000},
  "trace_retention_days": 30,
  "trace_payloads": false
}
```

Notes on the example:

- **Model and appearance.** The default model is GLM 5.3 Flash on OpenRouter; the two favorites and a local model are listed first in `/model`. Postmortems are reviewed by Claude Haiku 5.5. Spanish labels, automatic theme, safe icons, animations on, the bell when a long turn ends.
- **Approvals.** Manual approval, except reads (which never need a card anyway) and KMP's `kmp_ask`, which were always-allowed from their cards.
- **Context.** `context_tokens` 131,072 caps every model's window; the two loopback models keep their own 65,536. `prompt_tokens` and `turn_tool_calls` repeat their defaults. A session stops sending requests at $5.
- **Local models.** llama.cpp on port 8080 with the stream defaults written out; vLLM on 8082 with its served model name, whole replies and thinking off; a remote team server over `https`, whose key the launch reads from `TEAM_LLM_KEY` and stops if it is unset. None of them appears under `models`, which would be refused.
- **models.** GLM is routed by throughput, away from OpenInference and from any endpoint charging more than $1 per million output tokens, with low reasoning effort hidden from the reply and completions capped at 8,192 tokens. Haiku is restricted to endpoints that neither collect data nor retain it, with a 2,048-token reasoning budget (a budget, so no `effort`).
- **Safety and engines.** Commands are confined where bubblewrap works, with Go's caches writable and the network kept. Forged tools are on. Consoles share one KMP and one MADE process.
- **Ceremonies.** Compact profile for small windows (with the 131,072 cap, remote sessions stay standard), plans decomposed by the session's model and approved by you, repairs of AXLR itself in `/home/you/repairs` with every default spelled out, two jobs at once.
- **Jev and diagnostics.** Jev is off, so `TYPESAFE_API_KEY` is not needed; its other values are the defaults. Traces older than 30 days are pruned and request bodies are not captured.
