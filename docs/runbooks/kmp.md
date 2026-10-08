# Runbook: connect or remove KMP memory

KMP is AXLR's governed, durable memory layer. AXLR executes the agent turn and calls KMP over MCP for evidence-backed recall and writes. The built-in KMP row in `/plugin` is a product catalogue entry, not proof that an engine is installed or connected. `/mcp` shows the active connection.

This runbook uses a directly managed local `kmp-mcp` stdio process. See the [KMP embedded guide](https://github.com/underpass-ai/kmp/blob/main/docs/embedded/README.md) for engine releases, store selection, doctor output and upstream lifecycle details. Use one KMP registration per AXLR host.

## Preconditions and exit criteria

Use an engine supported by your platform and store format; the service distribution currently locks KMP 0.24.0 in [engines.lock.json](../../distribution/engines.lock.json). Record the binary version, absolute store path and owning account before connecting. A package version, a running process and the selected store are separate checks.

Success means AXLR discovers the intended `kmp` registration, can retrieve its guide and can recover a known memory from the intended store. A new project may legitimately have no memories. A successful connection alone does not prove that you selected the right data.

## Install and connect

1. Install a release-matched `kmp-mcp` binary using the upstream KMP instructions. If no published asset exists for your platform, KMP documents `cargo install kmp-mcp --locked`. Record its **absolute** executable path and check `kmp-mcp --version` and `kmp-mcp doctor`.
2. Choose the store deliberately. `KMP_MCP_DATA_DIR` is the most explicit selection; use an absolute directory containing the intended memory. Check it with `KMP_MCP_DATA_DIR=/absolute/path/to/memory kmp-mcp info` before any write. KMP also supports a saved selection and automatic project/user defaults; see its guide for precedence. Do not create a new empty store accidentally when you meant to resume an existing one.
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

If `kmp_guide` reports missing guide nodes, stop the AXLR connection and any writer for that embedded store, then synchronize the assets from the matching installed KMP plugin:

```bash
KMP_MCP_BACKEND=embedded KMP_MCP_DATA_DIR=/absolute/path/to/memory \
  /absolute/path/to/kmp-mcp guide sync --plugin-root /absolute/path/to/matching-kmp-plugin
```

This writes guide data to the selected store. Restart AXLR with the same selection and retry `kmp_guide` and the known-memory read. Do not repair a missing guide by silently switching to another store. Upstream tool feedback is authoritative for a different engine version or backend.

## Use memory in a task

1. Discover the exact live schemas through `axlr_tools`, then invoke them through `axlr_call_tool`.
2. Start or reuse KMP's guide identity and context. Wake the task's known stable project scope before re-deriving context; keep returned identities and finish relevant pages.
3. Ask targeted questions, inspect cited evidence and distinguish an evidence-backed answer from `UNKNOWN`. A missing project scope in a confirmed store can be a first-use condition; a protocol or store error is not `UNKNOWN`.
4. Record decisions, constraints and outcomes with their evidence, rather than copying the conversation. Supply one stable idempotency key per logical write and review any `needs_review` continuation before resuming it. Link decisions with justified relations when the evidence supports them.
5. Verify the write receipt and retrieve the recorded result. Retain the project scope for the next session.

With `kmp_write_memory` connected, the console tells the model in normal, review, writer and research sessions to record what a task settles before its final answer, and checks that it did: when a request changed a file or made five or more tool calls, ran no ceremony and attempted no `kmp_write_memory` (one the person denied counts), the console adds one visible `[AXLR · memory]` message asking the model to record the result or say in one sentence why there is nothing durable. It does this once per request and never where the mode refuses the model's writes (`/incident`, `/repair`, `/improve`, `/plan`, task sessions) or while a ceremony records its own outcome. Each write still follows `kmp`'s approval policy: with `manual`, every record waits on a card; set the policy deliberately in `/mcp`, or always-allow `kmp_write_memory` from its card, when the cards would keep writes from landing.

The console's `axlr:axlr-session` skill recovers the exact project scope, stores it with `axlr_session`, defines a missing title after the second user prompt and requests one relevant inter-about comparison. Titles and abouts remain separate; a comparison proposal is not a declared relation. See the [agent workflow](agent-workflow.md#start-the-task).

The debug/delivery driver uses that saved about, falling back to `ws:<session-id>`, and attempts a terminal observation with ceremony, step and workspace labels and a stable idempotency key. It retains reference-bearing bounded wake context and exposes pending continuation, but does not automatically exhaust every wake page or record all decisions. Wake errors are disclosed; failed or review-pending outcome writes are reported without undoing MADE progress. Inspect and finish relevant memory work through explicit project-memory calls.

## Disconnect KMP from AXLR

1. Finish or interrupt active turns and stop the AXLR console. Note the current store path from `kmp-mcp info` with the registration’s exact environment; disconnecting must not delete it.
2. Back up AXLR's private MCP configuration (`$XDG_CONFIG_HOME/axlr/mcp.json`, or `$HOME/.config/axlr/mcp.json`). Remove only the `plugins` array entry whose manifest is your KMP manifest. Preserve the other registrations, version, file ownership and mode `0600`. Also remove any one-launch `--plugin` flag or duplicate package registration that points to KMP.
3. Restart AXLR and confirm `/mcp` no longer lists `kmp`. The built-in `/plugin` catalogue row can remain: it does not represent a live server. Existing AXLR session transcripts remain, but new KMP memory calls are unavailable.
4. If you also intend to remove the engine from the machine, follow the upstream KMP uninstall procedure for **the installation method you used** after checking other hosts. Preserve or export the memory store separately. Removing the AXLR registration alone leaves the binary and all memory intact.

## Recovery

If `/mcp` reports KMP unavailable, compare the manifest's absolute command, executable permission, selected store and `kmp-mcp doctor` output. An `UNKNOWN` answer from `kmp_ask` is a valid evidence result; it does not mean the server failed. If a store is locked or fails a format check, keep its bytes, stop other writers and follow KMP's recovery guide rather than selecting an empty replacement.

For engine updates and a restore rehearsal, follow [recovery and maintenance](recovery.md). Disconnecting or updating AXLR does not authorize an upstream store migration.
