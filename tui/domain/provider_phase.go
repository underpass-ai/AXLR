package domain

// ProviderPhase describes activity without exposing the model's reasoning text.
type ProviderPhase string

const (
	ProviderWaiting   ProviderPhase = "waiting"
	ProviderReasoning ProviderPhase = "reasoning"
	ProviderToolCall  ProviderPhase = "tool_call"
	ProviderContent   ProviderPhase = "content"
)
