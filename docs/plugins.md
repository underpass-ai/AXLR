# Plugins and MCP connections

AXLR shows two inventories. They serve different hosts and persist in different places:

| In the console | Owns | Changes when you install |
|:--|:--|:--|
| `/plugin` | Codex plugin packages in the local Codex installation | Codex's package catalogue and installed package set |
| `/mcp` | MCP servers AXLR can discover and call | AXLR's `mcp.json` and server approval policy |

Installing a Codex package does not connect its MCP server to AXLR. Connecting an MCP server to AXLR does not install a Codex package. KMP and MADE can appear in both lists when both registrations exist.

## Browse Codex packages

Open `/plugin`. Tab switches installed and available packages; `/` searches; Enter reviews installation; `M` adds a marketplace from a Git URL, `owner/repo` or local path; `R` refreshes. The `codex` CLI must be on `PATH`. AXLR invokes Codex's plugin commands; it does not implement an independent marketplace or change plugin approval settings.

## Connect an MCP server in the console

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
