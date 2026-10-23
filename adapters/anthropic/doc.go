// Package anthropic speaks the Anthropic Messages API directly, without
// OpenRouter's translation: streaming, tool use, extended thinking kept
// across tool calls through the signed thinking blocks, prompt caching with
// cache_control, and usage with cache read and write tokens. It implements
// application.ModelStreamPort, with Complete for whole replies, and returns
// the errors the console reads (adapters/provider).
package anthropic
