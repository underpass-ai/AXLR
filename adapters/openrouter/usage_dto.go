package openrouter

type usageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// PromptTokensDetails carries the prompt cache counts OpenRouter reports
	// for providers that cache; absent otherwise.
	PromptTokensDetails *promptTokensDetailsDTO `json:"prompt_tokens_details"`
}

type promptTokensDetailsDTO struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}
