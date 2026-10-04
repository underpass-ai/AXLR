# Runbook: operate the HTTP service

The service runs one workspace with mTLS clients and remote KMP/MADE adapters. Use [API configuration](../api.md) and [Helm deployment](../helm.md) for installation. The service does not load console settings, packages or driven work modes.

## Verify readiness

1. Validate absolute config paths, workspace access, dedicated state directory, API certificate/CA, private principal mapping and model key file. KMP and MADE need their own remote endpoint, server identity, client certificate/key and executable adapter.
2. Start `axlr-serve --config=/absolute/path/service.json`, or inspect the single Helm replica's rollout and logs.
3. Run the native probes inside the host/container:

```bash
axlr-serve --probe=livez
axlr-serve --probe=readyz
```

These flags use `http://127.0.0.1:8081` regardless of the config file. With another `probe_listen` port, query that configured loopback endpoint directly. A probe exits zero only for HTTP 204.

`livez` proves that the process responds. `readyz` checks a configured nonempty model key and tool discovery through both engine adapters; it does not send a paid model request, validate account credit, exercise memory writes or prove ceremony grants. Missing/empty credentials stop startup; engine connection failures can leave a live but unready process. A certificate that chains to the API CA also needs an authorized fingerprint/role mapping.

## Verify one direct read without a model call

Use a client certificate with `tool_operator` and `approver` roles for this controlled smoke check, or separate operator/approver clients. Set these shell variables to existing files and your API address:

```bash
axlr_api=https://localhost:8443
axlr_ca=/absolute/path/to/api-ca.crt
axlr_cert=/absolute/path/to/client.crt
axlr_key=/absolute/path/to/client.key
curl --fail-with-body --cacert "$axlr_ca" --cert "$axlr_cert" --key "$axlr_key" \
  "$axlr_api/v1/tools?origin=local"
```

Find the exact read tool name/schema in that response. Create a direct call with a workspace-relative file that exists, using a new 16–128 character idempotency key:

```bash
curl --fail-with-body --cacert "$axlr_ca" --cert "$axlr_cert" --key "$axlr_key" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: read-smoke-20261004-0001' \
  --data '{"tool":"local_read","arguments":{"path":"README.md","max_bytes":1024}}' \
  "$axlr_api/v1/tool-calls"
```

Replace the tool name if discovery differs. Save the returned `call_id` and `revision`. Inspect `GET /v1/tool-calls/{call_id}`, then submit `POST /v1/tool-calls/{call_id}/decisions` with `{"decision":"approve","expected_revision":N}` and a distinct idempotency key. Poll the call until terminal and inspect its result. Creating the intent alone does not prove the read ran. Use a fresh key for a new logical smoke check; retry a lost response with the original key and identical body.

## Run and reconnect a session

Create a session and turn using [the API flow](../api.md), then read SSE with `Accept: text/event-stream`. Preserve the session ID, operation ID and last event sequence. Reconnect with `?after=N` or `Last-Event-ID: N`; if both are supplied, `after` takes precedence. A cursor beyond the journal returns `400 invalid_cursor`. The implementation has no retention-expiry/410 recovery flow.

An SSE disconnect does not cancel the turn. Read the current session before approving its first pending call or cancelling, and use the latest revision. On `409 stale_revision`, reload and reconsider the decision. Do not reuse an idempotency key with changed content; that returns `409 idempotency_conflict`. The API has no separate continue endpoint: pending interrupted calls can receive exact decisions, while an interrupted session with no pending calls can take a new prompt.

## Reconcile after interruption

Read the session/direct-call resource and its event or audit record first. A direct effectful call interrupted after its execution checkpoint can become `uncertain`. AXLR does not replay it automatically and offers no API to mark it reconciled. An authorized operator must inspect the target, preserve the receipt and decide whether a new operation is necessary. A fresh idempotency key is a new operation, not recovery of the old one.

For a session, inspect any interrupted result before deciding a still-pending call. Restart, certificate rotation or reconnect is not evidence that an external effect failed. Keep state, journals and idempotency together when restoring; see [backups](recovery.md#back-up-and-restore).

## Deployment exit criteria

One intended replica is live and ready; an unmapped/no-certificate client is rejected; an authorized direct read completes under review; event replay preserves sequence; state survives a restart without automatic effect replay. Report which checks were exercised. A local fake-server test does not establish the health of a real deployment.
