# Go library

Use the root module when your host owns the session, model selection, authorization and lifecycle. AXLR supplies a typed local executor, OpenRouter completion and streaming adapters, and an MCP client.

## Local execution

Create one `runtime.Executor` per workspace, call `Execute` with a `dto.Request`, and close it when the host stops. An executor serializes requests. The host can lower `Config` limits but a request cannot raise them.

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"

    "github.com/underpass-ai/AXLR/dto"
    "github.com/underpass-ai/AXLR/runtime"
)

func main() {
    executor, err := runtime.New(runtime.Config{Root: "."})
    if err != nil { log.Fatal(err) }
    defer executor.Close()

    response := executor.Execute(context.Background(), dto.Request{
        ProtocolVersion: 1,
        RequestID: "read-1",
        Tool: "read",
        Arguments: json.RawMessage(`{"path":"README.md"}`),
    })
    if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
        log.Fatal(err)
    }
    fmt.Fprintln(os.Stderr, response.Status)
}
```

`Config.Root` must be an existing workspace. `Config.Env` is the complete environment for local child processes. `Config.Plugins` accepts the plugin tool port, for example a `plugins.Manager` built from explicit registrations. The [worker contract](worker.md) describes the same request and response semantics.

## Model completions

The OpenRouter adapter requires an API key supplied by the host. AXLR does not load it from the environment on behalf of the library caller. Non-streaming calls use `application.CompleteModelUseCase`; `application.StreamModelUseCase` emits text deltas and returns a validated complete result.

```go
client, err := openrouter.New(openrouter.ClientConfig{APIKey: os.Getenv("OPENROUTER_API_KEY")})
if err != nil { return err }
model, err := domain.NewModelID("provider/model")
if err != nil { return err }
request := domain.CompletionRequest{
    Model: model,
    Messages: []domain.Message{{Role: domain.RoleUser, Content: "Say hello"}},
}
result, err := (application.CompleteModelUseCase{Models: client}).Execute(ctx, request)
```

The caller chooses a model its OpenRouter account supports. To offer tools, set `request.Tools` to typed `domain.ToolDefinition` values with JSON Schema parameters. The result may contain text and several `result.Message.ToolCalls`; AXLR does not execute them for a library host. Check every requested name against the host's authorized registry, execute allowed calls, append the assistant's call message and one matching `RoleTool` message per result, then call the model again with the same tool definitions. Keep call IDs and results in conversation history.

## MCP client

`mcpclient.Client` connects to named external MCP servers over stdio or Streamable HTTP. It discovers paginated tool lists and calls a tool by `(server, name)` identity. A Go host can supply an `http.Client` with its own authorization for an HTTP server. See the [MCP client package guide](../mcpclient/README.md) for a complete example and retry semantics.

## Modules and checks

The root module contains the library and JSON worker. `tui/` is a separate module that uses the root library from the same checkout. Run both gates:

```bash
GOWORK=off go test ./...
GOWORK=off go -C tui test ./...
```

CI also runs vet, race tests, coverage and static builds for both modules. See [Architecture](architecture.md) for package ownership.
