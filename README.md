# AXLR

**Agent eXecution Local Runtime** is Underpass's own local execution runtime. It is a focused rewrite of the execution layer in our [underpass-runtime](https://github.com/underpass-ai/underpass-runtime), delivered as a small Go library and a one-request JSON worker for `read`, `write`, `edit`, and `exec`. Pi informed the choice of a compact tool surface; AXLR does not use Pi code or runtime. The first profile is `trusted-local` on Linux. It runs with the OS access of its host account; it is **not a sandbox**.

## Build and run

Go 1.26 is required. The root module has no external dependencies.

```bash
go test ./...
CGO_ENABLED=0 go build -trimpath -o bin/axlr ./cmd/axlr
printf '%s\n' '{"protocol_version":1,"request_id":"demo-1","tool":"read","arguments":{"path":"README.md"}}' |
  bin/axlr --root "$PWD" --profile trusted-local
```

The worker consumes one JSON document from stdin and emits one JSON response on stdout. It accepts `--root` for an existing workspace and repeatable `--env KEY=VALUE` flags for the child process environment. The child does not inherit the worker environment by default. Avoid passing secrets through command-line flags; they may be visible to other processes on the host. A host that needs secrets should use an appropriate launcher and account boundary.

The library entrypoint is `New(Config)` followed by `Executor.Execute(ctx, Request)` and `Close()`. An executor serializes requests. A host can set lower maximums in `Config`; requests cannot raise them.

## Request contract

All requests contain `protocol_version: 1`, a nonempty `request_id`, one of the four tool names, and a typed `arguments` object. Unknown JSON fields, extra documents and bodies above 4 MiB are rejected.

| Tool | Arguments | Result |
| --- | --- | --- |
| `read` | `path`, optional `offset_bytes`, `max_bytes` | UTF-8 `content`, byte offsets, `truncated`, full-file `content_sha256` when complete |
| `write` | `path`, `content`, `mode: create\|replace`, `expected_sha256` for replace | `written_bytes`, `content_sha256` |
| `edit` | `path`, `old_text`, `new_text`, optional `expected_sha256` | `written_bytes`, `content_sha256` |
| `exec` | `program`, optional `args`, `cwd`, `stdin`, `timeout_ms`, `max_output_bytes` | `exit_code`, `stdout`, `stderr`, capture and discarded byte counts |

Paths for file tools are relative to the host workspace and anchored using `os.Root`; they cannot traverse outside through `..` or a symlink. `cwd` is also checked against the workspace, but `exec` itself has the host account's access. `program` and `args` are passed as argv without an implicit shell. To run shell syntax, explicitly choose `/bin/sh` as the program.

`write` create never overwrites an existing destination. Replace requires the SHA-256 of the previously observed full file. `edit` replaces exactly one literal occurrence; zero or multiple matches are conflicts. These checks do not promise a transaction against concurrent external writers. The host should serialize its own mutations.

Default read size is 64 KiB; the maximum is 1 MiB. Editable files are capped at 1 MiB. Default combined process output capture is 256 KiB; the maximum is 1 MiB. Default process timeout is 30 seconds; the maximum is five minutes. Output is drained and discarded bytes counted after the capture budget is exhausted. A serialized response larger than 4 MiB becomes a `response_too_large` failure because JSON escapes can expand text.

## Results and worker exit codes

Each response contains `status`, UTC timestamps, monotonic `duration_ms`, and either `output` or a stable error `code` and English `message`. `completed` for `exec` means the process was observed to exit; `exit_code` may be nonzero. `rejected`, `failed`, `cancelled`, and `timed_out` are distinct. Cancellation and timeout do not undo effects already produced.

The worker exits `0` after a valid protocol request even when the tool failed, `2` for malformed protocol input with a JSON rejection when possible, and `1` for a fatal worker or response write failure. `request_id` is for correlation, not deduplication. If the worker vanishes without a complete response, the host must treat the effect as unknown and reconcile before retrying.

## Architecture

`domain/` holds value objects and operation commands/results. `application/` holds ports and use cases. `adapters/local/` implements filesystem and process ports. Root-level DTOs, mappers, codec and executor compose the public library. `cmd/axlr/` is the protocol adapter. Each Go file has one primary type where a type is needed.

The separate [MCP client module](mcpclient/README.md) connects to external MCP servers over stdio or Streamable HTTP. It discovers and calls their tools by `(server, tool)` identity. Hosts can compose it with AXLR's local executor. AXLR does not expose its four local tools as an MCP server.

The [design](docs/plans/2026-09-29-hexagonal-design.md), [implementation plan](docs/plans/2026-09-29-minimal-runtime.md), and [provenance note](docs/provenance.md) record the boundaries and lineage. Sandboxing and comparative performance measurements are outside this delivery.
