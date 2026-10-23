package provider

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
	if e.maximumDuration {
		return e.Provider() + " stream maximum duration exceeded"
	}
	return e.Provider() + " stream inactivity timeout"
}
func (*StreamTimeoutError) Unwrap() error { return context.DeadlineExceeded }

// MaximumDuration reports a stream cut at its maximum duration rather than
// for inactivity, so a console can say which in its own words.
func (e *StreamTimeoutError) MaximumDuration() bool { return e.maximumDuration }

// Provider names the endpoint whose stream timed out.
func (e *StreamTimeoutError) Provider() string {
	if e.provider == "" {
		return DefaultProviderName
	}
	return e.provider
}
