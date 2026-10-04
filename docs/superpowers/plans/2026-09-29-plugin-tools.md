# External Tool Plugins Implementation Plan

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../../index.md) and its audited revision.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add explicitly registered MCP stdio tool plugins to the AXLR Go library and existing one-request worker.

**Architecture:** Domain value objects and an application port describe plugin identities, tools, calls and results without importing MCP. A `plugins/` adapter loads strict manifests and manages the existing MCP client lazily. `runtime/` maps two new request names to use cases; `cmd/axlr/` supplies manifests and per-plugin environment while preserving local tool behavior.

**Tech Stack:** Go 1.26, official Go MCP SDK v1.8.0, MCP stdio.

**Spec:** `docs/superpowers/specs/2026-09-29-plugin-tools-design.md`

## Global Constraints

- AXLR is Underpass's own runtime.
- One binary, `cmd/axlr`, handles local and plugin requests; local `read`/`write`/`edit`/`exec` JSON behavior is unchanged.
- Plugins are external MCP stdio processes contributing tools only; no hooks and no AXLR MCP server.
- Manifests are explicit, strict JSON, at most 64 KiB, with version 1, absolute command, argv and nonempty allowlist. No auto-discovery or hot reload.
- Plugin processes get no inherited environment by default. No shell launch or automatic retry of effectful calls.
- Keep no `.go` source at repository root, one primary type per Go file, aggregate race-test coverage above 80%, and cached minimal CI.
- Keep the existing 4 MiB worker response bound and distinguish MCP `is_error` from transport failures.

## Review Focus

- Malformed or duplicate manifests must fail before starting any plugin; Task 2 tests wrong version, unknown field, duplicate ID and oversized file.
- A configured plugin must not expose a newly advertised tool outside `allow_tools`; Task 3 tests it on a real MCP helper.
- A local read with plugins configured must not launch them; Task 4 and Task 5 test lazy startup.
- A cancelled or timed-out plugin call may have effects; Task 3 and Task 5 test cancellation with no retry.
- Plugin stdout or stderr must not corrupt the worker's one-document response; Task 5 tests framing.

---

### Task 1: Typed plugin contract and port

**Files:** Create `domain/plugin_id.go`, `domain/plugin_tool_name.go`, `domain/plugin_ref.go`, `domain/json_value.go`, `domain/plugin_tool.go`, `domain/plugin_call.go`, `domain/plugin_result.go`, `domain/plugin_list_command.go`, `application/plugin_tool_port.go`, `dto/plugin_list_args.go`, `dto/plugin_call_args.go`, `dto/plugin_tool_output.go`, `dto/plugin_call_output.go`; test in `domain/plugin_values_test.go` and `dto/plugin_contract_test.go`.

**Interfaces:** `NewPluginID(string) (PluginID,error)` accepts `[A-Za-z][A-Za-z0-9_-]*`. `NewPluginToolName(string) (PluginToolName,error)` follows MCP's max-128-byte `[A-Za-z0-9_.-]+` grammar. `PluginRef` holds both. `JSONValue` copies and validates raw JSON; plugin call arguments must be an object. `PluginTool` contains ref, description and schema JSON values; `PluginResult` contains content JSON values, optional structured JSON and `IsError`. `application.PluginToolPort` declares `List(context.Context) ([]domain.PluginTool,error)` and `Call(context.Context,domain.PluginCall) (domain.PluginResult,error)`.

- [ ] **Step 1:** Write tests proving invalid IDs/names/JSON are rejected, values cannot be mutated through returned bytes, and DTOs marshal to the spec's `plugins.list`/`plugins.call` shapes.
- [ ] **Step 2:** Run `go test ./domain ./dto`; expect compile failure for the missing types.
- [ ] **Step 3:** Implement the listed types and port, with one primary type per file.
- [ ] **Step 4:** Run `go test ./domain ./dto ./application`; expect pass.
- [ ] **Step 5:** Commit `feat: define typed plugin tool contract`.

### Task 2: Strict manifest registration

**Files:** Create `plugins/manifest.go`, `plugins/registration.go`, `plugins/manifest_test.go`.

**Interfaces:** `LoadManifest(path string) (Manifest,error)` reads at most 64 KiB, strictly decodes `manifest_version`, `id`, `command`, `args`, `allow_tools`, checks the exact version and validates identities. `NewRegistration(manifest Manifest, env []string) (Registration,error)` requires an absolute command and validates the complete child environment. It stores an empty non-nil environment when `env` is nil: `mcpclient.Server` otherwise inherits the host environment. Duplicate IDs are checked before manager construction in Task 3.

- [ ] **Step 1:** Write table tests for valid manifest, 64 KiB limit, unknown or duplicate JSON field, wrong version, relative command, empty allowlist, duplicate allowlist entry, NUL argv/env, and `NewRegistration(manifest,nil)` yielding a non-nil empty environment.
- [ ] **Step 2:** Run `go test ./plugins`; expect compile failure.
- [ ] **Step 3:** Implement strict parsing and registration validation without starting a process.
- [ ] **Step 4:** Run `go test ./plugins`; expect pass.
- [ ] **Step 5:** Commit `feat: validate explicit plugin manifests`.

### Task 3: MCP-backed plugin manager and one Go module

**Files:** Create `plugins/manager.go`, `plugins/manager_test.go`; modify `go.mod`, `.github/workflows/ci.yml`, `mcpclient/README.md`; remove `mcpclient/go.mod` and `mcpclient/go.sum`; generate root `go.sum`.

**Interfaces:** `NewManager(registrations []Registration) (*Manager,error)` rejects duplicate IDs without launching them. `Manager.List(ctx) ([]domain.PluginTool,error)` connects all configured plugins, filters by allowlist, errors on any unavailable plugin. `Manager.Call(ctx,domain.PluginCall) (domain.PluginResult,error)` connects only the named plugin, rejects unallowed tools and forwards exactly one MCP call. `Manager.Close() error` closes child sessions. Manager implements `application.PluginToolPort` and offers a Go library API directly.

- [ ] **Step 1:** Write tests with an actual MCP stdio helper for duplicate registration, discovery filtering, call success, `is_error`, missing plugin/tool, allowlisted tool absent from server, cancellation with no retry, child shutdown, one process per plugin session and actual environment isolation.
- [ ] **Step 2:** Run `go test ./plugins`; expect failure before implementation.
- [ ] **Step 3:** Fold `mcpclient` into the root module, update cached CI, and implement lazy session ownership and domain mapping through the existing client.
- [ ] **Step 4:** Run `go test -race ./plugins ./mcpclient` and `go vet ./...`; expect pass, including existing HTTP-client tests.
- [ ] **Step 5:** Commit `feat: manage MCP stdio tool plugins`.

### Task 4: Runtime use cases and JSON routing

**Files:** Create `application/list_plugin_tools_use_case.go`, `application/call_plugin_tool_use_case.go`, `runtime/plugin_test.go`; modify `runtime/config.go`, `runtime/executor.go`, `runtime/request_mapper.go`, `runtime/response_mapper.go`, `runtime/codec.go`.

**Interfaces:** `runtime.Config.Plugins application.PluginToolPort` is optional. `RequestMapper.Map` accepts `plugins.list` with `{}` and `plugins.call` with typed `dto.PluginCallArgs`; it returns `domain.PluginListCommand` or `domain.PluginCall`. Executor delegates to the two use cases. `ResponseMapper` returns `dto.PluginToolOutput` or `dto.PluginCallOutput`. `Decode` and `MarshalResponse` accept the two new names without changing existing v1 requests.

- [ ] **Step 1:** Write tests that plugin list/call requests map and execute, remote `is_error` stays `completed`, unknown plugin is `rejected`, protocol failure is `failed`, cancellation/timeout are distinct, and local read never touches the plugin port.
- [ ] **Step 2:** Run `go test ./runtime ./application`; expect failure from missing routes and types.
- [ ] **Step 3:** Implement the use cases and minimal runtime mapping, retaining the existing 4 MiB codec bound.
- [ ] **Step 4:** Run `go test -race ./runtime ./application`; expect pass and unchanged local tests.
- [ ] **Step 5:** Commit `feat: route plugin tools through AXLR runtime`.

### Task 5: Worker composition, documentation and final verification

**Files:** Create `cmd/axlr/plugin_flags.go`, `cmd/axlr/plugin_env_flags.go`, `cmd/axlr/plugin_worker_test.go`; modify `cmd/axlr/worker.go`, `README.md`, `docs/plans/2026-09-29-hexagonal-design.md` and `.github/workflows/ci.yml` as needed.

**Interfaces:** Repeatable `--plugin ABSOLUTE_MANIFEST` and `--plugin-env ID:KEY=VALUE` flags construct `plugins.Manager` without starting plugin processes. The worker passes it to `runtime.Config`, closes it after the response, and rejects an env override for an unregistered plugin. CLI remains a one-request JSON worker.

- [ ] **Step 1:** Write worker tests for `plugins.list`, `plugins.call`, one JSON response, per-plugin environment, local read without plugin launch, malformed manifest and process cancellation.
- [ ] **Step 2:** Run `go test ./cmd/axlr`; expect failure from missing flags and routes.
- [ ] **Step 3:** Implement CLI composition and document manifest, tool contract, authority and effect uncertainty; keep one cached CI job.
- [ ] **Step 4:** Run `gofmt` check, `go vet ./...`, `go test -race -count=1 -coverpkg=./... -coverprofile=coverage.out ./...` with total above 80%, `CGO_ENABLED=0 go build ./cmd/axlr`, and verify zero root `.go` files.
- [ ] **Step 5:** Commit `feat: expose plugins in AXLR worker`; request whole-branch review and open a draft PR after fixes.
