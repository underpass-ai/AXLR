# Runbook: connect or remove KMP memory

KMP is AXLR's governed, durable memory layer. AXLR executes the agent turn and calls KMP over MCP for evidence-backed recall and writes. The built-in KMP row in `/plugin` is a product catalogue entry, not proof that an engine is installed or connected. `/mcp` shows the active connection.

This runbook uses a directly managed local `kmp-mcp` stdio process. See the [KMP embedded guide](https://github.com/underpass-ai/kmp/blob/main/docs/embedded/README.md) for engine releases, store selection, doctor output and upstream lifecycle details. Use one KMP registration per AXLR host.

## Preconditions and exit criteria

Use an engine supported by your platform and store format; the service distribution locks the KMP release in [engines.lock.json](../../distribution/engines.lock.json), the only place the version is stated. Record the binary version, absolute store path and owning account before connecting. A package version, a running process and the selected store are separate checks.

Success means AXLR discovers the intended `kmp` registration, can retrieve its guide and can recover a known memory from the intended store. A new project may legitimately have no memories. A successful connection alone does not prove that you selected the right data.

## Let AXLR install it

On Linux x86_64/arm64 and macOS on Apple silicon, the first-run setup assistant (`axlr-tui setup`) or `/update` in the console installs KMP when `mcp.json` has no `kmp` entry. It downloads and verifies the official release into `$XDG_DATA_HOME/axlr/engines/` and registers the package launcher with `KMP_MCP_BACKEND=embedded`, `env_from {HOME, PATH}` and the approval you chose: `auto` by default in the assistant and in `/update`. It pins no store: the console chooses one per workspace at every launch and says so in its startup line (`axlr-tui: KMP store … (…), about …`). Inside a git checkout the store is the checkout's own `.kernel`, the directory KMP's other hosts open for the same repository; outside one it is the per-user default `$XDG_DATA_HOME/kmp/default`. A `KMP_MCP_DATA_DIR` in the entry's `env` that names any other directory is yours and wins; one that names the per-user default is the pin the installer wrote until 13 October 2026, and a checkout overrides it. A store the console creates gets KMP's self-ignoring `.gitignore`, the repository's committed bundle `.kmp/memory.jsonl` imported when there is one (`kmp-mcp import` needs an empty store, so this happens first), the configuration files of your per-user store (`typesafe.json`, `rerank.json`, `wake-focus.json`, `write-relations.json`, `judgement-book.json`, `curate.json`, `ask-judge.json`, `write-expansions.json`, `telemetry-salt`) when it has them, and the guide, all before the engine starts. An existing store is left as it is, its guide included. The console never exports or commits the bundle: `/memory save` writes the project about to `<checkout>/.kmp/memory.jsonl` with `kmp-mcp export` on demand (a live engine does not block it, and the file is replaced), and whether `.kmp/` is tracked is the person's decision. To keep another store, set `KMP_MCP_DATA_DIR` in the entry as below.

## Install and connect by hand

1. Install a release-matched `kmp-mcp` binary using the upstream KMP instructions. If no published asset exists for your platform, KMP documents `cargo install kmp-mcp --locked`. Record its **absolute** executable path and check `kmp-mcp --version` and `kmp-mcp doctor`.
2. Choose the store deliberately. Without `KMP_MCP_DATA_DIR` in the entry the console chooses per workspace, as [above](#let-axlr-install-it): the checkout's `.kernel`, or the per-user default outside one. `KMP_MCP_DATA_DIR` naming any other absolute directory is the most explicit selection and wins for every workspace; one naming the per-user default is treated as the old installer pin. Check a store with `KMP_MCP_DATA_DIR=/absolute/path/to/memory kmp-mcp info` before any write. Do not create a new empty store accidentally when you meant to resume an existing one.
3. Save this manifest as a private file at an absolute path, for example `$HOME/.config/axlr/kmp.json`, replacing the command path:

```json
{
  "manifest_version": 1,
  "id": "kmp",
  "command": "/absolute/path/to/kmp-mcp",
  "args": [],
  "allow_tools": ["*"]
}
```

4. Add one entry to AXLR's private `$XDG_CONFIG_HOME/axlr/mcp.json` (or `$HOME/.config/axlr/mcp.json`) `plugins` array, preserving any existing entries and mode `0600`:

```json
{
  "manifest": "/absolute/path/to/kmp.json",
  "name": "KMP",
  "purpose": "memory",
  "approval": "manual",
  "env": {"KMP_MCP_BACKEND": "embedded", "KMP_MCP_DATA_DIR": "/absolute/path/to/memory"}
}
```

Use `{"version":1,"plugins":[...]}` as the top-level shape if this is the first server. The child environment is explicit; set every variable the engine needs there or through `env_from`. Configure the intended store **before first discovery** so AXLR cannot select a different default store by accident.

5. Restart AXLR. Open `/mcp`, refresh and verify that `kmp` lists tools. Ask the agent to read KMP's guide and perform a read-only `kmp_wake` or `kmp_ask`. On a fresh store, synchronize the matching KMP guide assets using KMP's documented guide workflow before relying on guided recall. Approval remains manual until you deliberately change `kmp`'s policy in `/mcp`.

For a one-launch check without changing persistent configuration, keep the same explicit store:

```bash
AXLR_KMP_DATA_DIR=/absolute/path/to/memory \
  axlr-tui --root /absolute/workspace --plugin /absolute/path/to/kmp.json \
  --plugin-env-from 'kmp:KMP_MCP_DATA_DIR=AXLR_KMP_DATA_DIR'
```

The worker also accepts `--plugin` for a one-request MCP call; supply the same store through `--plugin-env`. Its process lifetime ends after that response.

## Verify guide and store selection

Run inspection with the same binary, backend and explicit store environment used by the registration. A saved global selection can point at an unrelated project, and `--root` controls AXLR's workspace rather than overriding KMP's store. Pin `KMP_MCP_DATA_DIR` before discovery; do not write until the selected path and a known memory agree.

A store without the guide (a new store, as on 8 October 2026; an existing `.kernel` the console did not create) refuses `kmp_guide` with "cannot serve guide node… Run `kmp-mcp guide sync --plugin-root`". Since 13 October 2026 the console answers that itself: when its own registration or context call meets the refusal, it runs the sync with the directory of the engine it installed (which holds `guide/guide.requests.json`) and tries again; a live writer on the store does not block it. By hand, with any writer stopped:

```bash
KMP_MCP_BACKEND=embedded KMP_MCP_DATA_DIR=/absolute/path/to/memory \
  /absolute/path/to/kmp-mcp guide sync --plugin-root /absolute/path/to/matching-kmp-plugin
```

This writes guide data to the selected store. Restart AXLR with the same selection and check the app log for the registration and the known-memory read. Do not repair a missing guide by silently switching to another store. Upstream tool feedback is authoritative for a different engine version or backend.

## What the model sees

The console runs KMP's protocol; the model never registers, wakes or pages KMP for bookkeeping (decision D3 of the [lifecycle integration design](../plans/2026-10-13-lifecycle-integration-design.md)):

- **Identity.** The console registers one agent per store with the install's key (`axlr:<install id>`, from `<state>/axlr/install-id`) and keeps the agent id per store in `<state>/axlr/kmp-agents.json`; a store that forgot the agent (emptied, replaced) is registered again. At a session's first prompt it opens that session's context (`kmp_guide` with the agent and the key `axlr:session:<id>`) and keeps it in the session's `.instance` file. Every KMP call of the session carries that context: the console's turn entries, `axlr_remember`, `axlr_recall` and the model's own `kmp_*` calls through `axlr_call_tool`, whose `context_id` the console sets (one the model names is replaced). A store without the guide is synced once (`kmp-mcp guide sync --plugin-root`) and the call tried again. `kmp_guide` itself is the console's: `axlr_tools` does not list it and the bridge refuses it, saying what to use instead.
- **The recall note.** At the first prompt the console wakes the project's about and the person's (`user:<login name>`), each with the prompt as intent, and appends what it found to the prompt as a `[AXLR · recall]` console note (the transcript shows it as a console row): the `current_state`, `open_loops` and `next_actions` lines, bounded to 2 KiB (1 KiB under the compact profile) and shared by the abouts in order. Nothing recorded under an about is one line (`project:X: new project, nothing recorded yet.`), and when no about holds anything there is no note at all (until 14 October 2026 every prompt of a new project carried those empty lines); a wake that fails is disclosed in the note. A resumed session (`--session`, `--continue`, the picker) gets its own `ws:<session id>` woken too. A later prompt that changes topic (fewer than a fifth of its words of four letters or more shared with the last woken prompt; a prompt under four such words continues the topic) gets a fresh note; the app log records each recall with its abouts and bytes.
- **`axlr_recall`.** Offered whenever KMP is connected, in every mode, read-only and approved without a card: `{question?, about?, since?, mode?}`. `ask` (the default with a question) runs `kmp_ask` with a generous budget and answers with the evidence KMP retrieved (ref, text, time), its status and confidence, since without Jev the answer is a pointer at the evidence; `UNKNOWN` passes as it is. `wake` (the default without a question) answers with the about's state lines and refs. `history` runs `kmp_time`: forward from `since` (an RFC 3339 instant or a date), else the newest ten entries rewound from now. The about defaults to the session's; the context and the budget are the console's.
- **`axlr_remember`.** One call records one memory (see [memory writes](../console.md#memory-writes)); `summary_en` carries the English summary of a text written in another language, which a strict store requires, and a refusal for its absence names the field. For other KMP verbs, discover the exact live schemas through `axlr_tools`, then invoke them through `axlr_call_tool`.

The system prompt says this in one paragraph ("Memory (KMP): the console recalled the workspace's memory in the `[AXLR · recall]` note of your first prompt and records every turn itself…"), identical across sessions so the prompt cache keeps it, with a second sentence asking the model to record what the work settles where it records memory itself.

With `kmp_write_memory` connected, the console also checks that the model did: when a request changed a file or ran a command (a `local_exec` or a forged tool whose result says it ran; a denied, cancelled or refused one does not count, and reads, searches, listings, session bookkeeping and KMP reads never do), ran no ceremony and attempted no `kmp_write_memory` (one the person denied counts), the console adds one visible `[AXLR · memory]` message asking the model to record the result or say in one sentence why there is nothing durable. It does this once per request and never where the mode refuses the model's writes (`/incident`, `/repair`, `/improve`, `/plan`, task sessions) or where the mode starts a ceremony (`/debug`, `/delivery`) or while a ceremony records its own outcome. Each write still follows `kmp`'s approval policy: with `manual`, every record waits on a card; set the policy deliberately in `/mcp`, or always-allow `kmp_write_memory` from its card, when the cards would keep writes from landing.

Until an about is selected for a session, the console gives the model the workspace's project about: `project:` and the workspace root's base name with its case, with runs of spaces as `-` (`/home/me/AXLR` gives `project:AXLR`, what KMP's other hosts write for that checkout and the scope self-repair uses for its repository). Session metadata and `axlr_session` show it, the latter with `about_is_default`, and `axlr_remember` writes there, labelled with the session id and the workspace. Every session in the same checkout therefore finds what the previous one recorded; a clone under another directory name gets its own about. When the checkout's committed bundle `.kmp/memory.jsonl` names a project about (one, or among several the one equal to the directory's name ignoring case), the console selects it for every new session of that workspace at its start, as the person would with `axlr_session`, so `about_is_default` is false and the bundle's spelling wins over the directory's. Until 13 October 2026 the name was lower-cased, which split this repository's memory between `project:AXLR` and `project:axlr`; memories written under the lower-case about stay there. On 10 October 2026 claude-haiku-5.5 never selected an about, so session A recorded under `ws:<session A>` and session B in the same workspace answered "nothing recorded". Memories written before this change remain under their `ws:<session-id>` about. A selected about (`axlr_session` with `about`, or the person) always wins.

Every turn with a result also leaves one `turn` entry under `ws:<session-id>`, written by the console with the MADE step as evidence (see [the session ceremony](../ceremonies.md#the-session-ceremony)); the about of a session's own history is `ws:` and its id, with the workspace, the project about, the mode and the model as labels. The debug/delivery driver uses that saved about, falling back to `ws:<session-id>` rather than the project about (a plan's tasks and syncs read the planner session's about for that plan's notes alone), and attempts a terminal observation with ceremony, step and workspace labels and a stable idempotency key. It retains reference-bearing bounded wake context and exposes pending continuation, but does not automatically exhaust every wake page or record all decisions. Wake errors are disclosed; failed or review-pending outcome writes are reported without undoing MADE progress. Inspect and finish relevant memory work through explicit project-memory calls.

## Compare abouts

`kmp_relate` reads comparisons and proposals between abouts; it does not create relations, and the console no longer asks for one at every start (the retired `axlr-session` skill did). Run one on demand, through `axlr_call_tool`, when the task or retrieved evidence identifies other relevant scopes: use the session's exact about and `dimensions.scope="abouts"` with a nonempty `dimensions.abouts` list that includes it; when none are known, `dimensions.scope="all_abouts"` discovers candidates in the selected store, at a real cost, so use it once, keep the budget bounded, finish the relevant pages and disclose partial results. An about that does not exist yet has nothing to compare: record the first source-backed durable fact there first, never a dummy fact. User scope restrictions take precedence.

Inspect both endpoints of a relevant proposal before relying on it. Shared labels, similar summaries and chronological order locate candidates; they do not establish identity or causality. No proposals is a valid outcome; never manufacture a link to make a comparison look successful. To declare a supported cross-about identity, use `kmp_write_memory`'s `relations` form for stored endpoints, not a replacement `memories` record: the source must belong to the declared about, the target may cross abouts only for `same_event_as` or `same_entity_as`, with the exact returned proposal in `read_context` as the live schema requires, canonical endpoint refs and proposal tokens copied exactly, your own `why` and concrete `evidence` for that identity, and one idempotency key for the logical write; review the returned neighbourhood before resuming its unchanged continuation, and `accepted=true` is the persistence receipt. Other dependencies or similarities remain findings with source refs; do not label a dependency `same_entity_as`. Use `kmp_trace` when a conclusion depends on a stored connection.

## Disconnect KMP from AXLR

1. Finish or interrupt active turns and stop the AXLR console. Note the current store path from `kmp-mcp info` with the registration’s exact environment; disconnecting must not delete it.
2. Back up AXLR's private MCP configuration (`$XDG_CONFIG_HOME/axlr/mcp.json`, or `$HOME/.config/axlr/mcp.json`). Remove only the `plugins` array entry whose manifest is your KMP manifest. Preserve the other registrations, version, file ownership and mode `0600`. Also remove any one-launch `--plugin` flag or duplicate package registration that points to KMP.
3. Restart AXLR and confirm `/mcp` no longer lists `kmp`. The built-in `/plugin` catalogue row can remain: it does not represent a live server. Existing AXLR session transcripts remain, but new KMP memory calls are unavailable.
4. If you also intend to remove the engine from the machine, follow the upstream KMP uninstall procedure for **the installation method you used** after checking other hosts. Preserve or export the memory store separately. Removing the AXLR registration alone leaves the binary and all memory intact.

## Recovery

If `/mcp` reports KMP unavailable, compare the manifest's absolute command, executable permission, selected store and `kmp-mcp doctor` output. An `UNKNOWN` answer from `kmp_ask` is a valid evidence result; it does not mean the server failed. If a store is locked or fails a format check, keep its bytes, stop other writers and follow KMP's recovery guide rather than selecting an empty replacement.

For engine updates and a restore rehearsal, follow [recovery and maintenance](recovery.md). Disconnecting or updating AXLR does not authorize an upstream store migration.
