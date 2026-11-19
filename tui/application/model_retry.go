package application

import (
	"context"
	"errors"
	"time"
)

// defaultModelRetryWaits: two more attempts, 2 and 8 seconds after the
// failure before each. Codex's own client retries a failed stream too.
var defaultModelRetryWaits = []time.Duration{2 * time.Second, 8 * time.Second}

// transientFailure is a model error that says whether asking again may
// succeed: the provider adapters' ProviderError (a 5xx or an error without
// a known code) and TransportError say so, a refused key, no credits, a
// bad request or a rate limit do not.
type transientFailure interface{ Transient() bool }

// transientModelFailure reports whether err is a failure worth asking the
// same model again; a timeout of the stream, a cancel or an unknown error
// is not.
func transientModelFailure(err error) bool {
	var failure transientFailure
	return err != nil && errors.As(err, &failure) && failure.Transient()
}

func (u ContinueTurnUseCase) modelRetryWaits() []time.Duration {
	if u.ModelRetryWaits == nil {
		return defaultModelRetryWaits
	}
	return u.ModelRetryWaits
}

// waitForRetry waits, false when the caller stopped the turn meanwhile.
func waitForRetry(ctx context.Context, wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
