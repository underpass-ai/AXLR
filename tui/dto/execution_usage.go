package dto

// ExecutionUsage is what an execution consumed over every session it
// spanned.
type ExecutionUsage struct {
	Requests         int     `json:"requests"`
	PromptTokens     int     `json:"prompt_tokens"`
	CachedTokens     int     `json:"cached_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	CostRequests     int     `json:"cost_requests"`
	Unmetered        int     `json:"unmetered_requests"`
}
