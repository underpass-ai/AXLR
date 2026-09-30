# AXLR MCP client

`mcpclient` is the root Go module's client for external MCP servers. It uses the [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) over stdio or Streamable HTTP. AXLR's own `read`, `write`, `edit` and `exec` contract is not an MCP server.

## Connect, discover and call

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/underpass-ai/AXLR/mcpclient"
)

func main() {
    ctx := context.Background()
    client := mcpclient.New()
    defer client.Close()

    if err := client.Connect(ctx, mcpclient.Server{
        Name: "search",
        Command: "/absolute/path/to/search-mcp-server",
        Args: []string{"--stdio"},
    }); err != nil { log.Fatal(err) }

    tools, err := client.ListTools(ctx, "search")
    if err != nil { log.Fatal(err) }
    fmt.Println("tools:", len(tools))

    result, err := client.Call(ctx,
        mcpclient.ToolRef{Server: "search", Name: "find"},
        map[string]any{"query": "AXLR"})
    if err != nil { log.Fatal(err) }
    fmt.Printf("tool error: %t, content: %v\n", result.IsError, result.Content)
}
```

For Streamable HTTP, use `mcpclient.Server{Name: "search", URL: "https://example.com/mcp", HTTPClient: clientWithAuth}`. The host supplies authorization through its `http.Client`. For stdio, `Env: nil` inherits the host process environment; a non-nil `Env` is the child's complete environment. This low-level client differs from AXLR's TUI registration policy, which builds an explicit child environment for configured servers.

`ListTools` follows server pagination and returns the owning server with each tool, avoiding ambiguous names. `Call` sends one logical invocation and returns MCP `isError` as data. Automatic reconnect retries are disabled for the HTTP transport. A timeout or broken connection cannot prove whether an effectful call ran; inspect the target before retrying it.

Pass a context with a suitable deadline to `Connect`, `ListTools` and `Call`, and close the client to release sessions and child processes. Treat discovered descriptions, annotations, schemas and content as untrusted data. The host decides which servers and tools are authorized.

```bash
GOWORK=off go test -race ./mcpclient
```

For console registration, manifests and approval policies, see [Plugins and MCP](../docs/plugins.md). For the library's other entry points, see [Go library](../docs/library.md).
