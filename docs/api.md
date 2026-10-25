# Service API (source builds)

`axlr-serve` is the service adapter in the `tui` Go module. It reuses the console's turn and tool-resolution use cases. The JSON worker remains a one-request process. The REST route and payload reference is [OpenAPI v1](../api/openapi/axlr-v1.yaml). See [service operations](runbooks/service.md) for probes, a direct-read smoke check and recovery.

The same service capabilities are available through gRPC and an authenticated Streamable HTTP MCP server at `/mcp`. The [transport parity contract](transport-parity.md) lists all 19 verbs, shared schemas, approvals and event delivery; the OpenAPI document also describes `POST /v1/operations/{verb}` and `/mcp`. Set optional `grpc_listen` to a separate address such as `127.0.0.1:9443`; an omitted field leaves it disabled. Existing REST payloads and probe routes remain available.

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

Each engine TLS directory contains `ca.crt`, `tls.crt` and `tls.key`. The adapters run over stdio and connect to remote gRPC with mutual TLS. Both engines are mandatory configuration entries. The service does not load console `mcp.json` or plugin packages, reads a console `settings.json` only through `settings_file`, and has no work-mode API. `/v1/sessions` starts normal sessions. Install engine policies/definitions through their operator channels; AXLR's service config does not bootstrap them.

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
5. Read the result from `GET /v1/sessions/{id}` (`result`), or from the payload of the event that ended the turn (`approval.required`, `turn.completed`, `turn.interrupted`, `operation.failed`): the [runtime contract](runtime-contract.md)'s object, with the status, the answer, the pending call, every call with what became of it, and the usage and cost. It is null until a turn ends.

`GET /v1/tools` lists exact local and connected MCP identities and schemas. `POST /v1/tool-calls` creates a durable direct-call intent. Its `POST /v1/tool-calls/{id}/decisions` endpoint approves or denies that one call. Effectful calls persist a checkpoint before execution. An `uncertain` result after an interrupted effectful call requires operator inspection; AXLR does not retry it automatically or expose a reconciliation endpoint. The shipped service uses manual approvals for local and engine tools. There is no separate continue endpoint: interrupted pending calls can be decided through their approval route; an interrupted session with no pending calls can accept a new prompt.

The state directory contains session snapshots, event journals, call arguments and audit records. On Windows, service startup replaces inherited ACLs on that directory and its existing contents with access for the current account, SYSTEM and Administrators; it refuses reparse points. Use a dedicated directory owned by the service account.

`/livez` and `/readyz` are available only on the separate loopback probe listener. Readiness checks a configured nonempty model key and discovery through both KMP and MADE MCP adapters. It does not verify account credit, make a model completion or prove that an engine write/ceremony action is authorized. Missing or empty model credentials stop startup; engine transport failures can leave the API alive with failing readiness. `axlr-serve --probe=livez` and `--probe=readyz` always query loopback port 8081, matching the chart; query the configured endpoint directly if you use another probe port.

## The console's loop in the service

Since 23 October 2026 the service composes its turns with the console's loop (`tui/compose`): the context cap and its successor sessions, the session's MADE instance and KMP context, Jev, forged and catalog tools, the memory tools, the cost cap and the exec sandbox. A client such as a bot needs these facts:

- **Result.** The session and the event that ends a turn carry the execution's result, the same object `axlr-tui --exec --json` writes last ([runtime contract](runtime-contract.md)); a bot reads it instead of the transcript.
- **Policy.** `CreateSession` takes `policy`, the [runtime contract](runtime-contract.md)'s policy per execution: an autonomy level and the tools the session may and may not use. The session keeps it, `GetSession` shows it, and a call it forbids is refused outside the model before any `approval.required`, with status `forbidden` in the result; a tool it forbids is not offered to the model. An invalid policy is `422 invalid_policy`.
- **Budget.** `CreateSession` takes `budget`, the contract's budget per execution (`seconds`, `requests`, `tool_calls`, `usd`), enforced outside the model; `GetSession` shows it. The seconds count from the session's creation. At the first limit reached the turn ends in `turn.interrupted` whose result says `budget_exceeded` with the limit in `error` (`budget: requests 20 reached`); no `operation.failed` follows, the session stays resumable, and a call the `tool_calls` limit refused stays pending, where a decision under the same budget refuses it again. An invalid budget is `422 invalid_budget`. The budget follows the session into the successor the context cap opens.
- **Artifacts.** A turn's `axlr_deliver` calls hand over deliverables: a workspace file by path, which runs without an approver as a read, or inline content written to `.axlr/deliverables/<session>/<name>`, which waits for an `approver` as a write does. Each is journaled as an `artifact` event after its `tool.completed` and listed in the result's `artifacts` with its size, sha256 and workspace-relative `reference` ([the contract](runtime-contract.md#artifacts)).
- **Sandbox.** `CreateSession` takes `sandbox`, the contract's [sandbox per execution](runtime-contract.md#sandbox-per-execution) (`profile` `none`, `process` or `container`; `egress` `none`, `allowlist` with its `allow` list, or `full`; `writable`, `readable`, `container`), which confines the session's `local_exec` and forged tools; `GetSession` shows it with its egress spelled out. The service checks it on this host before creating the session: one it cannot give (bwrap or the container runtime missing, user namespaces refused, the image not on the host) is `422 sandbox_unavailable`, never a weaker one, and an invalid one `422 invalid_sandbox`. Under `exec_sandbox.mode: required` in the service's settings the profile cannot be `none`. The model requests and MCP servers stay outside it. The sandbox follows the session into the successor the context cap opens.
- **Receipt.** `GetSession` shows `receipt` (null until a turn ends): every model request of the session with its model, provider, usage and cost, every call with the sha256 of its arguments and outcome and how long it ran, and the policy, budget, sandbox and correlation, read from a journal written as each request and call starts and ends, so it survives a restart; a request the process died in is listed `interrupted` ([the contract](runtime-contract.md#receipt)).
- **Correlation.** `CreateSession` takes `correlation`, the contract's ids per execution (`bot_id`, `task_id`, `execution_id`, `trace_id`); `GetSession` shows it, every event the service journals for the session carries it in its payload as `correlation`, the result repeats it, and the session's MADE record and KMP entries are labelled with it. An invalid correlation is `422 invalid_correlation`.
- **Approvals.** Every tool call a model makes in a service session stops at `approval.required`, except the ones the console approves by itself: reads of the workspace (`local_read`, `local_search`) and its own read-only tools, which go from `tool.requested` to `tool.completed`. The service's policy is manual for every other tool. `approval.required` is the last event of its operation; the turn waits until a principal with the `approver` role posts a decision. A bot that lets strangers prompt it must not approve on its own; give it a human approver or keep the session's tools to reads.
- **Context cap.** Past the cap (`prompt_tokens`, 75,000 by default) the turn stops, the session's instance closes and the service opens a successor owned by the same principal. The capped session's journal gets `session.continued` with `successor_id` and `operation_id`; the successor's first turn starts by itself with the capped prompt. Follow the successor's events and read its revision before the next turn: it starts at revision 2, because the successor is saved once to create it and once to set its owner.
- **CloseSession.** `POST /v1/sessions/{id}/close` with `expected_revision` (verb `CloseSession`, MCP `axlr_close_session`) ends the session's MADE instance and journals `session.closed`; the transcript stays and a later turn continues in the instance's successor. It refuses a running turn with `409 operation_in_progress`. The service closes every open instance when it stops and, at start, ends as abandoned those a crash left open.
- **`settings_file`.** An optional absolute path to a console `settings.json`. The service honours `local_models`, `models`, `prompt_tokens`/`context_tokens`, `jev`, `forged_tools`, `catalogs`, `max_session_usd`, `turn_tool_calls`, `ceremony_profile`, `reviewer_model` and `exec_sandbox`. A file the configuration names must exist.
- **Local commands.** `local_exec` and the forged tools that run through it get the console's environment ([`tui/compose/exec_environment.go`](../tui/compose/exec_environment.go)): the absolute folders of the service's own `PATH`, or `/usr/local/bin:/usr/bin:/bin` when it names none (on Windows the system folders it lacks are added, with `SystemRoot` and `USERPROFILE`), and an absolute `HOME`. Nothing else reaches a command: not the model key, not a `local_models` `api_key_env`, not Jev's key. There is no configuration key for this path; to change what commands find, change the service's own `PATH` (a systemd unit's `Environment=PATH=…`, the container's environment). The published image is distroless, with no shell or coreutils, so a command there finds only the image's own binaries unless you build an image on it. Before 25 October 2026 the service gave its commands no `PATH`, and every `local_exec` failed `program "…" not found in configured PATH`.
- **Grok without OpenRouter.** A `local_models` entry reaches any OpenAI-compatible server over HTTPS with its key in an environment variable:

  ```json
  {"local_models":[{"id":"xai/grok","url":"https://api.x.ai/v1","model":"grok-4","api_key_env":"XAI_API_KEY","context_tokens":131072}]}
  ```

  Create sessions with `{"model":"xai/grok"}`. Through OpenRouter, an `x-ai/…` id works without settings. `model_api_key_file` is still required either way.
- **MADE.** The session instance needs `axlr_session` published in MADE and the `axlr` actor granted to claim it; otherwise the service says so at start and sessions run without an instance.
