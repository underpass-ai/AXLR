package domain

import "time"

// PendingGeneration is an unmetered request whose exact cost OpenRouter
// may still report: its generation id, the provider subtotal it was
// counted in and the tokens and cost estimated for it, which the exact
// numbers replace. Cancelling a stream does not stop every provider: some keep
// generating and bill the whole answer, and the request's stats are final
// only once they finish, so the lookup is retried at later requests
// rather than once after the turn.
type PendingGeneration struct {
	ID               string
	Provider         string
	PromptTokens     int
	CompletionTokens int
	// EstimatedCost is what the request counts in the ledger's
	// EstimatedCost until it is settled, when Estimated.
	EstimatedCost float64
	Estimated     bool
	// Ended is when the request ended; Tried, when its cost was last asked
	// for, zero before the first lookup, and Lookups how many times.
	Ended   time.Time
	Tried   time.Time
	Lookups int
}

const (
	// MaxPendingGenerations bounds the pending list; an unmetered request
	// past it keeps its estimate.
	MaxPendingGenerations = 16
	// MaxGenerationLookups and MaxGenerationWait bound how often and for
	// how long a request's cost is asked for; past either it keeps its
	// estimate and stays unmetered. GenerationLookupSpacing is the least
	// time between two lookups of one request.
	MaxGenerationLookups    = 6
	MaxGenerationWait       = 30 * time.Minute
	GenerationLookupSpacing = 2 * time.Second
)

// Due reports whether the request's cost may be asked for at now: never
// asked, or asked at least GenerationLookupSpacing ago, and not expired.
func (p PendingGeneration) Due(now time.Time) bool {
	return !p.Expired(now) && (p.Tried.IsZero() || now.Sub(p.Tried) >= GenerationLookupSpacing)
}

// Expired reports whether the request has been waited for long enough or
// asked for often enough.
func (p PendingGeneration) Expired(now time.Time) bool {
	return p.Lookups >= MaxGenerationLookups || now.Sub(p.Ended) >= MaxGenerationWait
}
