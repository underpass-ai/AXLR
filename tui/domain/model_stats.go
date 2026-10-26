package domain

import "time"

// ModelStats is what a model's recent requests say about it: how many
// failed, what a token cost, how long the answers took.
type ModelStats struct {
	Requests int
	Failures int
	// TrailingFailures counts the failures since the last success, the
	// latest at LastFailure.
	TrailingFailures int
	LastFailure      time.Time
	// CostRequests reported a cost: Cost dollars for CostTokens tokens,
	// prompt and completion.
	CostRequests int
	Cost         float64
	CostTokens   int
	// Completed requests: their completion tokens and latencies.
	Completed        int
	CompletionTokens int
	FirstByte        LatencyStats
	Total            LatencyStats
}

// NewModelStats aggregates observations given in time order.
func NewModelStats(observations []ModelObservation) ModelStats {
	var stats ModelStats
	for _, observation := range observations {
		stats.Requests++
		if observation.Failed {
			stats.Failures++
			stats.TrailingFailures++
			stats.LastFailure = observation.At
			continue
		}
		stats.TrailingFailures = 0
		stats.Completed++
		stats.CompletionTokens += observation.CompletionTokens
		stats.FirstByte.add(observation.FirstByte)
		stats.Total.add(observation.Total)
		if observation.CostKnown {
			stats.CostRequests++
			stats.Cost += observation.Cost
			stats.CostTokens += observation.PromptTokens + observation.CompletionTokens
		}
	}
	return stats
}

// CostPerToken is the dollars a token cost in the requests that reported
// a cost, false when none did.
func (s ModelStats) CostPerToken() (float64, bool) {
	if s.CostRequests == 0 || s.CostTokens == 0 {
		return 0, false
	}
	return s.Cost / float64(s.CostTokens), true
}

// MeanCompletion is the completion tokens of an answer, false before any.
func (s ModelStats) MeanCompletion() (int, bool) {
	if s.Completed == 0 {
		return 0, false
	}
	return s.CompletionTokens / s.Completed, true
}

// ErrorRate is the share of requests the provider failed, smoothed
// towards one in twenty: a model never seen is neither trusted nor
// distrusted, and one failure among few requests does not condemn it.
func (s ModelStats) ErrorRate() float64 {
	return (float64(s.Failures) + routePriorFailures) / (float64(s.Requests) + routePriorRequests)
}

// Unhealthy reports a model whose last requests all failed, the latest
// recently: it is asked last until RouteCooldown has passed, then tried
// again.
func (s ModelStats) Unhealthy(now time.Time) bool {
	return s.TrailingFailures >= RouteUnhealthyFailures && now.Sub(s.LastFailure) < RouteCooldown
}
