package dto

import "time"

// ReceiptRequest is one model request in a receipt. CostUSD is null when
// the request reported no cost, DurationMS absent for one that never ended.
type ReceiptRequest struct {
	Session          string    `json:"session_id"`
	Index            int       `json:"index"`
	StartedAt        time.Time `json:"started_at"`
	DurationMS       *int64    `json:"duration_ms,omitempty"`
	Status           string    `json:"status"`
	Model            string    `json:"model"`
	Route            string    `json:"route,omitempty"`
	Provider         string    `json:"provider,omitempty"`
	PromptTokens     int       `json:"prompt_tokens"`
	CachedTokens     int       `json:"cached_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	ReasoningTokens  int       `json:"reasoning_tokens"`
	CostUSD          *float64  `json:"cost_usd"`
	Unmetered        bool      `json:"unmetered"`
}
