# Runbook: connect or remove KMP memory

KMP is AXLR's governed, durable memory layer. AXLR executes the agent turn and calls KMP over MCP for evidence-backed recall and writes. The built-in KMP row in `/plugin` is a product catalogue entry, not proof that an engine is installed or connected. `/mcp` shows the active connection.

This runbook uses a directly managed local `kmp-mcp` stdio process. See the [KMP embedded guide](https://github.com/underpass-ai/kmp/blob/main/docs/embedded/README.md) for engine releases, store selection, doctor output and upstream lifecycle details. Use one KMP registration per AXLR host.

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
  "env": {"KMP_MCP_DATA_DIR": "/absolute/path/to/memory"}
}
```

Use `{"version":1,"plugins":[...]}` as the top-level shape if this is the first server. The child environment is explicit; set every variable the engine needs there or through `env_from`. Configure the intended store **before first discovery** so AXLR cannot select a different default store by accident.

5. Restart AXLR. Open `/mcp`, refresh and verify that `kmp` lists tools. Ask the agent to read KMP's guide and perform a read-only `kmp_wake` or `kmp_ask`. On a fresh store, synchronize the matching KMP guide assets using KMP's documented guide workflow before relying on guided recall. Approval remains manual until you deliberately change `kmp`'s policy in `/mcp`.

For a one-launch check without changing persistent configuration, use `axlr-tui --root /absolute/workspace --plugin /absolute/path/to/kmp.json`. The worker also accepts `--plugin` for a one-request MCP call. Its process lifetime ends after that response.

## Disconnect KMP from AXLR

1. Finish or interrupt active turns and stop the AXLR console. Note the current store path from `kmp-mcp info`; disconnecting must not delete it.
2. Back up AXLR's private MCP configuration (`$XDG_CONFIG_HOME/axlr/mcp.json`, or `$HOME/.config/axlr/mcp.json`). Remove only the `plugins` array entry whose manifest is your KMP manifest. Preserve the other registrations, version, file ownership and mode `0600`. Also remove any one-launch `--plugin` flag or duplicate package registration that points to KMP.
3. Restart AXLR and confirm `/mcp` no longer lists `kmp`. The built-in `/plugin` catalogue row can remain: it does not represent a live server. Existing AXLR session transcripts remain, but new KMP memory calls are unavailable.
4. If you also intend to remove the engine from the machine, follow the upstream KMP uninstall procedure for **the installation method you used** after checking other hosts. Preserve or export the memory store separately. Removing the AXLR registration alone leaves the binary and all memory intact.

## Recovery

If `/mcp` reports KMP unavailable, compare the manifest's absolute command, executable permission, selected store and `kmp-mcp doctor` output. An `UNKNOWN` answer from `kmp_ask` is a valid evidence result; it does not mean the server failed. If a store is locked or fails a format check, keep its bytes, stop other writers and follow KMP's recovery guide rather than selecting an empty replacement.
