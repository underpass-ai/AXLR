package openrouter

import "context"

// StreamTimeoutError reports a stream inactivity or maximum-duration timeout.
type StreamTimeoutError struct {
	maximumDuration bool
	provider        string
}

// NewStreamTimeoutError is the error a stream of provider (OpenRouter when
// empty) ends with when it is cut at its maximum duration or, otherwise,
// for inactivity.
func NewStreamTimeoutError(provider string, maximumDuration bool) *StreamTimeoutError {
	return &StreamTimeoutError{maximumDuration: maximumDuration, provider: provider}
}

func (e *StreamTimeoutError) Error() string {
	provider := e.provider
	if provider == "" {
		provider = providerName
	}
	if e.maximumDuration {
		return provider + " stream maximum duration exceeded"
	}
	return provider + " stream inactivity timeout"
}
func (*StreamTimeoutError) Unwrap() error { return context.DeadlineExceeded }

// MaximumDuration reports a stream cut at its maximum duration rather than
// for inactivity, so a console can say which in its own words.
func (e *StreamTimeoutError) MaximumDuration() bool { return e.maximumDuration }

// Provider names the endpoint whose stream timed out.
func (e *StreamTimeoutError) Provider() string {
	if e.provider == "" {
		return providerName
	}
	return e.provider
}
