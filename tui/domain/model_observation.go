package domain

import "time"

// ModelObservation is one finished model request as the router remembers
// it: what it consumed, how long it took and whether the provider failed
// it. A request the caller cancelled says nothing about the model and is
// not observed.
type ModelObservation struct {
	At               time.Time
	Provider         string
	PromptTokens     int
	CompletionTokens int
	// Cost counts only when CostKnown, as in UsageSample.
	Cost      float64
	CostKnown bool
	FirstByte time.Duration
	Total     time.Duration
	Failed    bool
}

// ObservationOf is what the router remembers of a request's ledger sample.
func ObservationOf(sample UsageSample, failed bool, at time.Time) ModelObservation {
	return ModelObservation{
		At:               at.UTC(),
		Provider:         sample.Provider,
		PromptTokens:     max(0, sample.PromptTokens),
		CompletionTokens: max(0, sample.CompletionTokens),
		Cost:             sample.Cost,
		CostKnown:        sample.CostKnown && sample.Cost >= 0 && !sample.Unmetered,
		FirstByte:        max(0, sample.FirstByte),
		Total:            max(0, sample.Total),
		Failed:           failed,
	}
}
