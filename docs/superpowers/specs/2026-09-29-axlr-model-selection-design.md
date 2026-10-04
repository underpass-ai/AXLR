# AXLR model selection design

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../../index.md) and its audited revision.

**Date:** 2026-09-29

**Status:** Proposed for user review

**Repository:** `github.com/underpass-ai/AXLR`

**Companion:** [AXLR agent console design](2026-09-29-axlr-tui-design.md)

## Intent

AXLR users can discover and select an OpenRouter model inside the console by typing `/model`. The command presents a searchable list of models that support tool calls. A selected model applies to subsequent model requests in the current session. Running `axlr-tui` without flags uses the current directory as the workspace and lets the user choose a model with `/model` before creating a session. There is no automatic picker on launch.

This extends AXLR's own runtime and TUI. The existing `cmd/axlr` JSON worker and its interface remain unchanged.

## User experience

### Starting the console

- `axlr-tui` resolves the current working directory as the workspace. It checks that the directory exists, that the OpenRouter key is present, and that explicit plugin options are valid. It opens the TUI without a model or persisted session.
- The unconfigured screen shows the workspace and a clear `Type /model to choose a model` instruction. It accepts `/model` and safe navigation/help actions; the existing action palette also offers `Models` so a drafted prompt can stay in the editor. Sending a normal prompt leaves the draft in the editor and explains that a model must be selected first; it never sends a request or creates a session.
- Typing `/model` and pressing Enter opens the model list. The existing Ctrl+S send key also recognizes an exact `/model` command. The command is control input, never a user message in model history. `--model ID` remains a direct, noninteractive startup path. `--root PATH` remains an override for the current-directory default.
- `--session ID` loads that session's persisted model without prompting. Its workspace must match the resolved root. An explicitly supplied `--model` must match the saved model at startup; `/model` can change it after loading, subject to the safety rules below.
- Quitting before selection leaves no new session snapshot or lock file. A failed catalog request likewise leaves no session and allows retry.

### The `/model` selector

- The command fetches the current OpenRouter catalog on demand. The list shows each model's display name and exact ID, with context length and prompt/completion rates in USD per million tokens when valid data is supplied. It is searchable by name, ID and provider. Keyboard navigation uses arrows, PgUp/PgDn, Enter to select and Esc to close; visible controls also work by mouse. Resizing preserves query and selection.
- The list includes only text-output models whose API metadata reports support for `tools`. It must not silently advertise models that cannot receive the agent's tool definitions. A model omitted by this filter remains usable through explicit `--model ID`; the provider is the final authority on request compatibility.
- Fetching shows a loading state. A failed request, malformed response or empty filtered catalog shows a clear error and a retry action. It preserves the current model, session and editor draft. The API key and response body are never rendered or persisted.
- The model ID selected from the catalog is validated with AXLR's `ModelID` value object before it becomes session state. Provider-supplied names and descriptions are terminal-sanitized before display.

### Changing the current session

- `/model` may select a new model when no model/tool operation is running and no tool call awaits approval. In domain terms, idle and complete sessions qualify, as does an interrupted session with no pending calls. A pending call continues to require an explicit decision; opening the picker or changing models cannot bypass it. If the console is busy, the command explains why it cannot run and preserves the editor text.
- Selection updates the current session through a use case that validates the model, saves the new state atomically, then publishes it to the TUI. A failed save keeps the old model active. No provider request occurs merely because a model was selected.
- The selected model is used for subsequent requests. Completed transcript messages remain unchanged. An interrupted turn with no pending calls can be switched only by an explicit `/model` action; if later resumed, its next model request uses the newly selected model. Already completed or uncertain tool effects are never retried.
- The header and model/workspace info view show the active model. Search, saved-session switching and per-call approval continue to operate as before.

## Architecture and data flow

1. A TUI application `ModelCatalogPort` returns typed `AvailableModel` values. A catalog use case validates, filters and orders the list for presentation. No terminal component performs HTTP calls.
2. An OpenRouter adapter calls [`GET /api/v1/models`](https://openrouter.ai/docs/api/api-reference/models/get-models) with bearer authentication and `supported_parameters=tools`. It uses the API's default text-output filter and validates text output in returned metadata. It bounds the response, honors cancellation and a timeout, and maps provider/transport failures to sanitized errors. It never logs the key or forwards it to plugins.
3. A reusable terminal `ModelPicker` owns only query, selection and rendering state. The root Bubble Tea model owns asynchronous catalog loading and routes typed selection intents. It uses the existing one-operation-at-a-time lifecycle so a stale response cannot overwrite a newer selection or session.
4. A `CreateSessionUseCase` creates and saves the first typed session after model selection. A `ChangeSessionModelUseCase` validates the transition and saves a copy before publishing it. The session aggregate owns the rule that model changes cannot occur while calls are pending or execution is active. Existing version-1 session snapshots remain loadable; their `model` field continues to represent the selected model.
5. CLI composition supplies the catalog port, workspace, store and existing OpenRouter/model/tool dependencies to the terminal adapter. Explicit `--model` and saved sessions keep their current startup behavior. The TUI can render an unconfigured startup state without constructing an invalid domain `Session` or writing placeholder model IDs.

The catalog request is a discovery operation, not a model completion. It does not send prompts, tools or session transcripts. Model selection is a local state change, not a tool invocation.

## Failure and safety rules

- Missing `OPENROUTER_API_KEY` fails before starting the interactive console with a concise configuration error. The key is never shown in errors, state files, process arguments or plugin environments.
- A catalog timeout, HTTP error, malformed model entry or cancellation cannot change the selected model. Individual malformed entries are skipped; a malformed envelope fails the request.
- Catalog text is untrusted presentation data. Strip ANSI/OSC and control characters and keep rows within terminal bounds. Do not show an approval action behind the picker or allow the picker to cover an unresolved approval.
- Session switching still requires a workspace compatible with the configured executor. Loading a session does not issue a provider request. Changing the model does not resume an interrupted turn.
- The existing 32-call turn cap, explicit tool decisions, uncertain-effect checkpoint and shutdown ordering remain intact.

## Verification

- CLI tests cover bare startup using the current directory, no snapshot before selection, direct `--model`, saved-session startup, missing key, invalid root and cancellation.
- Adapter tests use an HTTP fixture for catalog filtering, authentication, parsing, non-2xx responses, timeout/cancellation, size limit, malformed entries and key redaction. No live OpenRouter request is required.
- Domain/application tests cover validated model selection, save-before-publish, rollback on save failure, busy/pending rejection, interrupted-session behavior and version-1 snapshot restoration after a model change.
- Terminal tests cover exact `/model` command handling, draft preservation, loading/error/retry, search and keyboard/mouse selection, narrow/wide resize, ANSI sanitization, approval focus and no provider call on selection.
- The root and TUI module gates remain: race tests, `go vet`, formatting, static builds and aggregate statement coverage above 80% in each module. A PTY smoke test verifies bare startup and the command path without a live model request.

## Scope limits

- The selector does not rank or recommend models, silently choose a paid model, or auto-select the first result.
- It does not alter OpenRouter account preferences, plugin manifests, or the JSON worker.
- Catalog availability is not required when starting with explicit `--model` or loading a saved session.
