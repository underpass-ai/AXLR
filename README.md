# AXLR

**Agent eXecution Local Runtime** is Underpass's own local execution runtime. It is a focused rewrite of the execution layer in our [underpass-runtime](https://github.com/underpass-ai/underpass-runtime), delivered as a Go library and a one-request JSON worker for `read`, `write`, `edit`, `exec`, and explicitly registered plugin tools.

## Build and run

Go 1.26 is required. The root module uses the official Go MCP SDK to connect to external tool plugins.

```bash
go test ./...
CGO_ENABLED=0 go build -trimpath -o bin/axlr ./cmd/axlr
printf '%s\n' '{"protocol_version":1,"request_id":"demo-1","tool":"read","arguments":{"path":"README.md"}}' |
  bin/axlr --root "$PWD" --profile trusted-local
```

The worker consumes one JSON document from stdin and emits one JSON response on stdout. It accepts `--root` for an existing workspace and repeatable `--env KEY=VALUE` flags for the child process environment. The child does not inherit the worker environment by default. Avoid passing secrets through command-line flags; they may be visible to other processes on the host. A host that needs secrets should use an appropriate launcher and account boundary.

The library entrypoint is `runtime.New(runtime.Config)` followed by `Executor.Execute(ctx, dto.Request)` and `Close()`. An executor serializes requests. A host can set lower maximums in `Config`; requests cannot raise them.

## Interactive agent console

The separate [`tui/` module](tui/README.md) provides `axlr-tui`: OpenRouter streaming, per-call tool approval, local and MCP tools, saved sessions and transcript search. Build it from this checkout with `go -C tui build -o /tmp/axlr-tui ./cmd/axlr-tui`, then supply `OPENROUTER_API_KEY` in the environment and run `/tmp/axlr-tui --root "$PWD" --model 'your-provider/your-model'`. See the TUI guide for controls, plugin environment selections and recovery.

Root and TUI modules have separate test gates. The root library has no terminal dependencies; the JSON worker above keeps its one-request contract.

## OpenRouter model client

The Go library supports non-streaming completions and SSE streaming through OpenRouter. `application.StreamModelUseCase` and `Client.Stream` emit text deltas and return a validated complete response; they do not execute tools. The host supplies `OPENROUTER_API_KEY` through its own secret mechanism; AXLR does not read the environment or store the key for the host. The model client is separate from the JSON worker.

```go
import (
    "context"
    "fmt"
    "os"

    "github.com/underpass-ai/AXLR/adapters/openrouter"
    "github.com/underpass-ai/AXLR/application"
    "github.com/underpass-ai/AXLR/domain"
)

func example() error {
    client, err := openrouter.New(openrouter.ClientConfig{
        APIKey: os.Getenv("OPENROUTER_API_KEY"),
    })
    if err != nil { return err }

    model, err := domain.NewModelID("openai/gpt-4o")
    if err != nil { return err }
    complete := application.CompleteModelUseCase{Models: client}
    request := domain.CompletionRequest{
        Model: model,
        Messages: []domain.Message{{Role: domain.RoleUser, Content: "Say hello"}},
    }
    result, err := complete.Execute(context.Background(), request)
    if err != nil { return err }
    fmt.Println(result.Message.Content)
    return nil
}
```

The caller chooses a model supported by its OpenRouter account.

To offer tools, set `request.Tools` to typed `domain.ToolDefinition` values with JSON Schema parameters. A completion may contain text and several `result.Message.ToolCalls`. AXLR returns those calls without executing them. The host checks each requested name against its own authorized tool registry, executes any allowed call, appends `result.Message` and then one `domain.Message{Role: domain.RoleTool, ToolCallID: call.ID, Content: output}` per result to `request.Messages`, and calls `complete.Execute` again with the same `request.Tools`. Tool results and the assistant's calls must remain in the conversation history.

## Request contract

All requests contain `protocol_version: 1`, a nonempty `request_id`, a supported tool name, and a typed `arguments` object. Unknown JSON fields, extra documents and bodies above 4 MiB are rejected.

| Tool | Arguments | Result |
| --- | --- | --- |
| `read` | `path`, optional `offset_bytes`, `max_bytes` | UTF-8 `content`, byte offsets, `truncated`, full-file `content_sha256` when complete |
| `write` | `path`, `content`, `mode: create\|replace`, `expected_sha256` for replace | `written_bytes`, `content_sha256` |
| `edit` | `path`, `old_text`, `new_text`, optional `expected_sha256` | `written_bytes`, `content_sha256` |
| `exec` | `program`, optional `args`, `cwd`, `stdin`, `timeout_ms`, `max_output_bytes` | `exit_code`, `stdout`, `stderr`, capture and discarded byte counts |
| `plugins.list` | `{}` | `tools` with plugin ID, tool name, description and input/output schemas |
| `plugins.call` | `plugin_id`, `tool_name`, `arguments` object | MCP `content`, optional `structured_content`, `is_error` |

## External tool plugins

A plugin is an external MCP server over stdio. The host registers it with an explicit JSON manifest (64 KiB maximum):

```json
{"manifest_version":1,"id":"search","command":"/absolute/path/search-server","args":[],"allow_tools":["find"]}
```

Run the existing worker with repeatable `--plugin /absolute/path/manifest.json` flags. Use repeatable `--plugin-env ID:KEY=VALUE` flags for a plugin's complete child environment. A plugin receives no inherited worker environment by default. AXLR rejects unknown manifest fields, duplicate plugin IDs, invalid identities and duplicate allowlist entries before launching any plugin. No directory is scanned automatically; plugins start only on `plugins.list` or a call to that plugin. Local tools do not start plugins.

```bash
printf '%s\n' '{"protocol_version":1,"request_id":"tools-1","tool":"plugins.list","arguments":{}}' |
  bin/axlr --root "$PWD" --profile trusted-local --plugin /absolute/path/search.json
```

The `plugins.Manager` Go API provides `List`, `Call` and `Close`, and can be passed as `runtime.Config.Plugins`. It reuses plugin sessions in a long-lived host. The one-request worker closes its sessions after responding. `plugins.list` fails if any configured plugin is unavailable. `plugins.call` only invokes a tool named in `allow_tools` and currently advertised by its server. MCP descriptions and schemas are data, not grants of authority. Plugins execute with the host account's OS access; configure only trusted executables. AXLR does not expose its local tools as an MCP server.

An MCP `is_error: true` result is a `completed` AXLR response with `is_error` in the output. Launch and protocol errors are `failed`; malformed or unauthorized calls are `rejected`. Cancellation and timeouts retain their own statuses. A lost response does not prove the tool had no effect, so AXLR never retries a plugin call automatically. Worker responses stay within 4 MiB.

Paths for file tools are relative to the host workspace and anchored using `os.Root`; they cannot traverse outside through `..` or a symlink. `cwd` is also checked against the workspace, but `exec` itself has the host account's access. `program` and `args` are passed as argv without an implicit shell. To run shell syntax, explicitly choose `/bin/sh` as the program.

`write` create never overwrites an existing destination. Replace requires the SHA-256 of the previously observed full file. `edit` replaces exactly one literal occurrence; zero or multiple matches are conflicts. These checks do not promise a transaction against concurrent external writers. The host should serialize its own mutations.

Default read size is 64 KiB; the maximum is 1 MiB. Editable files are capped at 1 MiB. Default combined process output capture is 256 KiB; the maximum is 1 MiB. Default process timeout is 30 seconds; the maximum is five minutes. Output is drained and discarded bytes counted after the capture budget is exhausted. A serialized response larger than 4 MiB becomes a `response_too_large` failure because JSON escapes can expand text.

## Results and worker exit codes

Each response contains `status`, UTC timestamps, monotonic `duration_ms`, and either `output` or a stable error `code` and English `message`. `completed` for `exec` means the process was observed to exit; `exit_code` may be nonzero. `rejected`, `failed`, `cancelled`, and `timed_out` are distinct. Cancellation and timeout do not undo effects already produced.

The worker exits `0` after a valid protocol request even when the tool failed, `2` for malformed protocol input with a JSON rejection when possible, and `1` for a fatal worker or response write failure. `request_id` is for correlation, not deduplication. If the worker vanishes without a complete response, the host must treat the effect as unknown and reconcile before retrying.

## Architecture

`domain/` holds value objects and operation commands/results. `application/` holds ports and use cases. `adapters/local/` implements filesystem and process ports; `plugins/` adapts MCP stdio servers to the plugin tool port. `dto/` holds the JSON contract. `runtime/` composes the executor, codecs and mappers. `cmd/axlr/` is the worker adapter. No Go source lives at the repository root. Each Go file has one primary type where a type is needed.

The [MCP client package](mcpclient/README.md) also connects to external MCP servers over stdio or Streamable HTTP. It discovers and calls their tools by `(server, tool)` identity. The plugin adapter uses its stdio transport.

The [design](docs/plans/2026-09-29-hexagonal-design.md), [implementation plan](docs/plans/2026-09-29-minimal-runtime.md), and [provenance note](docs/provenance.md) record the boundaries and lineage. Sandboxing and comparative performance measurements are outside this delivery.
