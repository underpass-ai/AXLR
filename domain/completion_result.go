package domain

type CompletionResult struct {
	Message      Message
	FinishReason FinishReason
	Usage        *TokenUsage
	// RequestBytes is the size of the request body the adapter sent; zero
	// when unknown. With Usage.PromptTokens it measures how many bytes a
	// prompt token of this model holds. A request that failed once sent
	// reports it beside the error, with Provider when known, so its cost
	// can be accounted for although no usage arrived.
	RequestBytes int
	// Provider names who served the request when the endpoint says, such as
	// the upstream OpenRouter routed to; empty otherwise.
	Provider string
	// GenerationID is OpenRouter's id of the request (gen-…), from the
	// first chunk of its stream that named one; empty for other endpoints
	// and for a stream that failed before its first chunk. A request that
	// ended without a usage report carries it beside the error, so its
	// exact cost can be asked for later.
	GenerationID string
}
