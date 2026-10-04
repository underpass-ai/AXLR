# AXLR OpenRouter model client: design

> **Historical record.** This page preserves its original design or investigation context. For current behavior and operating instructions, use the [current documentation](../../index.md) and its audited revision.

## Purpose and approved scope

AXLR needs a Go library client that can request a model completion through OpenRouter and return either assistant text, tool-call requests, or both. This is part of Underpass's own AXLR runtime. The host owns the conversation, chooses the model, provides the API key, decides whether to execute a requested tool, and supplies tool results in a later completion request.

The first delivery is library-only and non-streaming. It does not add a worker JSON operation, an agent loop, automatic tool execution, model discovery, or provider routing. It must work without the pending plugin PR: OpenRouter's tool definitions and calls are model-side data, while MCP plugins are one possible future execution target.

## Approach

Use a typed HTTP adapter built on Go's standard library. The adapter calls `POST https://openrouter.ai/api/v1/chat/completions` with bearer authentication and JSON. Domain and application packages own AXLR's model contract; the adapter owns OpenRouter wire DTOs and mappers. This avoids binding AXLR's public model API to the currently beta OpenRouter Go SDK and adds no root-module dependency. Using that SDK would reduce initial wire code but expose AXLR to its release and API changes. A generic OpenAI-compatible adapter would imply multiple-provider support that is outside this delivery.

The endpoint is fixed to OpenRouter in production. Construction accepts an HTTP client so tests can replace its transport without sending a credential to a configurable URL. The adapter does not follow redirects, send optional attribution headers, or retry requests automatically. The API key is supplied explicitly at construction and never included in errors or logs; reading `OPENROUTER_API_KEY` is the embedding host's responsibility.

## Components and contract

- `domain/` owns validated value objects for model ID, tool name, tool-call ID and JSON object arguments/schema; typed conversation messages, tool definitions, completion request, assistant result, finish reason and optional token usage. Model IDs are nonempty and reject control characters, but retain OpenRouter's actual ID syntax. Tool identities and call IDs cannot be blank. JSON arguments and schemas must be objects.
- `application/` owns a model port with one `Complete(context.Context, CompletionRequest) (CompletionResult, error)` operation and a completion use case. It validates the request before calling the port. Domain and application code import no HTTP, OpenRouter, MCP or worker DTO types.
- `adapters/openrouter/` implements the port. Its request/response DTOs and mapper functions stay under that adapter, not at the repository root or in the worker `dto/` package. One primary Go type per file is the project convention; helper functions may share that type's file.
- A library host constructs the adapter with its API key and HTTP client, passes it to the use case, and supplies a typed request containing a model, at least one message and optional tool definitions. Each call is stateless: the caller sends the full relevant message history.

Messages support `system`, `user`, `assistant` and `tool` roles. An assistant message may contain text and zero or more typed tool calls. A tool-result message carries the matching call ID and textual result. The completion result exposes the assistant message, all returned tool calls in order, finish reason and available token usage. The client must preserve a response containing both text and tool calls. It returns tool calls as data, never invokes them.

Tool definitions use a name, description and JSON Schema parameters object. Duplicate names in one request are invalid. The adapter maps them to OpenRouter `tools` with `type: function`. When the caller sends tool results in a later request, it also sends the assistant tool-call message and the same tool definitions; OpenRouter requires `tools` again on that request. Several tool calls in one response are supported. A provider-requested tool name is untrusted data; the host must authorize any execution against its own tool registry.

## Mapping and failure behavior

The adapter sends `stream: false` and selects the first choice from a valid completion response. It maps `message.content`, `message.tool_calls[*].id`, function name and JSON-string arguments, `finish_reason`, and token usage. It rejects missing choices, missing assistant role, malformed tool calls, non-object tool arguments, malformed JSON and a response body above 8 MiB rather than returning a partial result. Unknown extra provider response fields are ignored. An assistant message with no text and no calls is invalid.

Request validation errors return before network access. Context cancellation and deadlines propagate. Non-2xx responses become typed provider errors carrying the HTTP status and a stable category: authentication/authorization, insufficient credits, rate limit, invalid request, or provider/service failure. Error messages do not echo the request, API key or raw response body. Transport failures remain distinguishable from provider replies. No automatic retry occurs: a lost completion response does not prove OpenRouter did not process or bill the request.

## Acceptance and verification

- A small Go example in the README shows explicit key injection and a text completion. It explains how to append an assistant tool-call message plus tool-result messages for another completion. It never places a real key in code or shell history.
- Tests use an injected HTTP transport or local HTTP test server, not a real OpenRouter account. They cover text-only output, text plus parallel tool calls, the tool-result follow-up request, bearer and JSON headers, validation before network access, cancellation, HTTP error categories, malformed and oversized responses, redirect refusal, and no retry.
- `go vet`, race tests, a static worker build and aggregate root-module coverage above 80% pass. The existing CI job covers this package; no new CI job or external service is needed. The existing worker behavior remains unchanged.

API reference: [OpenRouter chat completions](https://openrouter.ai/docs/api/api-reference/chat/create-a-chat-completion) and [OpenRouter client tool calling](https://openrouter.ai/docs/guides/features/tool-calling).
