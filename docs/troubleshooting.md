# Troubleshooting

Start with the visible symptom, then use the relevant check. AXLR prints startup errors to stderr; the console also prints its private diagnostics path on launch.

| Symptom | Check | Next action |
|:--|:--|:--|
| `axlr-tui` rejects startup | Run `/tmp/axlr-tui --help` and check `OPENROUTER_API_KEY`, `--root`, `--lang` and `local_models` in `settings.json` | Supply a key through the host environment or configure a local model, use an existing workspace and `en` or `es` |
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
| The agent's `axlr_request_repair` is refused | Read the reason in the tool result: project failure, arguments, permission, provider, isolated error, duplicate, attempts or a repair session | Resolve external causes directly; retry the operation once so the failure recurs; a merged repair for this build needs an updated console, not another request |
| A self-repair shows `interrupted` or `blocked` on `/repair` | Read the record: step, pull request, clone, MADE instance and KMP report | `r` recovers an interrupted repair from what MADE and the forge recorded; a blocked one keeps its clone and pull request for a hand-made follow-up |
| A self-repair merged but the same failure continues | Check the build in the footer note and in `axlr_repair_status` | The running console does not contain the fix; update or rebuild AXLR and restart it |
| KMP returns unrelated context or no guide | Check the selected store and matching guide assets | Follow [KMP verification](runbooks/kmp.md#verify-guide-and-store-selection) before writing |
| A restored session is interrupted | Confirm the workspace matches the saved session | Press `Ctrl+R` to continue; AXLR does not run pending tools merely by loading it |
| Session is locked | Another console may have the same session open | Close the other writer or choose another session; do not delete the lock while it is active |
| No transcript text appears while waiting | Open the activity view with `Tab` | Reasoning, tool preparation and network waits have separate indicators; traces separate these timings |
| Active model context still exceeds its budget | Check prompt, arguments and discovery schema sizes after automatic result compaction | Use smaller source pages and a new scoped turn; full history remains saved |
| HTTP service is live but not ready | Inspect both adapter endpoints, TLS identities and discovery | Use [service operations](runbooks/service.md); readiness does not make a model completion |
| API revision or idempotency conflict | Reload the current resource and compare the original request | A changed decision needs a new key and the current revision; reconnecting SSE does not require a new turn |
| Worker exits `2` | Protocol JSON was malformed, oversized or contained unknown fields | Compare the request with the [worker contract](worker.md) |
| Worker exits `0` but tool failed | Inspect response `status` and `error.code` | A valid protocol request can produce `failed`, `rejected` or `timed_out` |

## Local models

| Symptom | Check | Next action |
|:--|:--|:--|
| `local model …: … API key is required (set NAME)` | The URL is not a loopback address | Export the variable named by `api_key_env`, or point `url` at `127.0.0.1` |
| `model endpoint must use https unless it is a loopback address` | A non-loopback `url` uses `http` | Serve it over TLS, or reach it through a loopback tunnel |
| `model … is not a configured local model and OPENROUTER_API_KEY is not set` | The session's model is an OpenRouter id | Export the key, or choose a local model in `/model` |
| `model endpoint HOST invalid_request (HTTP 400)` | The server refused the request, often because its template does not declare tools | Run the smoke test below |
| The model answers in text, or repeats the same call | The server's tool-call parser does not match the model's template | Fix the server's parser or template; AXLR receives only what the server returns |
| The answer ends with raw markup such as `<|tool_call>call:local_write{…}` | The server's streaming parser leaked a tool call as text | Set `"stream": false` on that local model. A ceremony whose model does this twice is cancelled with the reason; a restored one with its step open can be stopped with `/stop-ceremony` |
| `… stream inactivity timeout` | A cold prefill took longer than `stream_idle_seconds` | Raise `stream_idle_seconds` or lower `context_tokens` |
| A long session is refused by the server for its context length | `context_tokens` is larger than the server's window | Set `context_tokens` at or below the server's `-c` or `--max-model-len` |

Smoke test for a server, which must return `tool_calls` with valid JSON, not text:

```bash
curl -s http://127.0.0.1:8080/v1/chat/completions -H 'content-type: application/json' -d '{"model":"MODEL","messages":[{"role":"user","content":"Read README.md"}],"tools":[{"type":"function","function":{"name":"local_read","description":"Read a workspace file.","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}]}'
```

## Jev

| Symptom | Check | Next action |
|:--|:--|:--|
| `jev: TYPESAFE_API_KEY is required for Jev` | `jev.tool` or `jev.final_check` is on | Export the key, or turn both switches off |
| `axlr_judge` returns `TypeSafe rejected the API key (HTTP 401)` | The key in the console's environment | Replace the key; nothing else changes |
| An `[AXLR · Jev]` message appears after an answer that was complete | Jev doubted it below `final_threshold` | The model can say why the answer is complete; lower `final_threshold` or turn `final_check` off if it happens often |
| The model asks `axlr_judge` an "A or B" question and gets only `yes` | It omitted `options` | The tool's description asks for options; a model that ignores it is a model limit, not a Jev one |

## Plans

| Symptom | Check | Next action |
|:--|:--|:--|
| `/plan` cannot start | MADE is prepared with the seven definitions | Press `P` in `/mcp` again; it publishes `axlr_plan`, `axlr_task` and `axlr_sync` 1.0 |
| The plan comes back with defects three times and ends `BLOCKED` | Read the defects on the plan's last hand-back | Give a narrower brief, or let a larger model plan (`plan.model`) |
| A task ends `BLOCKED` with "the model did not use its tools" | The worker's transcript (the task's session id is on the plans panel) | Usually the server's tool-call parser: try `"stream": false` on the local model |
| A task ends `BLOCKED` asking for the person's approval | The worker called a tool the approval policy keeps under a card | Workers run without the person; allow that tool, or change the plan |
| A plan shows `interrupted` | The console stopped while it ran | Open `/plan` and press `r`; finished tasks are not run again |

## Inspect the right diagnostics

The console's JSONL trace reports timing and failure classes without prompt content. Its default payload directory includes redacted HTTP bodies and can include conversation content. If you only need timings, launch with `--trace-payloads=false`. Queued messages appear as `prompt_queued`; `steer_applied` records one joining the running turn and `steer_cancelled` a ceremony step stopped for one. Do not paste payload files into an issue without reviewing them.

The [MCP diagnosis](diagnostics/2026-09-30-tui-mcp.md) and [payload audit](diagnostics/2026-09-30-tui-payloads.md) are examples from a dated investigation, not current health checks.

For a reproducible code issue, run both module suites from a full checkout:

```bash
GOWORK=off go test ./...
GOWORK=off go -C tui test ./...
```

If a tool call was cancelled, timed out or lost its response, first inspect the target's state before retrying; the effect may have happened.
