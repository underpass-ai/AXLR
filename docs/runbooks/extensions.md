# Runbook: qualify and connect an extension

Use this procedure to add a capability beyond KMP and MADE. AXLR supports MCP tool servers and the skills/MCP portion of compatible OpenAI/Codex plugin packages. Check the [compatibility matrix](../plugins.md#codex-plugin-compatibility) against the candidate before installation.

## Qualify the candidate

Record the source and revision/version, required executable or endpoint, intended tools, credentials, data destination and a read-only smoke operation. Confirm:

- A direct server uses stdio or Streamable HTTP. The console has no remote OAuth or custom-header configuration; a library host can supply an authenticated HTTP client.
- A package contains `.codex-plugin/plugin.json`, a name and version, and supported `skills` / `mcpServers` declarations. A root `plugin.json` alone is not imported.
- Referenced binaries, shell utilities and credentials are available in AXLR's host. An account connected in OpenAI's host is not automatically connected in AXLR.
- The useful capability does not depend on unimplemented hooks, app rendering or host-specific components. Installed skill scripts are not automatically executed.

Success is a discoverable exact tool with a valid schema and a verified result, or a readable skill whose required tools are available. A successful package copy is only the installation step.

## Install a package

1. Open `/plugin`, press `M`, and enter an absolute local source path or HTTPS Git URL, optionally with `#subdir`.
2. Refresh, select the staged package and press Enter. AXLR copies it into its own data directory; it does not install into Codex or alter Codex's configuration.
3. Inspect `/mcp`. Converted servers receive IDs `<package>-<server>`, manual approval and generated allowlists of `["*"]`. Narrow the private manifests if the full advertised surface is unnecessary, then restart to reload them.
4. Inspect generated `env` / `env_from`: stdio packages select `HOME` and `PATH`, forward declared `env_vars`, and expand supported plugin-root references. Keep credentials in named host variables. Restart after changing startup configuration.
5. Ask AXLR to read the relevant skill and exact tool schema, then run the agreed read-only smoke operation. Check the result and owning server before enabling any persistent automatic policy.

Use direct IDs `kmp` and `made` for their built-in driver/setup/update integration; package-generated IDs are ordinary MCP registrations. If installation fails partway through, inspect both package storage and `mcp.json` before retrying: some servers may already be registered.

## Connect a server directly

For stdio, create a private absolute-path manifest:

```json
{
  "manifest_version": 1,
  "id": "notes",
  "command": "/absolute/path/to/notes-mcp",
  "args": [],
  "allow_tools": ["search"]
}
```

Open `/mcp`, press `I` and enter the manifest's absolute path. Add required `env` / `env_from` entries to its private persistent configuration, then restart before calling it. Direct stdio registrations use only the configured child environment.

For Streamable HTTP, enter `notes https://host/mcp` instead. This creates a private URL manifest with all advertised tools allowed and per-call manual approval. HTTP registrations do not accept child environment values. Confirm the endpoint's authentication fits this route; do not put credentials in its URL.

Refresh `/mcp`, inspect the exact tools, then ask for the read-only smoke operation. Retain the current manual policy unless the intended unattended use warrants an explicit exact-tool or per-server choice. Restricted console modes do not classify MCP side effects.

## Disconnect or roll back

1. Stop new work, settle any uncertain calls and close AXLR.
2. Back up `mcp.json` and the exact manifests. Remove only the affected registrations, preserving other entries and mode `0600`.
3. For an installed package, move its exact directory out of `$XDG_DATA_HOME/axlr/plugins/installed` (or `$HOME/.local/share/axlr/plugins/installed`) into a private backup location. There is no package-uninstall command in the current UI. Do not leave registrations pointing at moved manifests.
4. Review saved exact-tool approvals in `settings.json` before reusing the same IDs. Restart and confirm that the servers and skill index no longer expose the removed capability.

Keep remote resources, credentials and engine data under their own lifecycle. Restoring a package requires restoring matching manifest paths and configuration, then repeating discovery and the smoke operation.
