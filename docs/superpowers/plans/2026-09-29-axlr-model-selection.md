# AXLR Model Selection Implementation Plan

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../../index.md) and its audited revision.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users choose tool-capable OpenRouter models through `/model`, change the current session's model safely, and start `axlr-tui` without flags.

**Architecture:** Add a typed model-catalog port and an OpenRouter HTTP adapter in the separate TUI module. A composed picker fetches the catalog on demand; application use cases create or update a persisted session only after explicit selection. The root Bubble Tea model supports an unconfigured startup state without an invalid domain session.

**Tech Stack:** Go 1.26; existing Bubble Tea v2, Bubbles v2, Lip Gloss v2 and BubbleZone v2; OpenRouter `GET /api/v1/models`.

**Spec:** `docs/superpowers/specs/2026-09-29-axlr-model-selection-design.md`

## Global Constraints

- Keep `tui/` a separate Go module and root `cmd/axlr` unchanged. No new root-module terminal dependency.
- `/model` is a control command, never a model-history message. No automatic model choice or model request on startup, catalog fetch or selection.
- `axlr-tui` without flags uses the current directory and opens without a persisted session; exact `/model` via Enter or Ctrl+S opens the selector. Explicit `--model` bypasses discovery. `--session` loads its saved model.
- Catalog results must support `tools` and text output. Preserve existing model, session and draft after fetch or save failure. Never show or persist the API key.
- A model change may occur only with no running operation and no pending approval; an interrupted session without pending calls qualifies. Save a validated copy before publishing it.
- Reuse root-only BubbleZone scanning, terminal sanitization, owner-only atomic snapshots and cancellation-aware operation lifecycle. One primary Go type per file.
- Preserve race tests, `go vet`, formatting, static builds and aggregate coverage above 80% for both modules in the existing fast CI workflow.

## Review Focus

1. Bare startup must not write an invalid or empty-model snapshot; Task 6 tests quitting and failed discovery before selection.
2. A stale catalog completion after cancel/session change must not overwrite the active model; Task 5 tests completion ordering and cancellation.
3. `/model` typed in the composer must never reach OpenRouter as a user message, while an ordinary unsent draft survives a blocked send; Task 5 tests both.
4. A pending tool call must remain awaiting its explicit decision even if `/model` is requested by key, palette or mouse; Task 5 tests all routes.
5. Malformed provider metadata, ANSI text, oversized replies and catalog errors must not corrupt the picker or expose the key; Tasks 2 and 4 test these inputs.

## File map

- Catalog values and port: create `tui/domain/available_model.go`, `context_window.go`, `model_rate.go`, `tui/application/model_catalog_port.go`, `list_models_use_case.go` and focused tests.
- Provider adapter: create `tui/adapters/openrouter/model_catalog.go`, `model_catalog_response.go`, `model_catalog_test.go`.
- Session selection: modify `tui/domain/session.go` and tests; create `tui/application/create_session_use_case.go`, `change_session_model_use_case.go` and tests. Reuse snapshot version 1.
- Picker: create `tui/adapters/terminal/model_picker.go` and tests. It holds presentation state only.
- Terminal integration: modify `app_model.go`, `dependencies.go`, `navigation.go`, `action_palette.go`, `help_overlay.go`, `header.go`; add model-selection tests. Keep background work behind `BeginOperation`.
- Composition: modify `tui/cmd/axlr-tui/run.go`, CLI tests, `tui/README.md`, root README if its invocation example requires correction, and CI only if new checks are needed. Rebuild the installed user binary after the final review.

---

### Task 1: Typed model catalog contract

**Files:** Create `tui/domain/available_model.go`, `context_window.go`, `model_rate.go`, `available_model_test.go`, `tui/application/model_catalog_port.go`, `list_models_use_case.go`, `list_models_use_case_test.go`.

**Interfaces:** `AvailableModel{ID root.ModelID, Name root.Text, Context ContextWindow, PromptRate ModelRate, CompletionRate ModelRate, SupportsTools bool, TextOutput bool}`; zero context/rate values mean unavailable metadata. `NewContextWindow(int) (ContextWindow,error)` accepts positive token counts; `NewModelRate(perToken string) (ModelRate,error)` accepts nonnegative decimal USD/token and `ModelRate.Display() string` formats USD per million tokens. `ModelCatalogPort.List(context.Context) ([]domain.AvailableModel,error)` is provider-neutral. `ListModelsUseCase{Catalog ModelCatalogPort}.Execute(context.Context) ([]domain.AvailableModel,error)` validates, filters to both capabilities, deduplicates IDs and sorts by display name then ID. Invalid catalog entries are skipped; an empty usable catalog is a visible error.

- [ ] **Step 1: Write failing tests** for typed IDs/rates, tool/text filtering, duplicate IDs, stable sorting, nil port, cancelled context and empty filtered results.
- [ ] **Step 2: Run** `go -C tui test ./domain ./application -run 'TestAvailableModel|TestListModels' -count=1`; expect missing symbols.
- [ ] **Step 3: Implement** the three values, port and use case with the exact signatures above; return copied catalog slices so callers cannot mutate provider state.
- [ ] **Step 4: Run** `go -C tui test -race ./domain ./application -count=1`; expect PASS.
- [ ] **Step 5: Commit** `feat: define AXLR model catalog contract`.

### Task 2: OpenRouter catalog adapter

**Files:** Create `tui/adapters/openrouter/model_catalog.go`, `model_catalog_response.go`, `model_catalog_test.go`.

**Interfaces:** `ModelCatalog{APIKey string, HTTPClient *http.Client}.List(ctx)` implements Task 1's port. Send one authenticated GET to `https://openrouter.ai/api/v1/models?supported_parameters=tools&output_modalities=text`; use a 10-second maximum timeout and 8 MiB response cap. Map the API's ID, name, context length, pricing and capability metadata to Task 1's values. Do not log body/key or follow redirects.

- [ ] **Step 1: Write failing HTTP-fixture tests** for query/header, valid list, missing or malformed item fields, absent tool/text support, invalid optional prices, 401/429/500, malformed envelope, 8 MiB limit, timeout/cancel and key redaction. Assert one request and no completion call.
- [ ] **Step 2: Run** `go -C tui test ./adapters/openrouter -count=1`; expect missing symbols.
- [ ] **Step 3: Implement** the adapter and DTO mapping. Skip malformed entries and optional metadata while rejecting an invalid envelope; close every response body.
- [ ] **Step 4: Run** `go -C tui test -race ./adapters/openrouter ./application -count=1`; expect PASS.
- [ ] **Step 5: Commit** `feat: list OpenRouter models for AXLR`.

### Task 3: Persist explicit session model selection

**Files:** Modify `tui/domain/session.go`, `session_test.go`; create `tui/application/create_session_use_case.go`, `change_session_model_use_case.go`, `model_selection_test.go`.

**Interfaces:** `(*domain.Session).ChangeModel(root.ModelID) error` validates the ID and rejects streaming/approval or any pending call. `CreateSessionUseCase{Store SessionStorePort}.Execute(ctx, id domain.SessionID, workspace domain.Workspace, model root.ModelID) (domain.Session,error)` creates and saves before return. `ChangeSessionModelUseCase{Store SessionStorePort}.Execute(ctx, session *domain.Session, model root.ModelID) error` changes a copy, saves it, then publishes. Keep snapshot DTO version 1 and existing transcript unchanged.

- [ ] **Step 1: Write failing tests** for idle/complete/interrupted-without-pending changes, busy/pending rejection, invalid ID, no history mutation, save rollback, restored version-1 snapshot and no automatic request on change.
- [ ] **Step 2: Run** `go -C tui test ./domain ./application -run 'TestChangeModel|TestCreateSession|TestChangeSessionModel' -count=1`; expect missing methods/types.
- [ ] **Step 3: Implement** the aggregate transition and use cases through the existing `SessionStorePort`, using copy/save/publish ordering.
- [ ] **Step 4: Run** `go -C tui test -race ./domain ./application ./adapters/storage -count=1`; expect PASS.
- [ ] **Step 5: Commit** `feat: persist AXLR session model changes`.

### Task 4: Reusable model picker

**Files:** Create `tui/adapters/terminal/model_picker.go`, `model_picker_test.go`.

**Interfaces:** `ModelPicker` owns query, model rows, selected index, loading/error state and visible-window position. `NewModelPicker() ModelPicker`, `(*ModelPicker).SetModels([]domain.AvailableModel)`, `(*ModelPicker).SelectedModel() (domain.AvailableModel,bool)`, `ModelPicker.Update(msg tea.Msg, zones *zone.Manager, prefix string) (ModelPicker, ControlIntent, tea.Cmd)` and `ModelPicker.View(zones *zone.Manager, prefix string, width, height int) string` are its public seam. Its typed select/retry/close intents come from keyboard or mouse. It filters name/ID/provider locally and uses child `zone.Mark`; only the root calls `zone.Scan`.

- [ ] **Step 1: Write failing tests** for searchable rows, keyboard/mouse parity, empty/error/retry states, PgUp/PgDn, 70×20 and 40×10 layouts, wide glyphs, ANSI/OSC sanitization and query/selection preservation on resize.
- [ ] **Step 2: Run** `go -C tui test ./adapters/terminal -run TestModelPicker -count=1`; expect missing type.
- [ ] **Step 3: Implement** one focused composed picker, reusing Bubbles text input and the existing theme/zone conventions; no HTTP or persistence import.
- [ ] **Step 4: Run** `go -C tui test -race ./adapters/terminal -run TestModelPicker -count=1`; expect PASS.
- [ ] **Step 5: Commit** `feat: add searchable TUI model picker`.

### Task 5: `/model` command and terminal orchestration

**Files:** Modify `tui/adapters/terminal/app_model.go`, `dependencies.go`, `navigation.go`, `action_palette.go`, `help_overlay.go`, `header.go`; create `model_selection_test.go`.

**Interfaces:** `Dependencies` gains `Models application.ListModelsUseCase`, `Create application.CreateSessionUseCase`, `Change application.ChangeSessionModelUseCase`, `Workspace domain.Workspace`, and `NewSessionID domain.SessionID`. Exact `/model` via Enter or Ctrl+S opens Task 4's picker and never calls `StartTurnUseCase`; palette `Models` does the same. Fetch and selection run through `BeginOperation` on private snapshots; `operationComplete` carries an optional catalog result and publishes only the current operation's results. A zero-value session is presentation-only until explicit selection creates and saves a valid session.

- [ ] **Step 1: Write failing behavioral tests** for bare screen, draft-preserving blocked send, slash command not in history, direct flag session, loading/error/retry, create/change persistence, pending approval priority, busy rejection, stale completion, picker focus and post-selection prompt send.
- [ ] **Step 2: Run** `go -C tui test ./adapters/terminal -run 'TestModelSelection|TestSlashModel' -count=1`; expect missing wiring.
- [ ] **Step 3: Implement** root routing and composed presentation; preserve all existing approval, cancellation, resize and editor behavior.
- [ ] **Step 4: Run** `go -C tui test -race ./adapters/terminal -count=1`; expect PASS.
- [ ] **Step 5: Commit** `feat: select models with slash command`.

### Task 6: Bare CLI startup, docs and installed binary

**Files:** Modify `tui/cmd/axlr-tui/run.go`, `run_test.go`, `tui/README.md`, root `README.md` if needed, `.github/workflows/ci.yml` only if needed.

**Interfaces:** `run` keeps its existing signature. `--root` defaults to the current directory; `--model` is optional, and `--session` restores its saved model. Without either model source, compose Task 5's unconfigured TUI with a preallocated cryptographic 32-hex ID but do not save it. Explicit `--model` still skips catalog fetch. The OpenRouter key remains required and never reaches the picker, disk or plugins.

- [ ] **Step 1: Write failing CLI tests** for no-flag launch and quit without snapshot or lock file, `/model` selection through a fake HTTP catalog, resumed session without `--model`, direct `--model` without catalog request, missing key, workspace mismatch, plugin validation, model selection failure and no provider request before prompt.
- [ ] **Step 2: Run** `go -C tui test ./cmd/axlr-tui -run TestRunModelSelection -count=1`; expect FAIL.
- [ ] **Step 3: Implement** CLI composition and update user documentation/CI assertions. Keep existing root worker and module dependencies untouched.
- [ ] **Step 4: Run** `GOWORK=off go test -race -count=1 -coverpkg=./... -coverprofile=/tmp/axlr-model-root.cover ./...` and `go -C tui test -race -count=1 -coverpkg=./... -coverprofile=/tmp/axlr-model-tui.cover ./...`; both must pass and totals from `go tool cover -func` exceed 80%.
- [ ] **Step 5: Run** `gofmt -l .`, both `go vet ./...` gates, static builds of `./cmd/axlr` and `./cmd/axlr-tui`, and a PTY smoke test of bare startup, `/model` loading/error display and clean exit without a chat request. The HTTP-fixture CLI test in Step 1 covers a successful selection without live credentials.
- [ ] **Step 6: Commit** `feat: launch AXLR TUI with model selection`. After final review, rebuild and replace `/home/gx10a/.local/bin/axlr-tui`, verify `--help` and bare startup, then update the existing PR #6.

## Final review

- [ ] Compare every spec section with the six deliverables, including failure modes and unchanged worker behavior.
- [ ] Request a whole-branch review on the feature range, fix actionable findings, and repeat the root/TUI verification gates on the final commit before updating the installed binary and PR.
