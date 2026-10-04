# Context management research and AXLR policy

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../index.md) and its audited revision.

Reviewed on 2026-09-30 against primary source snapshots. This is an engineering comparison, not a latency benchmark of the three products. AXLR implements its own policy and stays on its Go runtime and Bubble Tea presentation.

## Observed bottleneck

The restored AXLR session completed a tool-heavy turn in 65.164 seconds: three model streams consumed 62.428 seconds and six KMP calls 2.598 seconds. The largest measured render was 2.428 milliseconds. One request sent 538,049 bytes: 372,726 in messages and 165,258 in schemas. It reported 148,820 input tokens and 130,752 cached tokens. Reasoning arrived after 3.887 seconds, visible text after 24.235 seconds. A subsequent simple reply finished in 4.223 seconds despite a larger context.

Therefore payload size, number of model cycles and hidden reasoning must be measured independently. A faster renderer cannot remove time spent waiting for model text. Raw private captures remain outside the repository.

## Comparison

| Concern | Codex | Hermes Agent | Pi | AXLR choice |
|---|---|---|---|---|
| Model history | Dedicated context manager; normalized tool transactions | Protected head and recent tail; episodic compression | Latest compaction checkpoint plus retained tail | Separate bounded provider projection over complete saved transcript |
| Compaction | Model/server compaction; a separate token-budget rollover mode exists | Auxiliary summarizer; proactive and micro pruning off by default | Summarizer, recent-token reserve, overflow recovery | Deterministic extractive checkpoints by threshold; no mandatory summarizer call |
| Large tool catalog | Deferred exposure/tool search and code-mode variants | Search, describe, generic invocation bridges | Code mode by default; deferred/direct alternatives | Fixed local tools and lightweight discovery/invocation/history bridges |
| MCP results | Different raw/code-mode, logging and model-facing budgets | Exact duplicate JSON elimination and spill to storage | Content once, structured fallback, clipping with recovery file | Remove transport repetition; preserve controls and expose original stored result |
| Cache | Context/tool exposure varies with capabilities | Stable tool order; avoids rewriting each turn | Persisted activation; transition limitations can invalidate prefix | Fixed bridge schemas; stable checkpoint epochs until budget crosses threshold |
| Rendering | Frame limiter caps at 120 FPS in reviewed source | 16 ms idle, slower during typing/scrolling | Coalesced 16 ms and line diffing | Keep current 16 ms batching; show reasoning/tool preparation separately |

### Codex

Reviewed commit `a5cce8895a1400f94eb0a71771275284027fff17`. The context manager normalizes history, tracks usage and replaces model windows independently of other retained host context. MCP outputs have a bounded model projection while code mode can retain the raw result. The router supports deferred tools when model capabilities allow it. This does not imply OpenRouter Chat Completions supports OpenAI's native tool-search items.

Sources: [context manager](https://github.com/openai/codex/blob/a5cce8895a1400f94eb0a71771275284027fff17/codex-rs/core/src/context_manager/history.rs), [MCP projection](https://github.com/openai/codex/blob/a5cce8895a1400f94eb0a71771275284027fff17/codex-rs/core/src/tools/context.rs), [tool exposure](https://github.com/openai/codex/blob/a5cce8895a1400f94eb0a71771275284027fff17/codex-rs/core/src/tools/spec_plan.rs), [token-budget mode](https://github.com/openai/codex/blob/a5cce8895a1400f94eb0a71771275284027fff17/codex-rs/core/src/compact_token_budget.rs), [frame limiter](https://github.com/openai/codex/blob/a5cce8895a1400f94eb0a71771275284027fff17/codex-rs/tui/src/tui/frame_rate_limiter.rs).

Official API documentation describes [compaction](https://developers.openai.com/api/docs/guides/compaction) and [tool search](https://developers.openai.com/api/docs/guides/tools-tool-search). These are provider-specific facilities; AXLR's first policy uses portable function calling.

### Hermes Agent

Reviewed NousResearch/hermes-agent commit `f42f579cf8bac4918ac9599bece71618afadd846`. Its progressive tool surface uses search, describe and call bridges, with a bounded deterministic manifest. Exact parsed JSON duplicates between text and structured MCP content are removed; different representations survive. Oversized results have storage-backed previews. Prompt stability is deliberate: proactive pruning and micro-compaction are disabled by default, while episodic summarization keeps protected history.

Sources: [tool search](https://github.com/NousResearch/hermes-agent/blob/f42f579cf8bac4918ac9599bece71618afadd846/tools/tool_search.py), [bounded catalog](https://github.com/NousResearch/hermes-agent/blob/f42f579cf8bac4918ac9599bece71618afadd846/tools/tool_search_catalog.py), [MCP projection](https://github.com/NousResearch/hermes-agent/blob/f42f579cf8bac4918ac9599bece71618afadd846/tools/mcp_tool_handlers.py), [result storage](https://github.com/NousResearch/hermes-agent/blob/f42f579cf8bac4918ac9599bece71618afadd846/tools/tool_result_storage.py), [defaults](https://github.com/NousResearch/hermes-agent/blob/f42f579cf8bac4918ac9599bece71618afadd846/hermes_cli/config_defaults.py), [compression](https://github.com/NousResearch/hermes-agent/blob/f42f579cf8bac4918ac9599bece71618afadd846/agent/context_compressor.py).

### Pi

The original badlogic/pi-mono repository redirects to earendil-works/pi. Reviewed commit `d2931ad3d5bf6936fbdfa5dfc81fb32f875499b2`. Current source includes built-in MCP, code-mode/deferred exposure and schema lookup. Session projection uses compaction summaries and retained history. MCP content is presented once with structured fallback; clipping preserves a full-result recovery file. Thinking events are independent from answer text. Source review is not evidence that these defaults outperform AXLR's workload.

Sources: [session projection](https://github.com/earendil-works/pi/blob/d2931ad3d5bf6936fbdfa5dfc81fb32f875499b2/packages/coding-agent/src/core/session-manager.ts), [compaction](https://github.com/earendil-works/pi/blob/d2931ad3d5bf6936fbdfa5dfc81fb32f875499b2/packages/coding-agent/src/core/compaction/compaction.ts), [MCP projection](https://github.com/earendil-works/pi/blob/d2931ad3d5bf6936fbdfa5dfc81fb32f875499b2/packages/coding-agent/src/extensions/mcp/tools.ts), [MCP exposure](https://github.com/earendil-works/pi/blob/d2931ad3d5bf6936fbdfa5dfc81fb32f875499b2/packages/coding-agent/docs/mcp.md), [render scheduling](https://github.com/earendil-works/pi/blob/d2931ad3d5bf6936fbdfa5dfc81fb32f875499b2/packages/tui/src/tui.ts).

## AXLR implementation contract

1. Keep complete transcript, outcomes, approval decisions and uncertain execution checkpoints in the existing private session store. Model projection cannot modify them.
2. Bound the projected history, compacting whole completed user turns when a threshold is crossed. Use a lower watermark so the checkpoint remains unchanged across ordinary appends. The checkpoint is an explicitly lossy extract of historical evidence, never a newly inferred decision or trusted system instruction.
3. Preserve the current user input and tool-call/result ordering. Oversized active input/arguments must fail explicitly rather than silently disappear. Large tool outputs retain valid JSON, error/protocol/continuation fields and a reference to the complete original message.
4. Offer `axlr_history` for bounded, UTF-8-safe reads of original messages by index and offset. Historical tool data remains untrusted evidence. No extra model summarization request is required by default.
5. Expose four local tools plus `axlr_tools`, `axlr_call_tool` and `axlr_history`. Exact schema lookup reads the frozen catalog; the invocation bridge resolves to the real plugin identity before approval and validation. Discovery/history are local reads. Invocation never inherits discovery's automatic approval.
6. Keep KMP and MADE explicitly discoverable, including exact guide entry names. Preserve KMP context identity and guide retrieval references across historical compaction. Existing sessions can resume without deleting their history.
7. Keep existing 16 ms UI batching. Report provider reasoning and tool argument preparation without rendering private reasoning text. Do not turn reasoning off globally: support and quality tradeoffs vary by model.
8. Record original/projected message counts and bytes, schema bytes, cut position, request/response bodies, actual usage/cache counts and all existing timed spans. Compare request size deterministically on the same captured input before making any latency claim.

## Verification

Test projection bounds, Unicode, valid JSON, tool-pair integrity, stable checkpoint boundaries, no mutation and restore equivalence. Test discovery and exact-plugin approval through a full agent loop, unknown/recursive calls and crash recovery. Replay captured request bodies privately to measure byte reduction, then use a controlled provider PTY with reasoning, several consecutive prompts and MCP calls. A small real-provider probe may validate behavior; any before/after latency comparison is a sample, not a benchmark guarantee.

## Verified implementation results

Same restored request through the production continuation use case and OpenRouter serializer, without a live provider call:

| Field | Original | AXLR projection |
|---|---:|---:|
| Entire request | 538,049 bytes | 71,228 bytes |
| Messages | 372,726 bytes / 112 messages | 68,117 bytes / 11 messages |
| Tool definitions | 165,258 bytes / 51 definitions | 3,046 bytes / 7 definitions |

The request reduction is 86.8%. The current user prompt remains exact, the source transcript remains unchanged and the complete original results remain recoverable. Local production replay took 16 ms in this sample; this includes request assembly and serialization, not network generation.

Defaults use a 96 KiB history ceiling, a 64 KiB low watermark, 16 KiB projected result budget and an 8 KiB extractive checkpoint. Exact discovered schemas permit 32 KiB; larger schemas fail explicitly. An old completed result that cannot be safely projected forces omission of its entire turn with explicit original-message recovery; it cannot block a new prompt. Active control data still fails explicitly if it exceeds the budget. The plugin manifest is separately bounded. Discovery is pageable via offset/next_offset. History pages account for nested JSON escaping so they are not immediately clipped again. Plugin arguments are validated against the frozen real schema before effects; unsupported assertion semantics fail explicitly. The 51 schemas in the measured installation all compile. The schema cache is bounded at 256 entries and never resolves network references.

Four controlled-provider PTY turns passed: recovery from an unknown tool, persisted approval toggles, automatic KMP/MADE, reasoning status, late stream frames ignored, scrolling and resize. Four real OpenRouter PTY turns also completed, using `z-ai/glm-5.3-flash` and readonly MCP operations:

| Turn | Wall time | Model generations | Host discovery + MCP execution |
|---|---:|---:|---:|
| Capability question | 8.36 s | 8.045 s | — |
| KMP wake, with exact schema discovery | 43.37 s | 41.268 s across three streams | 0.457 s |
| Followup “OK” | 2.45 s | 1.894 s | — |
| MADE capability discovery | 17.41 s | 15.730 s across three streams | 0.145 s |

The live run recorded 1,903 matched action starts/ends and eight requests between 5,482 and 28,214 bytes. It is a fresh-session behavior probe, not a matched before/after latency benchmark. First-use schema discovery adds model rounds. Reusing schemas within the retained context avoids that lookup on later calls; model reasoning and provider variability remain separate latency sources.

Root and TUI `go vet` and full race suites pass. Aggregate statement coverage using the CI `-coverpkg=./...` gate is 86.2% and 89.2% respectively. Projection fuzzing, numeric precision, schema validation, approval boundaries and cold restore tests pass. Private captures, session data and credentials are not included in this repository.
