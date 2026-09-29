package openrouter

import "context"

// StreamTimeoutError reports a provider stream that stopped making progress.
type StreamTimeoutError struct{}

func (*StreamTimeoutError) Error() string { return "OpenRouter stream inactivity timeout" }
func (*StreamTimeoutError) Unwrap() error { return context.DeadlineExceeded }
