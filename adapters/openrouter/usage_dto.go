package openrouter

type usageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// PromptTokensDetails carries the prompt cache counts OpenRouter reports
	// for providers that cache; absent otherwise.
	PromptTokensDetails     *promptTokensDetailsDTO     `json:"prompt_tokens_details"`
	CompletionTokensDetails *completionTokensDetailsDTO `json:"completion_tokens_details"`
	// Cost is what OpenRouter charged, in dollars; absent from local
	// servers, so a pointer tells an unknown cost from a free request.
	Cost *float64 `json:"cost"`
	// IsBYOK marks a request served with the person's own provider key:
	// Cost is then only OpenRouter's fee, and the provider bills
	// CostDetails.UpstreamInferenceCost to that key.
	IsBYOK      bool            `json:"is_byok"`
	CostDetails *costDetailsDTO `json:"cost_details"`
}

type promptTokensDetailsDTO struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type completionTokensDetailsDTO struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type costDetailsDTO struct {
	UpstreamInferenceCost float64 `json:"upstream_inference_cost"`
}
