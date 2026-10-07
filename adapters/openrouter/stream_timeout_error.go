package openrouter

import "context"

// StreamTimeoutError reports a stream inactivity or maximum-duration timeout.
type StreamTimeoutError struct {
	maximumDuration bool
	provider        string
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
