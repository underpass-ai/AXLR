package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// RouteRanking is the order in which a request asks an execution's
// candidates: Ranked, best first, are the ones its constraints allow; the
// first takes the request and, if it fails, the next takes it once more.
// Excluded are the others, with the constraint that kept each out.
type RouteRanking struct {
	Ranked   []RouteCandidate
	Excluded []RouteExclusion
}

// Priors of the ranking, for what a model's requests have not yet said.
const (
	// RoutePriorCompletionTokens sizes an answer of a model never seen.
	RoutePriorCompletionTokens = 512
	// RoutePriorTotal and RoutePriorFirstByte are a request's expected
	// latency before any; each observation weighs as much as the prior.
	RoutePriorTotal     = 10 * time.Second
	RoutePriorFirstByte = 2 * time.Second
	// The error rate starts at one in twenty.
	routePriorFailures = 1
	routePriorRequests = 20
	// RouteUnhealthyFailures failures in a row, the latest within
	// RouteCooldown, rank a model last.
	RouteUnhealthyFailures = 3
	RouteCooldown          = 5 * time.Minute
	// routeMaxErrorRate keeps a score finite.
	routeMaxErrorRate = 0.95
)

// RankRoutes ranks the candidates of routing for one request, at now.
// The constraints come first and are absolute: a candidate no client
// reaches, one off this host when remote providers are not allowed, one
// whose cap the request exceeds, one expected to cost more than
// max_request_usd (or whose cost cannot be told under it) is excluded
// and never asked. The objective orders the rest:
//
//   - cost: the request's expected dollars, per successful request (its
//     error rate counted): a model this host serves costs nothing; a
//     model's observed cost per token, else its catalog price, sizes the
//     rest; a cost no one can tell ranks after every known one. Ties go
//     to the faster.
//   - latency: the expected time to the answer's end, per successful
//     request, from the model's observed latency smoothed towards a prior.
//     Ties go to the cheaper.
//   - quality-first: the order the routing gives.
//
// A model whose last requests all failed, recently, ranks after every
// healthy one; the order given breaks every remaining tie.
func RankRoutes(routing ExecutionRouting, candidates []RouteCandidate, now time.Time) RouteRanking {
	var ranking RouteRanking
	type scored struct {
		candidate         RouteCandidate
		unhealthy         bool
		primary, tiebreak float64
		order             int
	}
	var eligible []scored
	for order, candidate := range candidates {
		if exclusion, excluded := routeExclusion(routing, candidate); excluded {
			ranking.Excluded = append(ranking.Excluded, exclusion)
			continue
		}
		cost, known := expectedCost(candidate)
		if !known {
			cost = math.Inf(1)
		}
		latency := expectedLatency(candidate.Stats).Seconds()
		penalty := 1 - min(candidate.Stats.ErrorRate(), routeMaxErrorRate)
		entry := scored{candidate: candidate, unhealthy: candidate.Stats.Unhealthy(now), order: order}
		switch routing.Objective() {
		case RoutingCost:
			entry.primary, entry.tiebreak = cost/penalty, latency/penalty
		case RoutingLatency:
			entry.primary, entry.tiebreak = latency/penalty, cost/penalty
		}
		eligible = append(eligible, entry)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		switch {
		case a.unhealthy != b.unhealthy:
			return !a.unhealthy
		case a.primary != b.primary:
			return a.primary < b.primary
		case a.tiebreak != b.tiebreak:
			return a.tiebreak < b.tiebreak
		}
		return a.order < b.order
	})
	for _, entry := range eligible {
		ranking.Ranked = append(ranking.Ranked, entry.candidate)
	}
	return ranking
}

// routeExclusion is the first constraint that keeps candidate out.
func routeExclusion(routing ExecutionRouting, candidate RouteCandidate) (RouteExclusion, bool) {
	exclusion := RouteExclusion{Model: candidate.Model}
	switch {
	case !candidate.Reachable:
		exclusion.Kind, exclusion.Reason = RouteUnreachable, "no client reaches it: it is not a local model and OPENROUTER_API_KEY is not set"
	case !candidate.Local && !routing.AllowsRemote():
		exclusion.Kind, exclusion.Reason = RouteRemote, "it runs off this host and the routing does not allow remote providers"
	case candidate.Cap > 0 && candidate.Tokens > candidate.Cap:
		exclusion.Kind, exclusion.Reason, exclusion.Cap = RouteWindow, fmt.Sprintf("the request's %d tokens exceed its cap of %d", candidate.Tokens, candidate.Cap), candidate.Cap
	case routing.MaxRequestUSD() > 0:
		cost, known := expectedCost(candidate)
		switch {
		case !known:
			exclusion.Kind, exclusion.Reason = RouteCost, fmt.Sprintf("its cost is unknown (no catalog price, no request that reported one) and max_request_usd is $%s", usd(routing.MaxRequestUSD()))
		case cost > routing.MaxRequestUSD():
			exclusion.Kind, exclusion.Reason = RouteCost, fmt.Sprintf("the request is expected to cost $%s, over max_request_usd $%s", usd(cost), usd(routing.MaxRequestUSD()))
		default:
			return RouteExclusion{}, false
		}
	default:
		return RouteExclusion{}, false
	}
	return exclusion, true
}

// expectedCost is what the request is expected to cost on candidate, in
// dollars: its observed cost per token, a model this host serves at
// nothing, its catalog price; false when none tells.
func expectedCost(candidate RouteCandidate) (float64, bool) {
	completion, seen := candidate.Stats.MeanCompletion()
	if !seen {
		completion = RoutePriorCompletionTokens
	}
	tokens := max(0, candidate.Tokens)
	if perToken, observed := candidate.Stats.CostPerToken(); observed {
		return perToken * float64(tokens+completion), true
	}
	if candidate.Local {
		return 0, true
	}
	if candidate.RatesKnown {
		return candidate.PromptRate*float64(tokens) + candidate.CompletionRate*float64(completion), true
	}
	return 0, false
}

// expectedLatency is a request's expected duration on a model: its
// observed mean, smoothed towards RoutePriorTotal by one observation's
// weight, with the time to the first byte likewise added as a fraction.
func expectedLatency(stats ModelStats) time.Duration {
	total := (RoutePriorTotal + stats.Total.Sum) / time.Duration(1+stats.Total.Count)
	first := (RoutePriorFirstByte + stats.FirstByte.Sum) / time.Duration(1+stats.FirstByte.Count)
	return total + first/10
}

// Summary says in one line why the ranking excluded what it excluded.
func (r RouteRanking) Summary() string {
	parts := make([]string, 0, len(r.Excluded))
	for _, exclusion := range r.Excluded {
		parts = append(parts, fmt.Sprintf("%s: %s", exclusion.Model, exclusion.Reason))
	}
	return strings.Join(parts, "; ")
}

// SuccessorCap reports a ranking that left no candidate while some only
// because the request exceeds their caps: a shorter context, a successor
// session's, would fit. It returns the largest of those caps.
func (r RouteRanking) SuccessorCap() (int, bool) {
	if len(r.Ranked) > 0 {
		return 0, false
	}
	largest, windows := 0, false
	for _, exclusion := range r.Excluded {
		if exclusion.Kind == RouteWindow {
			windows = true
			largest = max(largest, exclusion.Cap)
		}
	}
	return largest, windows
}

// usd writes dollars with the precision a small request needs.
func usd(amount float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", amount), "0"), ".")
}
