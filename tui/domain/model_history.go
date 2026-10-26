package domain

import (
	"sort"
	"time"
)

// ModelHistory is a model's recent requests, by provider: a rolling
// window of at most ModelHistorySamples per provider and
// ModelHistoryProviders providers, the oldest dropped first, so its size
// does not grow with use. What it says is read through Stats.
type ModelHistory struct {
	Providers map[string][]ModelObservation
}

const (
	ModelHistorySamples   = 32
	ModelHistoryProviders = 8
	// ModelHistoryAge is how long an observation counts: a provider's
	// prices and speed change, and a week-old failure says little.
	ModelHistoryAge = 7 * 24 * time.Hour
)

// Add records one request; the provider past the bound replaces the one
// least recently seen.
func (h *ModelHistory) Add(observation ModelObservation) {
	if h.Providers == nil {
		h.Providers = map[string][]ModelObservation{}
	}
	name := observation.Provider
	if _, seen := h.Providers[name]; !seen && len(h.Providers) >= ModelHistoryProviders {
		oldest, at := "", time.Time{}
		for provider, observations := range h.Providers {
			last := observations[len(observations)-1].At
			if oldest == "" || last.Before(at) {
				oldest, at = provider, last
			}
		}
		delete(h.Providers, oldest)
	}
	window := append(h.Providers[name], observation)
	if len(window) > ModelHistorySamples {
		window = append([]ModelObservation(nil), window[len(window)-ModelHistorySamples:]...)
	}
	h.Providers[name] = window
}

// Updated is when the model was last observed, zero for never.
func (h ModelHistory) Updated() time.Time {
	var latest time.Time
	for _, observations := range h.Providers {
		if last := observations[len(observations)-1].At; last.After(latest) {
			latest = last
		}
	}
	return latest
}

// Stats aggregates the model's observations younger than ModelHistoryAge
// at now, across its providers, in time order.
func (h ModelHistory) Stats(now time.Time) ModelStats {
	var recent []ModelObservation
	for _, observations := range h.Providers {
		for _, observation := range observations {
			if now.Sub(observation.At) < ModelHistoryAge {
				recent = append(recent, observation)
			}
		}
	}
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].At.Before(recent[j].At) })
	return NewModelStats(recent)
}
