# AXLR external tool plugins: design

## Purpose and approved direction

AXLR is Underpass's own runtime, selectively rewritten from `underpass-runtime`. Plugins add agent tools to AXLR. They are external processes that speak MCP over stdio. The same plugin tools must be available through the Go library and the existing one-request `axlr` worker. One AXLR binary serves local and plugin requests. AXLR does not publish its local tools as an MCP server.

The user chose external processes, tool contributions (not execution hooks), MCP stdio, and a single binary. This design preserves the four existing local tools and their JSON behavior.

## Chosen approach and trade-off

The existing `mcpclient` wraps the official Go MCP SDK, but currently lives in a nested Go module. To compose it into the existing worker, fold it into the root Go module without changing its import path, then add a `plugins/` registry over it. Existing stdio and Streamable HTTP client behavior remains available. This gives one binary and one authoritative MCP client implementation. The root module gains the official SDK and its transitive dependencies; CI uses the Go module cache and runs a single race/coverage job. A separate plugin-enabled binary would keep the local core dependency free but violate the requested single-binary experience.

## Plugin registration and authority

The host explicitly supplies one or more plugin manifest paths. There is no directory scanning, remote installation, hot reload, or arbitrary plugin execution merely because a file exists. Each JSON manifest is at most 64 KiB and has the shape `{"manifest_version":1,"id":"search","command":"/absolute/path/search-server","args":[],"allow_tools":["find"]}`. Unknown fields, duplicate IDs, invalid names, NUL bytes and oversized manifests are rejected before any plugin starts. The allowlist prevents a server update from silently exposing new tools. MCP descriptions, annotations and schemas never grant authority.

`cmd/axlr` accepts repeatable `--plugin /absolute/path/manifest.json` flags. Plugins receive no inherited environment by default; the CLI supplies values with repeatable `--plugin-env ID:KEY=VALUE` flags, and the Go API supplies an environment explicitly. The plugin executable runs with the host account's OS access. AXLR is not a sandbox and does not ask for installation or execution permission on behalf of the host; the host's explicit configuration is the authority. Plugin stdout is reserved for MCP and stderr is not included in the worker's JSON response.

## Library and worker contract

`plugins.Manager` loads manifests, starts MCP stdio sessions lazily, lists allowed tools, calls one allowed tool by a typed `(PluginID, ToolName)` reference, and closes all sessions. The same manager implements an `application.PluginToolPort`; `runtime.Config` optionally accepts that port. `domain/` owns the typed plugin identity and call command; `dto/` owns the wire arguments and outputs. The domain and application layers do not import MCP types.

The worker keeps `protocol_version: 1` and adds two tool names:

- `plugins.list` with `arguments: {}` returns the configured, currently available allowlisted tools. Each item includes plugin ID, MCP tool name, description and input/output schemas.
- `plugins.call` with `arguments: {"plugin_id":"...","tool_name":"...","arguments":{...}}` invokes one allowed tool and returns MCP content blocks, optional structured content and `is_error`.

The Go library can use the manager directly or pass it to `runtime.New` and send the same typed requests to `Executor.Execute`. A worker starts only the plugin needed for `plugins.call`; `plugins.list` connects to all configured plugins. Local tool requests do not start plugins. In a long-lived Go host, successful plugin sessions are reused. The one-request worker closes its sessions after emitting its response.

An MCP tool result with `is_error: true` produces AXLR status `completed` with `is_error` retained in the output. An unknown plugin, unallowed/unknown tool or malformed request is rejected. A plugin launch or protocol failure is `failed`; cancellation and deadline expiry retain their own status. `plugins.list` fails as a whole if any configured plugin cannot be reached, so callers never mistake a partial catalog for complete discovery. Lost responses and timeouts do not prove an effect did not happen, so AXLR never automatically retries an effectful call. Responses remain within the existing 4 MiB worker bound.

## Boundaries and acceptance

No Go source lives at the repository root. Keep one primary type per Go file, ports/adapters/mappers/DTOs/use cases in their layers, and avoid primitive identity strings at the domain boundary. MCP is only the plugin process protocol and existing external-tool client transport; AXLR's local tools are not served over MCP.

Acceptance requires tests with a real helper MCP stdio server for discovery, allowlist enforcement, plugin calls, tool-level errors, process shutdown, cancellation, failure propagation and unchanged local tools. Add unit tests for strict manifests, duplicate IDs, environment isolation and worker framing. Run `go vet`, race tests, a static worker build and aggregate coverage above 80%. CI must stay a minimal single Go job with dependency caching and the existing root-layout check.
