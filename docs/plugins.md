# Packages and MCP connections

AXLR shows two related inventories:

| In the console | Owns | Changes when you install |
|:--|:--|:--|
| `/plugin` | Packages copied into AXLR's data directory and their indexed skills | `$XDG_DATA_HOME/axlr/plugins` or `$HOME/.local/share/axlr/plugins` |
| `/mcp` | MCP servers AXLR can discover and call | AXLR's `mcp.json` and server approval policy |

AXLR extends its execution engine through MCP tools and the supported components of OpenAI/Codex plugin packages. Compatibility depends on the manifest, transport, authentication and host capabilities listed below. AXLR owns its copy of the package and its MCP approval policy; installation in one host does not install it in the other.

AXLR, KMP and MADE appear as built-in catalogue entries. AXLR supplies the embedded `axlr-session` skill for session titles and project continuity. KMP and MADE identify the intended memory and orchestration engines; their rows do not install an engine binary or prove that its MCP server is connected. Check `/mcp` for the live state. Use the [KMP](runbooks/kmp.md) and [MADE](runbooks/made.md) runbooks to connect, verify or remove them.

The MADE entry includes the embedded `axlr-ceremonies` skill and seven 1.0 procedures for explicit requests. The console reads them through `axlr_skill` without copying a package. Separately, `/debug` and `/delivery` use pinned 2.0 definitions and `/incident` uses `axlr_incident` 1.0, prepared through `/mcp` → `P`. [Ceremonies](ceremonies.md) explains the distinction; engine connection, definition publication and real execution are separate operations.

## Update MADE and KMP

Run `/update`, or choose **Update engines** in the action palette. AXLR checks
the latest stable releases from the official `underpass-ai/made` and
`underpass-ai/kmp` repositories. It verifies each package's SHA-256, plugin
version and engine version before changing that engine's persistent connection.
The panel reports each result separately; one engine's failure does not prevent
the other from updating.

Only explicitly configured local servers named `made` and `kmp` with no custom
arguments are updated. Remote, command-line-only and unconfigured connections
are skipped. Supported release packages are Linux x86_64/arm64 and macOS arm64.
Finish or cancel any active agent task before updating.

Verified packages live under `$XDG_DATA_HOME/axlr/engines` or
`$HOME/.local/share/axlr/engines`. AXLR saves a new private manifest reference
under its MCP configuration directory and keeps the previous package and
manifest. Explicit environments, selected stores, cursor keys, approval modes,
tool allowlists and other servers are preserved. The command installs no
ceremony definitions and performs no store migration or authorization bootstrap.

Restart AXLR after an update: existing MCP registrations keep their current
executables for the running session. Packages installed in Codex, Claude or
Hermes have their own update lifecycle; `/update` manages AXLR's copies.

## Codex plugin compatibility

| Codex package component | AXLR behavior |
|:--|:--|
| `.codex-plugin/plugin.json` | Reads package name, version, skill path and MCP server declarations |
| `.agents/plugins/marketplace.json` | Reads a marketplace source and stages its packages |
| Skills in the declared directory | Indexes `SKILL.md` files for the model to read on demand |
| `mcpServers` inline or in a referenced JSON file | Converts stdio or URL servers to AXLR MCP registrations with manual approval |
| `env_vars` and `env` for stdio servers | Forwards named host variables or explicit values; generated packages also select `HOME` and `PATH` |
| `${CODEX_PLUGIN_ROOT}` / `${CLAUDE_PLUGIN_ROOT}` | Expands package-root references in command, args and environment values |
| Root `plugin.json` without `.codex-plugin/plugin.json` | Not imported by this implementation; provide the compatibility manifest |
| Hooks, apps/connectors, UI and other host components | Retains files but does not activate host-specific behavior or inherit account connections |

OpenAI's [packaging guide](https://developers.openai.com/plugins/build/plugins) describes both portable root manifests and the `.codex-plugin/plugin.json` compatibility format. AXLR's importer currently implements the latter. The precise supported fields are in [manifest.go](../tui/adapters/axlrplugin/manifest.go); an OpenAI-compatible label alone is not proof that all package features run here.

The MCP server must still have its executable, credentials and configuration available to AXLR. The supported package shape is the compatibility promise; arbitrary Codex host behavior and future manifest fields are not automatically implemented. Follow the [extension qualification runbook](runbooks/extensions.md) before relying on a candidate. A package import generates server IDs as `<package>-<server>`; built-in driver/update/setup integration expects exact IDs `kmp` and `made`. Direct registrations are the documented route for those engines.

## Install a package into AXLR

Open `/plugin`. Tab switches installed and available packages, `/` searches and `R` refreshes. Press `M` to add an **absolute local path** or **HTTPS Git URL** (optionally with `#subdir`) containing `.codex-plugin/plugin.json` or `.agents/plugins/marketplace.json`. AXLR reads the Codex package format but owns this installation; it does not invoke the Codex CLI or change the Codex app's packages. Select a staged package and press Enter to install it.

AXLR indexes installed skills for the model to read on demand. If a package declares MCP servers, AXLR registers them in its MCP configuration with **manual** approval. Inspect the exact connection in `/mcp`. Other package components are retained but not activated. Package storage and MCP registration are different records; removing one does not mean its engine's persistent data was deleted.

## Connect an MCP server directly

Open `/mcp` and press `I`. Enter one of:

- An **absolute path** to an AXLR version 1 manifest for a local stdio server.
- `ID https://host/mcp` for a Streamable HTTP endpoint.

The new server receives `manual` approval and is saved in AXLR's MCP configuration. An HTTP URL installation creates a private manifest under the configuration directory with `allow_tools: ["*"]`. It allows every tool the server advertises, subject to AXLR's per-call manual approval until you change the policy. Use a trusted endpoint. The TUI URL route does not currently supply custom HTTP authentication; a Go host can provide an HTTP client when using `mcpclient` directly.

A stdio manifest looks like this:

```json
{
  "manifest_version": 1,
  "id": "notes",
  "command": "/absolute/path/to/notes-mcp",
  "args": [],
  "allow_tools": ["search"]
}
```

An HTTP manifest uses `"url": "https://host/mcp"` in place of `command` and `args`. Exactly one transport is required. `allow_tools` must be a nonempty list of exact names or the single wildcard `["*"]`. The manifest is limited to 64 KiB and rejects unknown fields, duplicate fields and duplicate allowed tools. A manifest path and a stdio executable must be absolute.

## Persist registrations and approvals

The console reads `$XDG_CONFIG_HOME/axlr/mcp.json`, or `$HOME/.config/axlr/mcp.json`, at startup. The file must be a private regular file (mode `0600`) no larger than 64 KiB. A missing file means no persistent external servers.

```json
{
  "version": 1,
  "plugins": [
    {
      "manifest": "/absolute/path/to/notes.json",
      "name": "Notes",
      "description": "Workspace knowledge",
      "purpose": "memory",
      "approval": "manual",
      "env_from": {"TOKEN": "NOTES_API_TOKEN"}
    }
  ]
}
```

`purpose` is `tools` (default), `memory` or `ceremony`. `approval` is `manual` (default) or `auto`. In `/mcp`, `A` or Enter opens a review of the exact server and policy change; confirmation persists it. Automatic approval applies to the allowed tools of that server, including future advertised tools when the manifest uses `["*"]`. Command-line-only registrations remain manual unless added to the persistent file.

When the dialog offers persistent choices, `L` saves an always-allow choice for the exact local operation or registered plugin tool and approves the current call. `/approvals` shows those choices. `/autonomy on` enables automatic approval for known tools permitted by the active mode; `/autonomy off` returns to saved per-tool and per-server policies. New choices are stored in `settings.json` under AXLR’s default configuration directory. `approvals.json` beside the selected MCP configuration is a legacy fallback until the settings file has an `approvals` section. `--mcp-config` selects connections, not an alternate settings file. Work modes can still require approval or refuse a local operation; see [console policy](console.md#work-modes).

`env_from` copies selected host variables by name. `env` can supply literal values, but secrets are better kept in the host environment. For a stdio server, these maps form the **complete** child environment; AXLR does not inherit everything by default. `OPENROUTER_API_KEY` cannot be forwarded to plugins through these fields. HTTP registrations do not accept child environment entries. The console also accepts `--mcp-config /absolute/path/config.json` to choose another file.

For one launch, use repeatable flags:

```bash
/tmp/axlr-tui --root "$PWD" --plugin /absolute/path/to/notes.json \
  --plugin-env-from 'notes:TOKEN=NOTES_API_TOKEN'
```

The JSON worker accepts repeatable `--plugin` manifests and `--plugin-env ID:KEY=VALUE` flags. The worker has no persistent `mcp.json` and closes plugin sessions after its one response.

## Surface boundaries

The console supports MCP tool discovery and calls over stdio and Streamable HTTP. It does not offer a general MCP resource/prompt browser, remote OAuth sign-in, custom HTTP headers or an MCP app renderer. Authentication that works in another host is not inherited. A library host can supply its own authenticated `http.Client`; a console integration must fit the available transport or use an explicitly configured local adapter.

The JSON worker and Go manager accept explicit MCP registrations but do not install packages or execute skills themselves. The shipped HTTP service connects only its configured KMP and MADE adapters; it does not read the console's `mcp.json`, `settings.json` or package directory. See the [product capability matrix](product.md#entry-points-and-current-capabilities).

## How discovery and errors work

MCP servers require explicit registration; arbitrary directories are not scanned for servers. Servers connect when AXLR discovers their tools or calls one. The TUI surfaces an unavailable server in `/mcp`; a failed discovery before a model request is reported as an error. The JSON worker's `plugins.list` fails if any configured server is unavailable. A call is limited to a configured ID, a manifest-allowed tool and a currently advertised tool.

MCP descriptions and schemas are untrusted data, not grants of authority. A server's `is_error: true` is a completed tool result, while launch or protocol failures are AXLR failures. A connection loss cannot prove whether a remote side effect occurred; AXLR does not retry effectful calls automatically. See [worker statuses](worker.md#responses-and-exit-codes) and [troubleshooting](troubleshooting.md).
