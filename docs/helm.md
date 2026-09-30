# Helm deployment

The [AXLR chart](../charts/axlr/Chart.yaml) installs one AXLR service replica. KMP and MADE remain existing remote services; the pod contains only their MCP adapters. Start from [`values.example.yaml`](../charts/axlr/values.example.yaml), replace every placeholder, then run:

```bash
helm lint charts/axlr -f /absolute/path/values.yaml
helm template axlr charts/axlr -f /absolute/path/values.yaml
helm upgrade --install axlr charts/axlr -f /absolute/path/values.yaml
```

Provide an image digest (`sha256:...`), workspace PVC, and either an existing state PVC or `state.volumeClaimTemplate.size`. The state PVC persists AXLR sessions, event journals, direct calls and idempotency records. The chart rejects more than one replica and uses a `Recreate` deployment strategy. A generated state PVC has a Helm `keep` annotation so uninstall does not erase sessions. Back it up as a unit; back up KMP and MADE separately using their own operations guidance.

Create the Kubernetes Secrets before installation:

| Value | Required Secret keys |
|:--|:--|
| `api.serverTLS.existingSecret` | `tls.crt`, `tls.key` for the API DNS name |
| `api.clientCA.existingSecret` | `ca.crt` trusted for API client certificates |
| `api.principals.existingSecret` | `principals.json` with version 1 fingerprint mapping |
| `model.apiKey.existingSecret` | `api-key` for OpenRouter |
| `kmp.tls.existingSecret`, `made.tls.existingSecret` | Each has `ca.crt`, `tls.crt`, `tls.key` for remote gRPC mTLS |

Set `kmp.endpoint` and `made.endpoint` to `host:port` and their `tls.serverName` to the DNS identity on each engine certificate. The locked adapter binaries and SHA-256 values are in [`distribution/engines.lock.json`](../distribution/engines.lock.json). The chart does not generate a CA, credentials, a KMP/MADE store or a remote authorization policy. A failed engine connection keeps `readyz` failing. Rotate mounted Secrets through the cluster's normal rollout procedure so adapter processes reconnect; do not replay uncertain calls after rotation.

The Service is `ClusterIP`. The API itself verifies client certificates; any external ingress must preserve TLS through to the pod. The chart rejects `ingress.enabled: true` because a generic Ingress can terminate mTLS before AXLR. Optional NetworkPolicy requires operator-supplied `ingressFrom` and `egressTo` peers covering intended clients, OpenRouter, both engines and DNS. The container runs as UID/GID 65532 with a read-only root filesystem, private Secret mounts and no service-account token. The workspace PVC must be writable by that UID for local write/edit/exec tools; mounting a workspace path does not itself sandbox child processes.
