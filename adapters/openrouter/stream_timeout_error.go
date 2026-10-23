package openrouter

import "github.com/underpass-ai/AXLR/adapters/provider"

// StreamTimeoutError reports a stream inactivity or maximum-duration timeout.
type StreamTimeoutError = provider.StreamTimeoutError

// NewStreamTimeoutError is the error a stream of provider (OpenRouter when
// empty) ends with when it is cut at its maximum duration or, otherwise,
// for inactivity.
func NewStreamTimeoutError(providerName string, maximumDuration bool) *StreamTimeoutError {
	return provider.NewStreamTimeoutError(providerName, maximumDuration)
}
