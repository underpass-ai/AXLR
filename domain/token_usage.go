package domain

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// CachedTokens are the prompt tokens the provider read from its prompt
	// cache, and CacheWriteTokens those it wrote to it; zero when the
	// provider reports no cache.
	CachedTokens     int
	CacheWriteTokens int
}
