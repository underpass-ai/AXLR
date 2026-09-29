package openrouter

import "context"

// StreamTimeoutError reports a stream inactivity or maximum-duration timeout.
type StreamTimeoutError struct{ maximumDuration bool }

func (e *StreamTimeoutError) Error() string {
	if e.maximumDuration {
		return "OpenRouter stream maximum duration exceeded"
	}
	return "OpenRouter stream inactivity timeout"
}
func (*StreamTimeoutError) Unwrap() error { return context.DeadlineExceeded }
