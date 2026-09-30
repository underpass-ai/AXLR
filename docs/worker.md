# One-request JSON worker

`cmd/axlr` is a small process boundary for a host that wants one local operation at a time. It consumes one JSON document on stdin and returns one JSON document on stdout. It does not manage a conversation, select a model or run an approval UI. The host owns those decisions.

## Run it

```bash
GOWORK=off CGO_ENABLED=0 go build -trimpath -o /tmp/axlr ./cmd/axlr
printf '%s\n' '{"protocol_version":1,"request_id":"read-1","tool":"read","arguments":{"path":"README.md"}}' |
  /tmp/axlr --root "$PWD" --profile trusted-local
```

`--root` must name an existing workspace and `--profile` must be `trusted-local`. `--env KEY=VALUE` sets the complete environment for local `exec` children; it is repeatable. A plugin can be registered with repeatable `--plugin /absolute/path/manifest.json` and `--plugin-env ID:KEY=VALUE`. Avoid putting secrets directly in command-line values, which other host processes may see.

## Request shape

```json
{
  "protocol_version": 1,
  "request_id": "read-1",
  "tool": "read",
  "arguments": {"path": "README.md"}
}
```

`request_id` is a nonempty correlation ID, not a deduplication key. The worker rejects unknown fields, extra JSON documents and request bodies over 4 MiB.

| Tool | Arguments | Main output |
|:--|:--|:--|
| `read` | `path`, optional `offset_bytes` and `max_bytes` | UTF-8 `content`, byte offsets, `truncated` and a full-file SHA-256 when complete |
| `write` | `path`, `content`, `mode` (`create` or `replace`); `expected_sha256` for replacement | `written_bytes` and `content_sha256` |
| `edit` | `path`, `old_text`, `new_text`, optional `expected_sha256` | `written_bytes` and `content_sha256` |
| `exec` | `program`, optional `args`, `cwd`, `stdin`, `timeout_ms` and `max_output_bytes` | `exit_code`, `stdout`, `stderr` and capture counts |
| `plugins.list` | Empty object | Allowed tools with plugin ID, name, description and schemas |
| `plugins.call` | `plugin_id`, `tool_name` and an `arguments` object | MCP `content`, optional `structured_content` and `is_error` |

File paths are relative to the workspace. AXLR anchors file operations through `os.Root` and refuses escapes through `..` or symlinks. Reading a symlink whose target remains inside the root is permitted; editing requires a regular file at the requested path. `cwd` is checked against the workspace, but the executed program has the host account's OS access. `program` and `args` are argv, without an implicit shell; choose `/bin/sh` explicitly if shell syntax is required.

`write` in `create` mode never overwrites a destination. `replace` needs the SHA-256 of a previously observed full file. `edit` replaces exactly one literal occurrence; zero or multiple matches are conflicts. These checks do not create a transaction against concurrent external writers. A host should serialize its mutations.

Default `read` size is 64 KiB and its maximum is 1 MiB. Editable files are capped at 1 MiB. Default combined process-output capture is 256 KiB, maximum 1 MiB; excess output is drained and counted. Default process timeout is 30 seconds, maximum five minutes. A serialized response above 4 MiB becomes a `response_too_large` failure because JSON escaping can expand content.

## Responses and exit codes

Every response includes `protocol_version`, `request_id`, `tool`, `status`, UTC start and finish times, and `duration_ms`. It contains either `output` or an `error` with stable `code` and English `message`.

| Status | Meaning |
|:--|:--|
| `completed` | AXLR observed the operation complete; an `exec` exit code may still be nonzero and an MCP result may have `is_error: true` |
| `rejected` | The request or arguments did not pass validation or policy |
| `failed` | The operation failed during execution |
| `cancelled` | Cancellation was observed |
| `timed_out` | The deadline was reached |

The process exits `0` for a valid protocol request even when the operation fails, `2` for malformed protocol input when it can emit a rejection, and `1` for a fatal worker or response-write failure. Cancellation and timeout do not undo effects. If the worker disappears without a complete response, the host must treat the effect as unknown and reconcile before retrying.

## External tools

The worker accepts version 1 MCP manifests for stdio or Streamable HTTP. No directory is scanned. The manifest's `allow_tools` list controls which currently advertised names can be called; `["*"]` explicitly allows every advertised tool. Plugin sessions close after the single response. See [Plugins and MCP](plugins.md) for manifest and environment rules, and [Go library](library.md) for a long-lived manager.
