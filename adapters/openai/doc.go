// Package openai speaks the OpenAI Responses API directly, without
// OpenRouter's translation: streaming, function tools, reasoning kept across
// tool calls by replaying the encrypted reasoning items (the request is
// stateless, store: false, so AXLR's own history stays the record), and
// usage with cached and reasoning tokens. It implements
// application.ModelStreamPort, with Complete for whole replies, and returns
// the errors the console reads (adapters/provider).
package openai
