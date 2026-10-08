package openrouter

import "strings"

// Prompt caching. OpenRouter forwards Anthropic's prompt cache for anthropic/*
// models on Anthropic, Claude Platform on AWS, Bedrock and Vertex: a cached
// prefix is read at a tenth of the input price and written at 1.25 times it,
// with a five-minute life that every request renews, and a prefix below 512
// tokens is not cached. Two breakpoints: the top-level cache_control asks
// for the automatic one, which OpenRouter places on the last block and moves
// forward as the conversation grows; the explicit one on the system prompt
// keeps the fixed prefix (schemas and guidance) readable after a cut of the
// history, which rewrites everything behind it. Session c33e8e86 (8 October
// 2026) sent 30.3M prompt tokens with no cache_control and cached nothing.
// Other models and other endpoints receive the request unchanged.

type cacheControlDTO struct {
	Type string `json:"type"`
}

type contentBlockDTO struct {
	Type         string           `json:"type"`
	Text         string           `json:"text"`
	CacheControl *cacheControlDTO `json:"cache_control,omitempty"`
}

// cachedSystemDTO is the system prompt as content blocks, the only form
// that carries a breakpoint.
type cachedSystemDTO struct {
	Role    string            `json:"role"`
	Content []contentBlockDTO `json:"content"`
}

// cachedRequestDTO is the request with its messages replaced: the outer
// Messages shadows the embedded one in the JSON encoding.
type cachedRequestDTO struct {
	requestDTO
	Messages     []any           `json:"messages"`
	CacheControl cacheControlDTO `json:"cache_control"`
}

// promptCached reports whether a request is worth marking: an Anthropic
// model through OpenRouter. A local server would see fields it never asked
// for, and OpenRouter ignores the marks for other providers.
func promptCached(openRouter bool, model string) bool {
	return openRouter && strings.HasPrefix(model, "anthropic/")
}

// promptCacheBody is the request body to encode: the wire request itself,
// or its cached form.
func promptCacheBody(wire requestDTO, openRouter bool) any {
	if !promptCached(openRouter, wire.Model) {
		return wire
	}
	ephemeral := cacheControlDTO{Type: "ephemeral"}
	messages := make([]any, 0, len(wire.Messages))
	for i, message := range wire.Messages {
		if i == 0 && message.Role == "system" && message.Content != nil {
			messages = append(messages, cachedSystemDTO{Role: message.Role, Content: []contentBlockDTO{{Type: "text", Text: *message.Content, CacheControl: &ephemeral}}})
			continue
		}
		messages = append(messages, message)
	}
	return cachedRequestDTO{requestDTO: wire, Messages: messages, CacheControl: ephemeral}
}
