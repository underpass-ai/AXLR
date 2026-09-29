# OpenRouter Model Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Go library client that obtains OpenRouter assistant text and tool-call requests without executing tools.

**Architecture:** AXLR domain types describe model conversations and validate their invariants. An application use case calls a model port. A standard-library HTTP adapter maps those types to OpenRouter chat completions and maps the first choice back; the existing worker remains separate.

**Tech Stack:** Go 1.26, `net/http`, `encoding/json`, `httptest`; no new root-module dependencies.

**Spec:** `docs/superpowers/specs/2026-09-29-openrouter-model-client-design.md`

## Global Constraints

- AXLR is Underpass's own runtime; no Pi code or runtime.
- Library-only, non-streaming, fixed production endpoint `https://openrouter.ai/api/v1/chat/completions`.
- No automatic tool execution, completion retry, provider routing, new worker operation, or root-level Go source.
- API key injected explicitly; it must not appear in errors or logs. No redirects or optional attribution headers.
- One primary Go type per file; domain and application import no HTTP, OpenRouter, MCP, or worker DTO types.
- Provider response cap: 8 MiB. Aggregate root coverage must exceed 80%; keep CI minimal.

## Review Focus

- A model ID containing only whitespace or control characters must fail before HTTP; Task 1 pins this.
- Reused assistant tool-call IDs or an unmatched tool-result ID must fail before HTTP; Task 1 pins this.
- A nil model port must return an error rather than panic; Task 2 pins this.
- OpenRouter `content: null` with valid tool calls must map to a valid assistant message; Task 4 pins this.
- An HTTP redirect must not forward the API key; Task 5 pins this.

---

### Task 1: Typed conversation domain

**Files:** Create `domain/model_id.go`, `domain/tool_name.go`, `domain/tool_call_id.go`, `domain/json_object.go`, `domain/message_role.go`, `domain/message.go`, `domain/tool_call.go`, `domain/tool_definition.go`, `domain/completion_request.go`, `domain/completion_result.go`, `domain/finish_reason.go`, `domain/token_usage.go`, `domain/model_conversation_test.go`.

**Interfaces:** `NewModelID(string) (ModelID, error)`, `NewToolName(string) (ToolName, error)`, `NewToolCallID(string) (ToolCallID, error)`, `NewJSONObject([]byte) (JSONObject, error)`, `JSONObject.Bytes() []byte`, `Message.Validate() error`, `CompletionRequest.Validate() error`. `Message` has `Role MessageRole`, `Content Text`, `ToolCalls []ToolCall`, `ToolCallID ToolCallID`; roles are typed constants `RoleSystem`, `RoleUser`, `RoleAssistant`, `RoleTool`. `ToolCall` has `ID ToolCallID`, `Name ToolName`, `Arguments JSONObject`. `ToolDefinition` has `Name ToolName`, `Description Text`, `Parameters JSONObject`. `CompletionRequest` has `Model ModelID`, `Messages []Message`, `Tools []ToolDefinition`; `CompletionResult` has `Message Message`, `FinishReason FinishReason`, `Usage *TokenUsage`. `TokenUsage` has `PromptTokens int`, `CompletionTokens int`, `TotalTokens int`.

- [ ] **Step 1: Write failing tests** in `domain/model_conversation_test.go`: `TestModelIdentifiersRejectBlankAndControl` checks `"  "` and `"a\n"`; `TestJSONObjectCopiesAndRejectsNonObjects` checks `[]`, malformed JSON, and mutation of input/output bytes; `TestCompletionRequestValidatesConversation` checks no messages, duplicate tool names, duplicate assistant call IDs, unmatched tool-result ID, and a valid user → assistant calls → tool results sequence with two calls.
- [ ] **Step 2: Run** `go test ./domain -run 'TestModelIdentifiers|TestJSONObject|TestCompletionRequest' -count=1`; expect compilation failure for the missing types.
- [ ] **Step 3: Implement** the listed constructors and `Validate` methods. Model ID, tool name and call ID reject blank/whitespace-only values and control characters. `CompletionRequest.Validate` rejects invalid zero values, duplicate tool definitions and call IDs, repeated tool results, and tool results whose IDs do not occur in a preceding assistant message; it allows assistant text plus calls and tool messages with empty content. `JSONObject` owns copied bytes and marshals as an object, never as a quoted JSON string.
- [ ] **Step 4: Run** `go test ./domain -count=1`; expect PASS.
- [ ] **Step 5: Commit** the task as `feat: add typed model conversation domain`.

### Task 2: Model port and completion use case

**Files:** Create `application/model_port.go`, `application/complete_model_use_case.go`, `application/complete_model_use_case_test.go`.

**Interfaces:** `type ModelPort interface { Complete(context.Context, domain.CompletionRequest) (domain.CompletionResult, error) }`; `type CompleteModelUseCase struct { Models ModelPort }`; `func (u CompleteModelUseCase) Execute(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error)` validates the request, checks for a nil port, then forwards the same context and request once. Task 5's adapter implements `ModelPort`.

- [ ] **Step 1: Write failing tests** in `application/complete_model_use_case_test.go`: invalid request and nil port each return an error with zero port calls; a valid request forwards context and request once and returns the port result; a port error propagates unchanged.
- [ ] **Step 2: Run** `go test ./application -run TestCompleteModelUseCase -count=1`; expect compilation failure for the missing port/use case.
- [ ] **Step 3: Implement** `ModelPort` and `CompleteModelUseCase.Execute` with the signatures above.
- [ ] **Step 4: Run** `go test ./application -count=1`; expect PASS.
- [ ] **Step 5: Commit** the task as `feat: add model completion port and use case`.

### Task 3: OpenRouter request mapping

**Files:** Create `adapters/openrouter/request_dto.go`, `adapters/openrouter/message_dto.go`, `adapters/openrouter/tool_dto.go`, `adapters/openrouter/tool_call_dto.go`, `adapters/openrouter/request_mapper.go`, `adapters/openrouter/request_mapper_test.go`.

**Interfaces:** `func mapRequest(domain.CompletionRequest) (requestDTO, error)`. Wire DTOs stay private to `adapters/openrouter`. `requestDTO` has JSON fields `model`, `messages`, optional `tools`, and `stream:false`. Assistant tool-call messages serialize `content:null` when text is empty; tool results serialize `role:tool`, `tool_call_id`, and textual `content`. Tool definitions use `type:function` and `function.parameters` as a JSON object. Function-call `arguments` is a JSON-encoded string, not an object.

- [ ] **Step 1: Write failing tests** in `request_mapper_test.go`: text-only request JSON has `model`, `messages`, `stream:false`, no `tools`; a follow-up with two assistant calls and two tool results preserves order, call IDs, JSON-string arguments and the repeated `tools` definitions; assistant text plus calls retains both; invalid domain request returns an error.
- [ ] **Step 2: Run** `go test ./adapters/openrouter -run TestMapRequest -count=1`; expect compilation failure.
- [ ] **Step 3: Implement** `mapRequest` and the four private DTO types; use `json.RawMessage` for schemas and `JSONObject.Bytes()` for stringified arguments.
- [ ] **Step 4: Run** `go test ./adapters/openrouter -run TestMapRequest -count=1`; expect PASS.
- [ ] **Step 5: Commit** the task as `feat: map model requests to OpenRouter`.

### Task 4: OpenRouter response mapping

**Files:** Create `adapters/openrouter/response_dto.go`, `adapters/openrouter/choice_dto.go`, `adapters/openrouter/usage_dto.go`, `adapters/openrouter/response_mapper.go`, `adapters/openrouter/response_mapper_test.go`.

**Interfaces:** `func mapResponse(responseDTO) (domain.CompletionResult, error)`. Decode the first choice only. Preserve assistant content, all tool calls in order, finish reason and optional token usage. `content:null` is valid when calls exist; no content and no calls is invalid. Each function's `arguments` must decode as a JSON object.

- [ ] **Step 1: Write failing tests** in `response_mapper_test.go`: text response maps finish reason and usage; assistant text plus two parallel calls maps both; null content plus one call succeeds; missing choices, wrong role, blank call ID/name, duplicate call ID, non-object/malformed arguments, and empty assistant message fail.
- [ ] **Step 2: Run** `go test ./adapters/openrouter -run TestMapResponse -count=1`; expect compilation failure.
- [ ] **Step 3: Implement** private response DTOs and `mapResponse`; validate the produced assistant message and reject duplicate call IDs.
- [ ] **Step 4: Run** `go test ./adapters/openrouter -run TestMapResponse -count=1`; expect PASS.
- [ ] **Step 5: Commit** the task as `feat: map OpenRouter model responses`.

### Task 5: HTTP adapter, documentation and verification

**Files:** Create `adapters/openrouter/client_config.go`, `adapters/openrouter/client.go`, `adapters/openrouter/provider_error.go`, `adapters/openrouter/provider_error_category.go`, `adapters/openrouter/transport_error.go`, `adapters/openrouter/client_test.go`; modify `README.md`.

**Interfaces:** `type ClientConfig struct { APIKey string; HTTPClient *http.Client }`; `func New(config ClientConfig) (*Client, error)` rejects an empty key; `func (c *Client) Complete(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error)` implements `application.ModelPort`. `ProviderError` exposes `StatusCode int` and `Category ProviderErrorCategory`; category constants are `authentication`, `insufficient_credits`, `rate_limited`, `invalid_request`, `provider_failure`. `TransportError` exposes `Cause error` and `Unwrap() error`. The production URL is exactly `https://openrouter.ai/api/v1/chat/completions`.

- [ ] **Step 1: Write failing tests** in `client_test.go` with an injected `http.RoundTripper`: a blank key is rejected; a valid call through `CompleteModelUseCase` uses POST, bearer and JSON headers, a fixed OpenRouter URL, and one HTTP request; an unknown extra response field is ignored; response bodies beyond 8 MiB, malformed JSON and a 2xx response without a choice fail; 401/403, 402, 429, 400/422 and 5xx map to the stated categories without echoing API key or raw body; canceled context avoids HTTP; transport error is discoverable with `errors.As` and unwraps its cause; redirect response is returned as an error without a second request.
- [ ] **Step 2: Run** `go test ./adapters/openrouter -run TestClient -count=1`; expect compilation failure.
- [ ] **Step 3: Implement** `New` and `Complete`: copy or construct the supplied HTTP client with redirect following disabled, serialize `mapRequest`, issue one context-bound request, limit response reading to 8 MiB + 1 byte, classify non-2xx without exposing body text, decode `responseDTO`, then `mapResponse`. Do not retry. Never include API key in formatted errors.
- [ ] **Step 4: Run** `go test ./adapters/openrouter -count=1`; expect PASS.
- [ ] **Step 5: Add** a README example that constructs `openrouter.New(openrouter.ClientConfig{APIKey: os.Getenv("OPENROUTER_API_KEY")})`, uses `application.CompleteModelUseCase`, and explains appending returned assistant calls and tool results for the next request. State explicitly that the library does not execute tools.
- [ ] **Step 6: Verify** `gofmt -l .`, `go vet ./...`, `go test -race -count=1 -coverpkg=./... -coverprofile=coverage.out ./...`, `go tool cover -func=coverage.out` with total above 80%, and `CGO_ENABLED=0 go build -trimpath -o /tmp/axlr ./cmd/axlr`; expect all pass. Do not add a CI job.
- [ ] **Step 7: Commit** the task as `feat: connect AXLR model port to OpenRouter`.
