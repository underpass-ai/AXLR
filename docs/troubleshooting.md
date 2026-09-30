# Troubleshooting

Start with the visible symptom, then use the relevant check. AXLR prints startup errors to stderr; the console also prints its private diagnostics path on launch.

| Symptom | Check | Next action |
|:--|:--|:--|
| `axlr-tui` rejects startup | Run `/tmp/axlr-tui --help` and check `OPENROUTER_API_KEY`, `--root` and `--lang` | Supply a key through the host environment, use an existing workspace and `en` or `es` |
| No model is selected | Open `/model` or pass `--model provider/model` | Choose a tool-capable text model available to the OpenRouter account |
| Model catalog fails | Inspect the error and use its Retry action | Check connectivity, account access and key; `--model` can bypass catalog lookup for a known model |
| `/plugin` is unavailable | Check `codex` on `PATH` | Install or expose the Codex CLI; the AXLR MCP list remains separate |
| A server is unavailable in `/mcp` | Inspect its manifest path, executable or URL and the configured environment | Repair the server, then press `R` to refresh; installing a Codex package alone does not connect it |
| MCP configuration is rejected | Check that `mcp.json` is regular, private (`0600`), valid version 1 JSON and under 64 KiB | Correct the file and restart the console |
| An MCP call asks for approval | Inspect the target, arguments and server policy | Approve once, or deliberately set that persisted server to `auto` in `/mcp` |
| A restored session is interrupted | Confirm the workspace matches the saved session | Press `Ctrl+R` to continue; AXLR does not run pending tools merely by loading it |
| Session is locked | Another console may have the same session open | Close the other writer or choose another session; do not delete the lock while it is active |
| No transcript text appears while waiting | Open the activity view with `Tab` | Reasoning, tool preparation and network waits have separate indicators; traces separate these timings |
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
