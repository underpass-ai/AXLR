# Packages and MCP connections

AXLR shows two related inventories:

| In the console | Owns | Changes when you install |
|:--|:--|:--|
| `/plugin` | Packages copied into AXLR's data directory and their indexed skills | `$XDG_DATA_HOME/axlr/plugins` or `$HOME/.local/share/axlr/plugins` |
| `/mcp` | MCP servers AXLR can discover and call | AXLR's `mcp.json` and server approval policy |

AXLR adopts the Codex plugin package format and standard MCP transports. A plugin made for Codex will usually work in AXLR when its useful components are skills and MCP servers. AXLR owns its copy of the package and its MCP approval policy; installation in one host does not install it in the other.

KMP and MADE appear as built-in catalogue entries. Those entries identify AXLR's intended memory and orchestration engines; they do not install an engine binary or prove that its MCP server is connected. Check `/mcp` for the live state. Use the [KMP](runbooks/kmp.md) and [MADE](runbooks/made.md) runbooks to connect, verify or remove them.

The MADE entry includes the embedded `axlr-ceremonies` skill and seven [default working procedures](ceremonies.md). The console agent reads them through `axlr_skill` without copying a package. They guide workflow selection by default; engine connection, definition publication and real execution remain separate operations.

## Codex plugin compatibility

| Codex package component | AXLR behavior |
|:--|:--|
| `.codex-plugin/plugin.json` | Reads package name, version, skill path and MCP server declarations |
| `.agents/plugins/marketplace.json` | Reads a marketplace source and stages its packages |
| Skills in the declared directory | Indexes `SKILL.md` files for the model to read on demand |
| `mcpServers` inline or in a referenced JSON file | Converts stdio or URL servers to AXLR MCP registrations with manual approval |
| Other Codex plugin components | Retains their files in the package but does not activate them |

The MCP server must still have its executable, credentials and configuration available to AXLR. The supported package shape is the compatibility promise; arbitrary Codex host behavior and future manifest fields are not automatically implemented. Test a candidate by adding its source in `/plugin`, installing it, then checking skills and live servers in `/mcp`.

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

At a tool approval dialog, `L` saves an always-allow choice for the exact local operation or registered plugin tool and approves the current call. `/approvals` shows those choices. `/autonomy on` enables automatic approval for all known tools; `/autonomy off` returns to saved per-tool and per-server policies. These settings are stored in `approvals.json` beside `mcp.json` (or beside the file supplied with `--mcp-config`).

`env_from` copies selected host variables by name. `env` can supply literal values, but secrets are better kept in the host environment. For a stdio server, these maps form the **complete** child environment; AXLR does not inherit everything by default. `OPENROUTER_API_KEY` cannot be forwarded to plugins through these fields. HTTP registrations do not accept child environment entries. The console also accepts `--mcp-config /absolute/path/config.json` to choose another file.

For one launch, use repeatable flags:

```bash
/tmp/axlr-tui --root "$PWD" --plugin /absolute/path/to/notes.json \
  --plugin-env-from 'notes:TOKEN=NOTES_API_TOKEN'
```

The JSON worker accepts repeatable `--plugin` manifests and `--plugin-env ID:KEY=VALUE` flags. The worker has no persistent `mcp.json` and closes plugin sessions after its one response.

## How discovery and errors work

No directory is scanned automatically. Servers connect when AXLR discovers their tools or calls one. The TUI surfaces an unavailable server in `/mcp`; a failed discovery before a model request is reported as an error. The JSON worker's `plugins.list` fails if any configured server is unavailable. A call is limited to a configured ID, a manifest-allowed tool and a currently advertised tool.

MCP descriptions and schemas are untrusted data, not grants of authority. A server's `is_error: true` is a completed tool result, while launch or protocol failures are AXLR failures. A connection loss cannot prove whether a remote side effect occurred; AXLR does not retry effectful calls automatically. See [worker statuses](worker.md#responses-and-exit-codes) and [troubleshooting](troubleshooting.md).
