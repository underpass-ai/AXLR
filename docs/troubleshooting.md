# Troubleshooting

Start with the visible symptom, then use the relevant check. AXLR prints startup errors to stderr; the console also prints its private diagnostics path on launch.

| Symptom | Check | Next action |
|:--|:--|:--|
| `axlr-tui` rejects startup | Run `/tmp/axlr-tui --help` and check `OPENROUTER_API_KEY`, `--root` and `--lang` | Supply a key through the host environment, use an existing workspace and `en` or `es` |
| No model is selected | Open `/model` or pass `--model provider/model` | Choose a tool-capable text model available to the OpenRouter account |
| Model catalog fails | Inspect the error and use its Retry action | Check connectivity, account access and key; `--model` can bypass catalog lookup for a known model |
| `/plugin` cannot add a source | Check that it is an absolute local path or an HTTPS Git URL containing a supported marketplace or package manifest | Fix the source and retry; AXLR stores packages in its own data directory |
| A server is unavailable in `/mcp` | Inspect its manifest path, executable or URL and the configured environment | Repair the server, then press `R` to refresh; a built-in catalogue entry alone does not connect it |
| MCP configuration is rejected | Check that `mcp.json` is regular, private (`0600`), valid version 1 JSON and under 64 KiB | Correct the file and restart the console |
| An MCP call asks for approval | Inspect the target, arguments and server policy | Approve once, or deliberately set that persisted server to `auto` in `/mcp` |
| Local write/edit is refused | Check the active mode | `/review` refuses changes; writer/research permit documents only. Change to `/normal` between turns if the task requires it |
| Exec asks despite autonomy, or stdin is refused | Review/writer/research require a decision for every process and reject nonempty stdin | Put reviewable arguments in the command; do not bypass the mode through another tool |
| `/debug` or `/delivery` cannot start | Verify exact `made` ID, work grant and matching 2.0 definition digest | Follow [MADE preparation](runbooks/made.md#prepare-the-driven-ceremonies); 1.0 skill definitions do not satisfy the driver |
| Ceremony completed but memory says `not recorded` | Inspect the connected `kmp` engine and exact session scope | Preserve MADE evidence, repair KMP and explicitly record the outcome if needed |
| KMP returns unrelated context or no guide | Check the selected store and matching guide assets | Follow [KMP verification](runbooks/kmp.md#verify-guide-and-store-selection) before writing |
| A restored session is interrupted | Confirm the workspace matches the saved session | Press `Ctrl+R` to continue; AXLR does not run pending tools merely by loading it |
| Session is locked | Another console may have the same session open | Close the other writer or choose another session; do not delete the lock while it is active |
| No transcript text appears while waiting | Open the activity view with `Tab` | Reasoning, tool preparation and network waits have separate indicators; traces separate these timings |
| Active model context still exceeds its budget | Check prompt, arguments and discovery schema sizes after automatic result compaction | Use smaller source pages and a new scoped turn; full history remains saved |
| HTTP service is live but not ready | Inspect both adapter endpoints, TLS identities and discovery | Use [service operations](runbooks/service.md); readiness does not make a model completion |
| API revision or idempotency conflict | Reload the current resource and compare the original request | A changed decision needs a new key and the current revision; reconnecting SSE does not require a new turn |
| Worker exits `2` | Protocol JSON was malformed, oversized or contained unknown fields | Compare the request with the [worker contract](worker.md) |
| Worker exits `0` but tool failed | Inspect response `status` and `error.code` | A valid protocol request can produce `failed`, `rejected` or `timed_out` |

## Inspect the right diagnostics

The console's JSONL trace reports timing and failure classes without prompt content. Its default payload directory includes redacted HTTP bodies and can include conversation content. If you only need timings, launch with `--trace-payloads=false`. Do not paste payload files into an issue without reviewing them.

The [MCP diagnosis](diagnostics/2026-09-30-tui-mcp.md) and [payload audit](diagnostics/2026-09-30-tui-payloads.md) are examples from a dated investigation, not current health checks.

For a reproducible code issue, run both module suites from a full checkout:

```bash
GOWORK=off go test ./...
GOWORK=off go -C tui test ./...
```

If a tool call was cancelled, timed out or lost its response, first inspect the target's state before retrying; the effect may have happened.
