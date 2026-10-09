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
	// ReasoningTokens are the completion tokens the model spent reasoning;
	// zero when the provider does not say.
	ReasoningTokens int
	// Cost is what the request cost in US dollars when CostKnown. OpenRouter
	// reports it with every request; local servers report none, and their
	// requests are not free but unknown, never $0.
	Cost      float64
	CostKnown bool
}
