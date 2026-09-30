# AXLR service API and Helm deployment

## Intent

AXLR originated as a simpler execution layer extracted from `underpass-runtime`, with Pi as a conceptual reference for a compact local tool surface. It is an agentic runtime: AXLR owns the model loop, sessions, tool execution and approval boundary; KMP governs durable memory; MADE governs orchestration. The first service API must let a client run agent turns and tools without a terminal. The same agent behavior should be available through the TUI and service.

## Decisions and scope

- Serve versioned HTTP JSON and Server-Sent Events (SSE) over TLS with required client certificates. Bind to loopback by default. A non-loopback bind is an explicit deployment setting.
- A Helm chart deploys AXLR only. KMP and MADE are existing remote services. Their current native topologies differ: KMP can use its authenticated HTTP MCP gateway or a local `kmp-mcp` adapter that forwards to remote gRPC; MADE exposes remote gRPC through a local `made-mcp` stdio adapter. The AXLR chart will use the **stdio adapter to remote gRPC** route for both engines, with release-matched adapter binaries in the AXLR image and client credentials from Kubernetes Secrets. This keeps AXLR's existing MCP contract while leaving memory and ceremony state in the remote services. A remote AXLR deployment fails readiness until both engines are reachable and their identities are verified.
- Keep one workspace root per service instance, mounted into the container. AXLR local file and process tools act inside that pod with its service-account and container permissions; the root path alone is not a sandbox. Do not expose arbitrary host paths in an API request.
- The first release supports sessions, model turns, streamed events, pending approvals, cancellation and direct tool calls. It does not create a public anonymous endpoint, a multi-tenant account system or a cluster scheduler.

## API contract

`POST /v1/sessions` creates a session with a chosen model and returns its ID, state and revision. `GET /v1/sessions/{id}` returns the current state and transcript projection. `POST /v1/sessions/{id}/turns` accepts a prompt plus an idempotency key, starts one turn and returns an operation ID. `GET /v1/sessions/{id}/events?after=<cursor>` streams ordered SSE events for text deltas, state changes, tool requests, approval decisions and completion. Events carry a stable event ID so a client can resume after disconnect without restarting a tool call.

`POST /v1/sessions/{id}/approvals/{call_id}` accepts one approve or deny decision against the pending call and session revision. A duplicate decision with the same key returns the prior outcome; a conflicting decision returns `409`. `POST /v1/sessions/{id}/cancel` requests cancellation and reports the resulting session state. Cancellation cannot roll back a completed external effect.

`GET /v1/tools` returns the available local tools and explicitly registered MCP tools. `POST /v1/tool-calls` accepts one exact tool identity, validated arguments and an idempotency key. It creates a pending call and returns a call ID, policy decision and revision; it does not bypass review. `POST /v1/tool-calls/{id}/decisions` accepts approve or deny with that revision. `GET /v1/tool-calls/{id}` returns status and result. An explicitly configured automatic policy may execute the call without a separate decision. mTLS alone authenticates the client but does not authorize tool effects. The response uses the worker's status and error meanings where applicable.

`GET /livez` reports process liveness. `GET /readyz` reports model configuration and required KMP/MADE connection readiness without leaking credentials or private tool data. Both are served on a separate loopback probe listener. Helm uses an exec probe through `axlr-serve probe`, because a kubelet HTTP probe cannot reach the container's loopback address. The public API listener always requires a valid client certificate. Readiness uses cached transport/discovery health and never generates a paid model completion. Unknown fields and oversized JSON bodies are rejected. Error responses include a stable code, message and request ID, without secrets.

## Components and state

The service adapter will reuse the existing `tui/application` agent turn, tool resolution and session use cases. The TUI and HTTP entry points must call the same application behavior; HTTP handlers cannot implement a separate agent loop. The server binary may initially live in the console module because the root module cannot import it without a dependency cycle. A later package extraction can move shared agent code to the root module without changing the API.

The current file session store uses per-session writer locks. The first service replica count is one, with a persistent volume for sessions and a bounded in-memory event buffer backed by durable event IDs. The Helm chart must reject or document replica counts above one until session storage and leases are cluster safe. The API must persist each state transition before telling a client that it is complete. A restart resumes an interrupted session; it never silently executes a pending tool call.

## Authentication, engines and Helm

The service requires a server certificate and verifies a client certificate against a configured CA. TLS material comes from mounted Secrets, never chart values rendered into ConfigMaps or logs. Client certificate fingerprints map to explicit caller roles (`session_client`, `approver`, `tool_operator`, `admin`) in a private policy file; unmatched clients fail closed. Client identity is recorded in audit events. Loopback is the development default; Helm binds inside the pod and exposes a ClusterIP service. External ingress is opt-in and must preserve end-to-end client certificate verification.

KMP and MADE are configured as distinct stdio MCP registrations. The release-matched `kmp-mcp` and `made-mcp` adapter processes connect to existing remote gRPC services using CA, client certificate, key and DNS identity from mounted Secrets. AXLR preserves exact tool identity (`kmp` or `made` plus tool name). KMP remains the authority for memory; MADE remains the authority for ceremony state and decisions. A missing or failed required engine makes readiness fail and tool calls return an explicit unavailable error; AXLR does not substitute local memory or orchestration. The chart does not install or delete either remote engine or its data.

## Release and compatibility

Versioned archives will contain `axlr`, `axlr-tui` and `axlr-serve` for the supported OS/architecture matrix once the service binary exists. A tagged release validates all target archives and checksums before attaching assets. Helm chart version and application version match the release tag; its image is pinned by digest in deployment guidance. The API starts at `/v1`; incompatible contract changes require a new path or explicit migration.

## Verification

Exercise the HTTP adapter with TLS client certificates: reject no certificate, wrong CA and invalid paths; create/resume a session; stream and reconnect to ordered events; approve, deny and replay a pending tool call; execute read and effectful tools under policy; cancel and restart without duplicate effects. Use fake model and MCP servers for deterministic tests. Run both Go module suites, cross-compile the release matrix and validate the Helm templates with required remote endpoints and Secrets. A native smoke test checks the packaged service binary on each release runner where available.
