package domain

type CompletionResult struct {
	Message      Message
	FinishReason FinishReason
	Usage        *TokenUsage
	// RequestBytes is the size of the request body the adapter sent; zero
	// when unknown. With Usage.PromptTokens it measures how many bytes a
	// prompt token of this model holds.
	RequestBytes int
	// Provider names who served the request when the endpoint says, such as
	// the upstream OpenRouter routed to; empty otherwise.
	Provider string
}
