# Service API (source builds)

`axlr-serve` is the HTTP adapter in the `tui` Go module. It reuses the console's turn and tool-resolution use cases. The JSON worker remains a one-request process. The complete route and payload contract is [OpenAPI v1](../api/openapi/axlr-v1.yaml).

Build with `GOWORK=off go -C tui build -trimpath -o /tmp/axlr-serve ./cmd/axlr-serve`; run `/tmp/axlr-serve --config=/absolute/path/service.json`. `--version` prints the injected build version. The service configuration is strict JSON with absolute paths. The Helm chart renders a usable example of the configuration keys in its ConfigMap.

The API listener requires TLS 1.3, a client certificate signed by the configured CA, and an entry in the private principals file. A certificate that chains to the CA but is not mapped is forbidden. The policy is JSON:

```json
{"version":1,"entries":[{"certificate_sha256":"64-lowercase-hex-characters","principal_id":"alice","roles":["session_client","approver","tool_operator"]}]}
```

`certificate_sha256` is SHA-256 of the client's DER leaf certificate. Roles are `session_client`, `approver`, `tool_operator`, and `admin`. Sessions belong to the creating principal; `admin` can access any session. An `approver` can decide a pending exact call. A `tool_operator` can create a direct call. mTLS authentication never approves a tool by itself.

All command bodies use `application/json`, UTF-8, at most 4 MiB, and reject unknown fields. `Idempotency-Key` (16–128 safe ASCII characters) is required for turns, approvals and direct calls. A repeated key with the same method, route, body and principal returns the original resource; conflicting reuse returns `409 idempotency_conflict`. `expected_revision` guards decisions and cancellation. Errors have `error.code`, `error.message` and `error.request_id`; the same ID is returned in `X-Request-Id`.

Typical flow:

1. `POST /v1/sessions` with `{"model":"provider/model"}`; save `id` and `revision`.
2. `POST /v1/sessions/{id}/turns` with `{"prompt":"...","expected_revision":1}` and an idempotency key; save the `operation_id`.
3. `GET /v1/sessions/{id}/events` with `Accept: text/event-stream`. Use `Last-Event-ID` or `?after=N` to resume after sequence `N`. Closing the stream does not cancel the turn.
4. If an `approval.required` event appears, inspect the session's first pending call and `POST /v1/sessions/{id}/approvals/{call_id}` with `{"decision":"approve","expected_revision":N}` and a new idempotency key.

`GET /v1/tools` lists exact local and connected MCP identities and schemas. `POST /v1/tool-calls` creates a durable direct-call intent. Its `POST /v1/tool-calls/{id}/decisions` endpoint approves or denies that one call. Effectful calls persist a checkpoint before execution. An `uncertain` result after a crash or lost response requires human inspection; AXLR does not retry it automatically.

The state directory contains session snapshots, event journals, call arguments and audit records. On Windows, service startup replaces inherited ACLs on that directory and its existing contents with access for the current account, SYSTEM and Administrators; it refuses reparse points. Use a dedicated directory owned by the service account.

`/livez` and `/readyz` are available only on the separate loopback probe listener. Readiness requires configured model credentials and reachable KMP and MADE MCP adapters. Transport or identity failures keep the API process alive for diagnosis and readiness failing.
