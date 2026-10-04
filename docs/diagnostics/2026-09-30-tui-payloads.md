# TUI timings and payload capture — 2026-09-30

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../index.md) and its audited revision.

## Existing-session audit

The running process had started without `--trace-file`. Its saved session contained 92 messages, 18 user turns and 31 tool results with execution timestamps. User submission, model request, first-text and response-completion timestamps were not saved; these cannot be recovered retrospectively.

The tool executions totaled 4,391.068 ms, with a maximum of 770.864 ms. In one uninterrupted user turn, 66,204.167 ms passed between one KMP tool finishing and the next beginning; that next tool ran for 298.629 ms. The gap includes model generation, host checkpoints and any approval wait. It is not an exact provider latency measurement.

A capture-only request reconstruction using the current mappers and saved 51-tool catalog produced 43 requests. Tool definitions occupy 165,258 bytes per reconstructed request; the first request is 168,774 bytes and the last is 454,433 bytes. These are reconstructed JSON sizes, not observed network traffic. Conversation payloads and the detailed report remain in private local storage.

## One live generation probe

A copy of that context was used for one real OpenRouter generation with a short response-only prompt. The session store was inert and no tool executor was supplied, so the probe could not execute MCP tools. The original snapshot checksum remained unchanged. The model returned two visible bytes and no tool calls.

| Measurement | Value |
| --- | ---: |
| Outgoing request | 456,321 bytes |
| Tool definitions JSON | 165,168 bytes |
| HTTP 200 headers | 10.271 s |
| First SSE frame / reasoning | 10.401 s |
| First visible text | 12.139 s |
| Text callback | 12.143 s |
| Use case completion | 12.144 s |
| Response SSE | 6,848 bytes |
| Provider-reported input / cached tokens | 125,102 / 0 |
| Provider-reported output / reasoning tokens | 41 / 40 |

The wait exists without the TUI renderer. This single probe does not distinguish network latency, provider queueing or context processing before headers. The context volume and absence of cached input are measured facts; their causal contribution needs a controlled comparison.

## Instrumentation and verification

Every valid launch now creates a private JSONL trace without an extra flag. Numeric events distinguish input submission, request size, headers, received bytes, SSE frames, reasoning, tool-argument deltas, visible text, tool execution, session checkpoints and rendering. Request IDs correlate provider stages with private redacted request JSON and response SSE captures. Explicit trace paths, including relative paths, are supported; each launch receives a unique payload directory even if the trace is appended.

Captures exclude authorization headers and redact the configured API key and recognizable credentials. Files are 0600, directories 0700, with limits of 8 MiB per file without a per-launch budget that disables later captures. Capture failures are recorded; HTTP error bodies are captured on close with a bounded 500 ms drain and explicit partial/failure markers. `--trace-payloads=false` keeps numeric telemetry without capturing bodies.

The final four-turn PTY test recorded 405 actions with matching starts/ends; maximum render time was 0.849 ms and context assembly 0.090 ms. The four-turn PTY test used a controlled TLS/SSE provider and real configured MCP clients. It verified KMP and MADE automatic approval, unknown-tool recovery, reasoning and keepalive telemetry, 14 body captures, late-delta handling, transcript scrolling and terminal resizing. It queried MADE capabilities; it did not execute a live ceremony. The policy applies to every allowed tool of the configured exact plugin ID, including ceremony calls.

`go -C tui vet ./...` and the complete race suite passed. Aggregate TUI coverage is 88.8%. The test suite also covers CR, LF and split CRLF streams, repeated closes, unavailable/oversized captures, reused trace paths and credential redaction.

## Controlled comparison: first guide vs baseline

Two further requests used the same model, exactly equal 51-tool definitions and host instruction, and the same short response-only prompt. The first kept the initial user/assistant exchange; the second additionally kept the first guide call and its response. Neither could execute tools.

| Context | Request bytes | Input tokens | First text |
| --- | ---: | ---: | ---: |
| Initial exchange | 169,106 | 43,244 | 5.805 s |
| After first guide | 172,308 | 44,056 | 4.876 s |

The first guide adds 3,202 bytes and 812 tokens; this comparison did not reproduce a slowdown from that guide alone. The initial catalog occupies 165,168 bytes, or 97.67% of the baseline request. The long-context probe has 125,102 input tokens and 12.139 s to first text. These are single sequential samples; backend variation prevents a causal latency conclusion.

## Model invocation during MCP work

`StartTurnUseCase` discovers the current tool catalog and freezes it for the turn. `AgentTurnUseCase` calls `ContinueTurnUseCase` for one model stream. The model produces tool requests; the host resolves the pending calls under their plugin policy, saves their results, then opens the next model stream. Multiple pending calls are resolved before the next model request. The local MCP tool itself does not invoke OpenRouter.

Each continuation constructs the host system instruction plus the entire saved message history and the complete current tool catalog. Tool messages contain AXLR's serialized execution response, including MCP content and structured content; the full history is sent again on later turns. There is currently no context compaction or deferred tool-schema loading. This is an AXLR context-management cost, independent of the observed short KMP execution times.

Operational spans cover context assembly, tool discovery and each plugin's discovery, tool resolution and execution, model generation, HTTP exchange, bounded error-body reading and payload capture, session saves/loads/listing, UI updates, event delivery, agent operations, model catalog, plugin policy changes and rendering. The JSONL records carry microsecond durations and run/span/parent/request IDs. Wire arrival events remain separate from visible text and model completion.
