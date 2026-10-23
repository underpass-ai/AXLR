// Package provider holds what the model adapters share: the errors the
// console reads for its hints (ProviderError, TransportError,
// StreamTimeoutError), the bounded SSE decoder, the endpoint rules and the
// repair of tool arguments a model sent broken. adapters/openrouter,
// adapters/anthropic and adapters/openai build on it; the console keeps
// reading the error types through adapters/openrouter's aliases.
package provider
