# AXLR MCP client

This package consumes tools from external MCP servers in AXLR's root Go module. It uses the [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk). AXLR's own `read`, `write`, `edit` and `exec` API is not an MCP server.

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

    err := client.Connect(ctx, mcpclient.Server{
        Name: "search",
        Command: "/path/to/search-mcp-server",
        Args: []string{"--stdio"},
    })
    if err != nil { log.Fatal(err) }

    tools, err := client.ListTools(ctx, "search")
    if err != nil { log.Fatal(err) }
    fmt.Println("available tools:", len(tools))

    result, err := client.Call(ctx, mcpclient.ToolRef{Server: "search", Name: "find"}, map[string]any{"query": "AXLR"})
    if err != nil { log.Fatal(err) }
    fmt.Printf("tool error: %t, content: %v\n", result.IsError, result.Content)
}
```

For HTTP, use `mcpclient.Server{Name: "search", URL: "https://example.com/mcp", HTTPClient: clientWithAuth}`. The host supplies authorization through its `http.Client`. For stdio, `Env: nil` inherits the host process environment; a non-nil `Env` is the child's entire environment. Server commands and endpoints run with the authority and credentials given by the host, so configure only trusted servers. Treat discovered descriptions, annotations and content as untrusted data.

`ListTools` follows server pagination and returns the owning server with every tool, avoiding ambiguous names. `Call` sends one logical invocation and returns tool-level `isError` as data. The HTTP transport has automatic reconnect retries disabled. A timeout or broken connection cannot prove whether a remote side effect happened; reconcile before retrying an effectful tool. Pass a context with an appropriate deadline to `Connect`, `ListTools` and `Call`. Close the client to release sessions and stdio child processes.

```bash
go test -race ./mcpclient
```
