# MCP Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Let AXLR consume tools exposed by external MCP servers, without serving AXLR's local tools over MCP.

**Architecture:** A separate `mcpclient` Go module wraps the official Go MCP SDK. Its public client owns named server sessions, exposes paginated tool discovery and invokes tools by `(server, tool)` identity. Callers compose this client with the existing AXLR executor; the root module and one-shot worker keep their current contracts and zero external dependencies.

**Tech Stack:** Go 1.26; `github.com/modelcontextprotocol/go-sdk` v1.8.0; stdio and Streamable HTTP.

**Spec:** `docs/plans/2026-09-29-mcp-client-design.md` and existing AXLR constraints in `docs/plans/2026-09-29-hexagonal-design.md`.

## Global Constraints

- AXLR is Underpass's own selective rewrite of `underpass-runtime`.
- Hexagonal boundaries, typed DTOs and value objects, one primary type per Go file, tests above 80%, fast CI.
- No MCP server endpoint for `read`, `write`, `edit`, or `exec`.
- The host owns server commands, URLs and credentials. MCP tool descriptions and annotations are untrusted data.

## Review Focus

- Duplicate tool names across servers: preserve `(server, tool)` identity, test with two servers.
- Server pages repeating a cursor: stop with an explicit error, test a scripted server.
- A tool-level `isError` response: return the result as data, not a transport failure, test with an in-memory server.
- Cancelled or lost call responses: propagate context/protocol failure without implicit call retry, test cancellation.
- Malformed server config and invalid arguments: reject before connection or call, test each.

---

### Task 1: Typed endpoint configuration

**Files:** Create `mcpclient/go.mod`, `mcpclient/server.go`, `mcpclient/server_name.go`, `mcpclient/tool_name.go`, `mcpclient/tool_ref.go`, and tests.

**Interfaces:** `Server` value with `Name`, `Command`, `Args`, `Env`, `URL`, `HTTPClient`; `Server.Validate()` checks exactly one transport and valid identity; `NewServerName` and `NewToolName` construct identity values. Internal `transport()` creates official SDK transport.

- [x] Write tests for invalid identity, mixed transport, malformed URL, and valid stdio/HTTP configurations.
- [x] Run `go test ./...` within `mcpclient`; confirm feature failure.
- [x] Implement validation and transport construction.
- [x] Run tests; confirm pass.

### Task 2: Discovery and invocation

**Files:** Create `mcpclient/client.go`, `mcpclient/tool.go`, `mcpclient/result.go`, `mcpclient/client_test.go`.

**Interfaces:** `New()` returns `*Client`; `Connect(ctx, Server) error`; `ListTools(ctx, ServerName) ([]Tool, error)`; `Call(ctx, ToolRef, map[string]any) (Result, error)`; `Close() error`. Tool and Result expose JSON-compatible SDK schemas/content without discarding structured output.

- [x] Write failing in-memory and HTTP integration tests for two servers, pagination, tool success, tool error, unknown server/tool, cancellation.
- [x] Run tests; confirm feature failure.
- [x] Implement session ownership, pagination and call forwarding; disable HTTP reconnect retries.
- [x] Run tests; confirm pass.

### Task 3: Documentation and CI

**Files:** Modify `README.md`, `.github/workflows/ci.yml`; create `mcpclient/README.md`.

- [x] Add an executable consumer example and state the effect/credential boundaries.
- [x] Add a module-specific CI job that measures coverage above 80% and runs race tests, leaving root job unchanged.
- [x] Run root and MCP test suites, coverage, vet, formatting and static root build.
- [x] Commit and open a draft PR targeting the existing AXLR baseline branch.
