package domain

type CompletionResult struct {
	Message      Message
	FinishReason FinishReason
	Usage        *TokenUsage
}
