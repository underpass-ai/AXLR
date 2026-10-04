# AXLR agent console: design

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../../index.md) and its audited revision.

## Intent and scope

AXLR needs a capable terminal console for an agent working in a local workspace. The console must support a real OpenRouter conversation, incremental output, AXLR's local tools and explicitly registered MCP tools, deliberate approval of every tool call, and sessions that can be resumed. It should remain usable with a keyboard alone while offering mouse controls. This is Underpass's own AXLR runtime and interface.

The console is a separate `tui/` Go module in this repository. Its executable is `axlr-tui`; the existing one-request `cmd/axlr` JSON worker and its protocol retain their current behavior. The first release targets the existing trusted-local Linux profile. It is a terminal application for one human user and one active agent session at a time. Saved sessions may be reopened, but concurrent processes must not write the same session.

## Chosen approach and alternatives

Build a full agent console backed directly by AXLR's Go library. The TUI owns conversation orchestration, approval, rendering and persistence through hexagonal boundaries. This provides a useful end-to-end experience without making the one-request worker a session host.

A tool-only dashboard would have less state but would not meet the conversational goal. Routing the console through repeated worker subprocesses would duplicate session ownership, complicate streaming and plugin lifetimes, and hide the Go library's existing typed contracts. A multi-workspace IDE with several concurrent agent sessions would increase navigation and storage complexity before the single-session flow is sound.

## Module boundaries

`tui/go.mod` declares `github.com/underpass-ai/AXLR/tui` and depends on the root AXLR module. A repository `go.work` includes both modules for local builds. The root module does not gain terminal dependencies. CI explicitly checks both modules because `go test ./...` in the root module does not serve as a TUI test gate. No Go source lives at the repository root.

Within `tui/`:

- `domain/` owns typed session identities, session/turn state, pending tool decisions and activity records. It reuses AXLR's existing model, message and tool value objects where applicable and adds TUI-specific value objects for session and workspace identity. Raw UI strings and provider DTOs do not become domain types.
- `application/` owns use cases for starting/continuing a turn, resolving a tool decision, searching a transcript and saving/loading sessions. Its port interfaces describe model streaming, tool discovery/execution and session storage. It imports neither Bubble Tea nor provider/MCP wire types.
- `adapters/axlr/` maps those ports to AXLR's model use case, `runtime.Executor` and `plugins.Manager`. Local tool definitions and plugin tool definitions are built here. A catalog maps each advertised model tool name to one exact local operation or `(plugin ID, tool name)` pair; execution looks up that catalog rather than trusting a parsed name from the model.
- `adapters/storage/` stores versioned session snapshots under the user's XDG state directory with owner-only directory and file permissions. A temporary file plus rename prevents a partial snapshot from replacing the last valid one; an exclusive session lock rejects a second writer. The API key is never written. A session contains prompts and tool results, so the UI and documentation identify it as local user data.
- `dto/` holds the session file schema and explicit mappers between storage records and the TUI domain.
- `adapters/terminal/` holds the Bubble Tea v2 root model and reusable visual components. It turns component intents into application commands and maps application events back to view state. `cmd/axlr-tui/` composes adapters, validates CLI options and owns shutdown.

Each Go file has one primary type where a type is needed. Components remain small enough to understand and test independently. Dependency direction is from adapters to application/domain; no application use case calls terminal code.

## Model streaming and agent loop

The root AXLR module gains a separate streaming model port/use case while preserving `ModelPort.Complete` and the existing non-streaming API. OpenRouter's streaming adapter requests `stream: true`, parses complete SSE events across arbitrary network chunks, emits validated text deltas, assembles tool-call fragments by index, and returns one validated final `domain.CompletionResult`. Partial tool arguments are never executable. The stream observes context cancellation, caps each SSE event at 1 MiB and the aggregate response at 8 MiB, handles provider errors and incomplete streams, and does not retry automatically. The API key stays in the OpenRouter adapter, supplied by `cmd/axlr-tui` from `OPENROUTER_API_KEY`.

For each user turn, the TUI application snapshots the currently allowed local and MCP tools and sends the conversation plus their definitions to the model. Text deltas update the transcript immediately. After a complete model response, the application appends its assistant message, including all tool calls, to history. Each requested call is matched against the catalog, validated and queued for explicit human approval. Unknown names, malformed arguments or unavailable tools are rejected without execution. Approvals are per call, never remembered as a blanket grant. Denials are recorded as tool-result messages. Approved calls execute through AXLR's library one at a time in model order; each result is recorded under the original call ID. When every call from that response has a result or denial, the application sends the expanded conversation and the same tool definitions to the model for the next completion. A turn ends on a response without tool calls or when the user cancels. The application permits at most 32 tool calls per user turn; reaching the limit pauses the turn with an explicit error and no further calls.

The application persists after stable transitions: user submission, completed assistant message, each tool decision/result and final turn state. Streaming text is display state until a complete assistant message exists; if a stream ends early, the partial text is saved as an interrupted draft, not inserted as a valid assistant message. Cancelling while calls are queued records a cancellation result for every unexecuted call; an in-flight call records an uncertain outcome if its effect cannot be ruled out. Reopened sessions show pending approvals as interrupted; they cannot execute on load. Before a new model request, every preceding assistant call must have a corresponding result or explicit interruption result. Resuming requires an explicit new user action.

## Terminal experience and component composition

The root view composes reusable `Header`, `Transcript`, `Composer`, `ToolActivity`, `StatusBar`, `ApprovalDialog`, `SearchBox` and `ActionPalette` components. Components receive immutable view data and emit intents. They do not own OpenRouter, filesystem or MCP clients. The transcript keeps messages, tool requests, decisions, results and errors adjacent. The activity panel shows current operation and token usage when supplied by the provider. The message editor is multiline; sending and adding a newline use distinct documented keys. Search is scoped to the current session; the palette offers session switching, help, search, model/workspace information and cancel. A saved session picker makes recovery available from the UI.

At wider widths, activity sits beside the transcript; at narrow widths it becomes a tab. Very small terminals show a concise resize message rather than clipping approval controls. Width and height changes recalculate component sizes without losing composer text or scroll position. All controls have keyboard access; mouse clicks are conveniences. A visible help overlay lists the actual bindings. Colors and emphasis have readable monochrome fallbacks, and status is conveyed in text as well as color.

The terminal adapter uses `charm.land/bubbletea/v2`, `charm.land/bubbles/v2` for focused input/viewport components, `charm.land/lipgloss/v2` for styling and `github.com/lrstanley/bubblezone/v2` for clickable zones. BubbleZone's `Mark` is applied to controls and `Scan` only to the composed root view, with alt screen and mouse mode enabled. Layout uses ordinary Lip Gloss joins and placement, not its v2 canvas/compositor, because BubbleZone's maintainer warns that combination may not work. Every BubbleZone action has an equivalent key binding.

## Failures, cancellation and trust

Network and tool work runs outside Bubble Tea's render path; terminal updates arrive as messages. Provider authentication, credits, rate limits, malformed streams and tool errors have distinct visible states. Cancellation closes the active model stream or tool context and preserves the last stable session snapshot. A tool timeout or lost response is shown as outcome unknown if effects may have occurred; AXLR does not silently retry it. MCP `is_error` is displayed as a tool result, while transport/protocol failures are displayed as operation failures. The requested tool name, arguments and exact target are visible before approval. Plugin descriptions and model output are treated as untrusted data, not instructions or authority. Rendering sanitizes terminal control sequences in external text.

`cmd/axlr-tui` requires a workspace root and model ID, reads `OPENROUTER_API_KEY` at startup, and accepts explicit plugin manifests. It does not discover plugins by scanning a directory. Plugin process environments are supplied explicitly and never inherit the whole TUI environment by default. The application closes OpenRouter streams, plugin sessions, executor and storage handles on exit. The worker's flags and JSON output remain unchanged.

## Verification and acceptance

- Root tests verify streaming text and parallel tool-call assembly across split SSE frames, completion validation, provider errors, cancellation, size limits, no retry and preservation of the existing non-streaming API.
- TUI application tests use fake model, tool and storage ports to cover multi-step turns, multiple calls, approval/denial, unknown names, cancellation, tool-call limits, interruption and resume. They assert no tool executes before explicit approval or merely because a session was loaded, and that every continued model request contains a result for each preceding tool call.
- Storage tests cover versioning, round-trip, partial-file recovery, owner-only permissions and omission of the API key. Terminal tests cover component intents, keyboard and mouse parity, composition at wide/narrow/tiny sizes, resize, search and unreadable control-sequence input. No test calls live OpenRouter or launches a real user terminal.
- CI keeps one short Go workflow with cached dependencies. It runs formatting, `go vet`, race tests and aggregate coverage above 80% separately for root and `tui/`, then builds both executables. The root worker's current tests and contract remain a required gate.

References: [OpenRouter chat completions](https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request), [OpenRouter tool calling](https://openrouter.ai/docs/guides/features/tool-calling), [Bubble Tea v2 upgrade guide](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md), [BubbleZone v2 guidance](https://github.com/lrstanley/bubblezone).
