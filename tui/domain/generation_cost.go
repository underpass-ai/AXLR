package domain

// GenerationCost is what OpenRouter's generation endpoint reports of one
// request once its stats are final: the dollars it charged and the tokens
// it counted. It replaces the estimate of a request that ended without a
// usage report. TokensKnown is false when the stats gave no token
// counts: the estimated ones are kept.
type GenerationCost struct {
	Cost             float64
	PromptTokens     int
	CompletionTokens int
	TokensKnown      bool
}
