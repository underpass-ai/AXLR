# HTTP, gRPC and MCP service parity

`axlr-serve` exposes one inventory of 18 capabilities through HTTP operations,
gRPC and Streamable HTTP MCP. They share the existing session store, journal,
tool resolver, approval policy, audit trail and idempotency store. Existing
REST `/v1` routes remain available.

This contract covers the service/worker capabilities below. Console work modes,
package installation, skill reading, repair controls and driven ceremony UI
remain console capabilities. KMP/MADE own their engine contracts; these AXLR
transports do not add missing engine RPCs or downstream MCP Apps negotiation.

## Connections and identity

The HTTPS listener serves `/mcp` and `POST /v1/operations/{RPCName}`. Set optional
`grpc_listen` (for example `127.0.0.1:9443`) to start a separate gRPC listener.
Both require TLS 1.3 and a verified client certificate, mapping its fingerprint
through the same principals file. CA-signed but unmapped certificates are
forbidden. `session_client`, `tool_operator`, `approver` and `admin` have the
same meaning on every transport; authentication alone does not approve effects.

MCP uses stateless HTTP requests authenticated independently. A session ID does
not transfer authority between callers. gRPC uses the verified TLS peer
certificate, never a client-supplied principal. Helm leaves gRPC disabled by
default; `grpc.enabled=true` configures container port 9443 and the corresponding
Service/NetworkPolicy ports. `grpc.port` controls the Service port. `/mcp` uses
the existing HTTPS port; original probes remain on loopback.

## Capability ledger

Each row has one HTTP operation, one RPC in `underpass.axlr.v1.AxlrService` and
one MCP tool. `tui/service/operations.go` supplies their common schemas and
constructs both runtime inventories. The checked public contract is
[axlr.proto](../api/proto/underpass/axlr/v1/axlr.proto).

Common optional fields are `request_id` and `idempotency_key`. Where required,
keys contain 16–128 characters from `A-Za-z0-9._~-`. IDs and uint64 revisions
are the same values returned by the REST API.

| HTTP operation / RPC | MCP tool | Required arguments | Shared behavior |
|---|---|---|---|
| `Read` | `axlr_read` | `arguments`, `idempotency_key` | Create a durable `local_read` intent |
| `Write` | `axlr_write` | `arguments`, `idempotency_key` | Create a durable `local_write` intent |
| `Edit` | `axlr_edit` | `arguments`, `idempotency_key` | Create a durable `local_edit` intent |
| `Exec` | `axlr_exec` | `arguments`, `idempotency_key` | Create a durable `local_exec` intent |
| `ListPlugins` | `axlr_list_plugins` | — | List registered external identities and schemas |
| `CallPlugin` | `axlr_call_plugin` | `plugin_id`, `tool_name`, `arguments`, `idempotency_key` | Resolve an exact pair from a frozen catalogue; create its durable intent |
| `CreateSession` | `axlr_create_session` | `model` | Create a normal session |
| `GetSession` | `axlr_get_session` | `session_id` | Read owned state, transcript and pending calls |
| `StartTurn` | `axlr_start_turn` | `session_id`, `prompt`, `expected_revision`, `idempotency_key` | Start the existing model/tool loop |
| `StreamEvents` | `axlr_stream_events` | `session_id` | Replay/follow the journal with a resume cursor |
| `DecideSessionTool` | `axlr_decide_session_tool` | `session_id`, `call_id`, `decision`, `expected_revision`, `idempotency_key` | Approve/deny the exact pending call |
| `CancelSessionTurn` | `axlr_cancel_session_turn` | `session_id`, `expected_revision` | Cancel without undoing completed effects |
| `ListTools` | `axlr_list_tools` | — | List identities/schemas; optional `origin` is `local` or `mcp` |
| `CreateToolCall` | `axlr_create_tool_call` | `tool`, `arguments`, `idempotency_key` | Create an exact registered call intent |
| `GetToolCall` | `axlr_get_tool_call` | `call_id` | Read durable status/result |
| `DecideToolCall` | `axlr_decide_tool_call` | `call_id`, `decision`, `expected_revision`, `idempotency_key` | Approve/deny one direct call |
| `Liveness` | `axlr_liveness` | — | Authenticated process liveness |
| `Readiness` | `axlr_readiness` | — | Authenticated model/engine discovery readiness |

Local and plugin aliases return a call ID, revision and status, normally
`pending_approval`. Use `DecideToolCall`, then `GetToolCall`. Unlike the
trusted-local one-request worker, remote clients execute through the service's
approval policy. `arguments` uses the selected tool's existing schema. Unknown
fields, invalid types and oversized requests are rejected.

## Requests, results and errors

HTTP operations accept an `application/json` object. For example, POST to
`/v1/operations/CreateSession` with:

```json
{"model":"provider/model","request_id":"example-create"}
```

MCP uses the same object as `arguments` of `axlr_create_session`. gRPC uses
`google.protobuf.BytesValue` containing that object's exact UTF-8 JSON bytes.
This avoids protobuf Struct/float64 conversions of revisions or tool numbers.
Consumers must likewise preserve numbers when encoding and reading JSON.

Successful unary responses contain the shared envelope:

```json
{"status_code":201,"request_id":"example-create","body":{"id":"...","model":"provider/model","status":"idle","revision":1}}
```

HTTP operations use the envelope's status as their response code. Probe
projections use HTTP 200 with `status_code:204` so HTTP does not discard the
envelope. MCP returns it as `structuredContent` and exact JSON text. Application
errors set `isError:true` and retain the existing `body.error` vocabulary.

gRPC maps input failures to `InvalidArgument`, forbidden to `PermissionDenied`,
missing resources to `NotFound`, revision/idempotency/state conflicts to
`FailedPrecondition`, request/stream limits to `ResourceExhausted`, unavailable
engines to `Unavailable`, and remaining failures to `Internal`.
`google.rpc.ErrorInfo` retains the service code in `reason`, the service name in
`domain`, and `http_status`/`result_json` metadata containing the full envelope.
Transport failures can prevent an envelope from being returned.

Idempotency uses principal and the underlying REST method, route and body,
not the protocol name. HTTP can create a call, gRPC approve it, and MCP
retrieve/replay it without executing another effect. Changed arguments or
identities conflict. Plugin replay retains the persisted exact identity and
does not require rediscovery when an engine is unavailable.

Go callers can generate clients from the proto or use the standard connection:

```go
output := new(wrapperspb.BytesValue)
err := conn.Invoke(ctx,
    "/underpass.axlr.v1.AxlrService/ListTools",
    wrapperspb.Bytes([]byte(`{"origin":"local"}`)), output)
// conn uses credentials.NewTLS with the client certificate and CA.
// On success, output.Value is the exact result-envelope JSON.
```

## Events and cancellation

`StreamEvents` optionally accepts `after` (exclusive uint64 cursor), `max_events`
(1–256; default 128), and `wait_ms` (0–30000; default 0). It replays existing
events then follows the journal for the requested wait. Zero wait returns a
snapshot. The result contains `events`, `next_sequence` and `end_reason`:
`snapshot`, `event_limit`, `wait_elapsed`, `cancelled` or `server_stopping`.
Cursors ahead of the journal fail. Reading never advances session state.

HTTP operations return that bounded result. gRPC emits each event immediately
as a BytesValue with `{"kind":"event","event":...}`, then the shared final
envelope including the complete bounded page. Apply either live frames or the
final page, not both; use the final cursor to resume.

MCP callers supplying `_meta.progressToken` receive events live through
`notifications/progress`: `message` contains exact event JSON and `progress`
tracks its journal sequence. The final tool result contains the same page.
Clients without progress support read the final page. Transport cancellation
does not cancel the model turn; use `CancelSessionTurn` explicitly. Original
REST SSE remains an unbounded stream with `Last-Event-ID` support.

## Verification

`tui/service/transport_parity_test.go` exercises all 18 verbs over real mTLS HTTP,
gRPC and MCP with fake model/tool ports. It checks colliding plugin names,
cross-transport approval/replay executing once, ordered events and MCP live
progress, roles/unmapped certificates, exact uint64 values and proto/catalogue
schema equality. Existing tests cover persistence, recovery, audit, revisions
and cancellation races. No live model or engine credentials are needed.
