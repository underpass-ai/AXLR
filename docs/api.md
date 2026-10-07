# Service API (source builds)

`axlr-serve` is the service adapter in the `tui` Go module. It reuses the console's turn and tool-resolution use cases. The JSON worker remains a one-request process. The REST route and payload reference is [OpenAPI v1](../api/openapi/axlr-v1.yaml). See [service operations](runbooks/service.md) for probes, a direct-read smoke check and recovery.

The same service capabilities are available through gRPC and an authenticated Streamable HTTP MCP server at `/mcp`. The [transport parity contract](transport-parity.md) lists all 18 verbs, shared schemas, approvals and event delivery; the OpenAPI document also describes `POST /v1/operations/{verb}` and `/mcp`. Set optional `grpc_listen` to a separate address such as `127.0.0.1:9443`; an omitted field leaves it disabled. Existing REST payloads and probe routes remain available.

Build with `GOWORK=off go -C tui build -trimpath -o /tmp/axlr-serve ./cmd/axlr-serve`; run `/tmp/axlr-serve --config=/absolute/path/service.json`. `--version` prints the injected build version. The service configuration is strict JSON (at most 64 KiB) with absolute file paths and explicit listeners. This template names existing files/services; replace every placeholder before launch:

```json
{
  "api_listen": "127.0.0.1:8443",
  "probe_listen": "127.0.0.1:8081",
  "workspace": "/absolute/workspace",
  "state_dir": "/absolute/private/axlr-state",
  "server_cert_file": "/absolute/secrets/api/tls.crt",
  "server_key_file": "/absolute/secrets/api/tls.key",
  "client_ca_file": "/absolute/secrets/client-ca/ca.crt",
  "principals_file": "/absolute/secrets/principals.json",
  "model_api_key_file": "/absolute/secrets/openrouter-key",
  "kmp": {
    "endpoint": "kmp.example.internal:50051",
    "server_name": "kmp.example.internal",
    "tls_dir": "/absolute/secrets/kmp",
    "command": "/absolute/bin/kmp-mcp"
  },
  "made": {
    "endpoint": "made.example.internal:50051",
    "server_name": "made.example.internal",
    "tls_dir": "/absolute/secrets/made",
    "command": "/absolute/bin/made-mcp"
  }
}
```

Each engine TLS directory contains `ca.crt`, `tls.crt` and `tls.key`. The adapters run over stdio and connect to remote gRPC with mutual TLS. Both engines are mandatory configuration entries. The service does not load console `mcp.json`, `settings.json` or plugin packages, and has no work-mode API or wired console ceremony driver. `/v1/sessions` starts normal sessions. Install engine policies/definitions through their operator channels; AXLR's service config does not bootstrap them.

The API listener requires TLS 1.3, a client certificate signed by the configured CA, and an entry in the private principals file. A certificate that chains to the CA but is not mapped is forbidden. The policy is JSON:

```json
{"version":1,"entries":[{"certificate_sha256":"64-lowercase-hex-characters","principal_id":"alice","roles":["session_client","approver","tool_operator"]}]}
```

`certificate_sha256` is SHA-256 of the client's DER leaf certificate. Roles are `session_client`, `approver`, `tool_operator`, and `admin`. Sessions belong to the creating principal; `admin` can access any session. An `approver` can decide a pending exact call. A `tool_operator` can create a direct call. mTLS authentication never approves a tool by itself.

All command bodies use `application/json`, UTF-8, at most 4 MiB, and reject unknown fields. `Idempotency-Key` (16–128 safe ASCII characters) is required for turns, approvals and direct calls. A repeated key with the same method, route, body and principal returns the original resource; conflicting reuse returns `409 idempotency_conflict`. `expected_revision` guards decisions and cancellation. Errors have `error.code`, `error.message` and `error.request_id`; the same ID is returned in `X-Request-Id`.

Typical flow:

1. `POST /v1/sessions` with `{"model":"provider/model"}`; save `id` and `revision`.
2. `POST /v1/sessions/{id}/turns` with `{"prompt":"...","expected_revision":1}` and an idempotency key; save the `operation_id`.
3. `GET /v1/sessions/{id}/events` with `Accept: text/event-stream`. Use `Last-Event-ID` or `?after=N` to resume after sequence `N`; the query takes precedence if both are supplied. A cursor ahead of the journal returns `400 invalid_cursor`. Closing the stream does not cancel the turn.
4. If an `approval.required` event appears, inspect the session's first pending call and `POST /v1/sessions/{id}/approvals/{call_id}` with `{"decision":"approve","expected_revision":N}` and a new idempotency key.

`GET /v1/tools` lists exact local and connected MCP identities and schemas. `POST /v1/tool-calls` creates a durable direct-call intent. Its `POST /v1/tool-calls/{id}/decisions` endpoint approves or denies that one call. Effectful calls persist a checkpoint before execution. An `uncertain` result after an interrupted effectful call requires operator inspection; AXLR does not retry it automatically or expose a reconciliation endpoint. The shipped service uses manual approvals for local and engine tools. There is no separate continue endpoint: interrupted pending calls can be decided through their approval route; an interrupted session with no pending calls can accept a new prompt.

The state directory contains session snapshots, event journals, call arguments and audit records. On Windows, service startup replaces inherited ACLs on that directory and its existing contents with access for the current account, SYSTEM and Administrators; it refuses reparse points. Use a dedicated directory owned by the service account.

`/livez` and `/readyz` are available only on the separate loopback probe listener. Readiness checks a configured nonempty model key and discovery through both KMP and MADE MCP adapters. It does not verify account credit, make a model completion or prove that an engine write/ceremony action is authorized. Missing or empty model credentials stop startup; engine transport failures can leave the API alive with failing readiness. `axlr-serve --probe=livez` and `--probe=readyz` always query loopback port 8081, matching the chart; query the configured endpoint directly if you use another probe port.
