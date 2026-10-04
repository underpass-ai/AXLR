# AXLR MCP client design

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../index.md) and its audited revision.

AXLR consumes external MCP tools through an optional Go module. AXLR remains Underpass's own runtime, selectively rewritten from `underpass-runtime`. The existing `read`, `write`, `edit` and `exec` executor and worker retain their own contract and are not published as MCP tools.

## Boundary

`mcpclient` owns MCP protocol sessions and depends on the official Go MCP SDK. The root module remains dependency free and handles local execution. A host composes the two libraries according to its own tool policy. This placement keeps protocol parsing and transport concerns out of the local domain and keeps the existing CI path fast.

The host provides a named `Server` configuration. Exactly one transport is selected: a child command using MCP stdio, or a Streamable HTTP URL with an optional configured `http.Client`. The host controls credentials, environment, command authority, deadlines and tool policy. The client closes child sessions on shutdown.

`ServerName` and `ToolName` are value objects. `ToolRef` combines them, so tools on different servers with the same name remain distinct. `ListTools` returns these references with descriptions and JSON schemas. `Call` takes a `ToolRef` and an object of arguments, then preserves MCP content blocks, optional structured content and the tool-level error flag. Protocol and transport failures are Go errors. A host can therefore present an external tool failure to an agent without confusing it with loss of the MCP session.

## Failure and trust rules

Tool descriptions, schemas and results originate outside AXLR and must be treated as untrusted data. Tool annotations do not authorize effects. An MCP call may have performed an effect before a timeout or disconnection; the HTTP transport disables automatic reconnect retries, and the caller must reconcile before retrying an effectful call. Tool discovery follows cursor pagination with a repeated-cursor check and a 1000-page limit. Calls and discovery use caller-supplied contexts for cancellation and deadlines.

The client library intentionally does not expose an MCP server, silently route `read`/`write`/`edit`/`exec` through MCP, or decide which discovered tools an agent may invoke. Those decisions belong to the embedding host.
