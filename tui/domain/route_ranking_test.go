package domain

import (
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
)

var rankNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func routingOf(t *testing.T, objective RoutingObjective, allowRemote bool, maxContext int, ceiling float64, ids ...string) ExecutionRouting {
	t.Helper()
	models := make([]root.ModelID, 0, len(ids))
	for _, id := range ids {
		models = append(models, root.ModelID(id))
	}
	routing, err := NewExecutionRouting(models, objective, allowRemote, maxContext, ceiling)
	if err != nil {
		t.Fatal(err)
	}
	return routing
}

func ranked(ranking RouteRanking) string {
	names := make([]string, 0, len(ranking.Ranked))
	for _, candidate := range ranking.Ranked {
		names = append(names, string(candidate.Model))
	}
	return strings.Join(names, ",")
}

// observations builds n requests of one model, each latency long, costing
// cost (unknown when negative), failing when failed.
func observations(n int, latency time.Duration, cost float64, failed bool, at time.Time) []ModelObservation {
	out := make([]ModelObservation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, ModelObservation{At: at.Add(time.Duration(i) * time.Second), Provider: "p", PromptTokens: 1000, CompletionTokens: 100, Cost: cost, CostKnown: cost >= 0 && !failed, FirstByte: latency / 4, Total: latency, Failed: failed})
	}
	return out
}

func TestCostPrefersTheLocalModelBeforeAnyData(t *testing.T) {
	routing := routingOf(t, RoutingCost, true, 0, 0, "anthropic/claude-haiku-5.5", "local")
	candidates := []RouteCandidate{
		{Model: "anthropic/claude-haiku-5.5", Reachable: true, Cap: 64000, Tokens: 2000, PromptRate: 1e-6, CompletionRate: 5e-6, RatesKnown: true},
		{Model: "local", Local: true, Reachable: true, Cap: 24000, Tokens: 2000},
	}
	if got := ranked(RankRoutes(routing, candidates, rankNow)); got != "local,anthropic/claude-haiku-5.5" {
		t.Fatalf("cost ranked %s, want the local model first", got)
	}
}

func TestCostRanksAnUnknownPriceLast(t *testing.T) {
	routing := routingOf(t, RoutingCost, true, 0, 0, "mystery", "priced")
	candidates := []RouteCandidate{
		{Model: "mystery", Reachable: true, Tokens: 2000},
		{Model: "priced", Reachable: true, Tokens: 2000, PromptRate: 3e-6, CompletionRate: 15e-6, RatesKnown: true},
	}
	if got := ranked(RankRoutes(routing, candidates, rankNow)); got != "priced,mystery" {
		t.Fatalf("cost ranked %s", got)
	}
}

func TestCostLearnsFromObservedCost(t *testing.T) {
	routing := routingOf(t, RoutingCost, true, 0, 0, "listed-cheap", "observed-cheaper")
	cheaper := NewModelStats(observations(4, 3*time.Second, 0.0001, false, rankNow.Add(-time.Hour)))
	candidates := []RouteCandidate{
		{Model: "listed-cheap", Reachable: true, Tokens: 1000, PromptRate: 1e-6, CompletionRate: 1e-6, RatesKnown: true},
		{Model: "observed-cheaper", Reachable: true, Tokens: 1000, PromptRate: 9e-6, CompletionRate: 9e-6, RatesKnown: true, Stats: cheaper},
	}
	if got := ranked(RankRoutes(routing, candidates, rankNow)); got != "observed-cheaper,listed-cheap" {
		t.Fatalf("cost ranked %s: the observed cost per token must win over the catalog's", got)
	}
}

func TestLatencyPrefersTheObservedFaster(t *testing.T) {
	routing := routingOf(t, RoutingLatency, true, 0, 0, "slow", "fast")
	candidates := []RouteCandidate{
		{Model: "slow", Local: true, Reachable: true, Stats: NewModelStats(observations(5, 40*time.Second, -1, false, rankNow.Add(-time.Hour)))},
		{Model: "fast", Reachable: true, Stats: NewModelStats(observations(5, 3*time.Second, 0.001, false, rankNow.Add(-time.Hour)))},
	}
	if got := ranked(RankRoutes(routing, candidates, rankNow)); got != "fast,slow" {
		t.Fatalf("latency ranked %s", got)
	}
}

func TestQualityFirstKeepsTheGivenOrder(t *testing.T) {
	routing := routingOf(t, RoutingQualityFirst, true, 0, 0, "best", "good", "cheap")
	candidates := []RouteCandidate{
		{Model: "best", Reachable: true, PromptRate: 1e-5, CompletionRate: 1e-4, RatesKnown: true},
		{Model: "good", Reachable: true, PromptRate: 1e-6, CompletionRate: 1e-5, RatesKnown: true},
		{Model: "cheap", Local: true, Reachable: true},
	}
	if got := ranked(RankRoutes(routing, candidates, rankNow)); got != "best,good,cheap" {
		t.Fatalf("quality-first ranked %s", got)
	}
}

func TestRemoteForbiddenNeverRanksARemoteModel(t *testing.T) {
	for _, objective := range []RoutingObjective{RoutingCost, RoutingLatency, RoutingQualityFirst} {
		routing := routingOf(t, objective, false, 0, 0, "remote", "local")
		candidates := []RouteCandidate{
			{Model: "remote", Reachable: true, PromptRate: 0, CompletionRate: 0, RatesKnown: true, Stats: NewModelStats(observations(5, time.Second, 0, false, rankNow.Add(-time.Hour)))},
			{Model: "local", Local: true, Reachable: true, Stats: NewModelStats(observations(3, time.Minute, -1, true, rankNow.Add(-time.Minute)))},
		}
		ranking := RankRoutes(routing, candidates, rankNow)
		if got := ranked(ranking); got != "local" {
			t.Fatalf("%s ranked %s with remote providers forbidden", objective, got)
		}
		if len(ranking.Excluded) != 1 || ranking.Excluded[0].Kind != RouteRemote {
			t.Fatalf("%s excluded %+v", objective, ranking.Excluded)
		}
	}
	// With only remote candidates there is nothing to ask, never the
	// remote one.
	routing := routingOf(t, RoutingCost, false, 0, 0, "remote")
	ranking := RankRoutes(routing, []RouteCandidate{{Model: "remote", Reachable: true}}, rankNow)
	if len(ranking.Ranked) != 0 || !strings.Contains(ranking.Summary(), "does not allow remote providers") {
		t.Fatalf("ranking = %+v, %q", ranking.Ranked, ranking.Summary())
	}
	if _, successor := ranking.SuccessorCap(); successor {
		t.Fatal("a remote exclusion is no window a successor could fix")
	}
}

func TestAWindowTooSmallExcludesTheModel(t *testing.T) {
	routing := routingOf(t, RoutingCost, true, 0, 0, "local", "remote")
	candidates := []RouteCandidate{
		{Model: "local", Local: true, Reachable: true, Cap: 24000, Tokens: 30000},
		{Model: "remote", Reachable: true, Cap: 64000, Tokens: 30000, RatesKnown: true, PromptRate: 1e-6, CompletionRate: 5e-6},
	}
	ranking := RankRoutes(routing, candidates, rankNow)
	if got := ranked(ranking); got != "remote" {
		t.Fatalf("ranked %s: the local window cannot take 30000 tokens", got)
	}
	if ranking.Excluded[0].Kind != RouteWindow || !strings.Contains(ranking.Excluded[0].Reason, "30000 tokens exceed its cap of 24000") {
		t.Fatalf("excluded %+v", ranking.Excluded)
	}
	// Remote forbidden too: no candidate, but a successor's shorter
	// context fits the local window.
	strict := routingOf(t, RoutingCost, false, 0, 0, "local", "remote")
	ranking = RankRoutes(strict, candidates, rankNow)
	if cap, successor := ranking.SuccessorCap(); !successor || cap != 24000 {
		t.Fatalf("successor cap = %d, %v", cap, successor)
	}
}

func TestTheCostCeilingExcludesExpensiveAndUnknownCosts(t *testing.T) {
	routing := routingOf(t, RoutingLatency, true, 0, 0.01, "dear", "unknown", "cheap", "local")
	candidates := []RouteCandidate{
		{Model: "dear", Reachable: true, Tokens: 10000, PromptRate: 15e-6, CompletionRate: 75e-6, RatesKnown: true},
		{Model: "unknown", Reachable: true, Tokens: 10000},
		{Model: "cheap", Reachable: true, Tokens: 10000, PromptRate: 1e-7, CompletionRate: 4e-7, RatesKnown: true},
		{Model: "local", Local: true, Reachable: true, Tokens: 10000},
	}
	ranking := RankRoutes(routing, candidates, rankNow)
	if got := ranked(ranking); got != "cheap,local" && got != "local,cheap" {
		t.Fatalf("ranked %s under a $0.01 ceiling", got)
	}
	kinds := map[root.ModelID]RouteExclusionKind{}
	for _, exclusion := range ranking.Excluded {
		kinds[exclusion.Model] = exclusion.Kind
	}
	if kinds["dear"] != RouteCost || kinds["unknown"] != RouteCost {
		t.Fatalf("excluded %+v", ranking.Excluded)
	}
}

func TestAnUnreachableModelIsExcluded(t *testing.T) {
	routing := routingOf(t, RoutingQualityFirst, true, 0, 0, "openrouter/model", "local")
	ranking := RankRoutes(routing, []RouteCandidate{{Model: "openrouter/model"}, {Model: "local", Local: true, Reachable: true}}, rankNow)
	if got := ranked(ranking); got != "local" || ranking.Excluded[0].Kind != RouteUnreachable {
		t.Fatalf("ranked %s, excluded %+v", got, ranking.Excluded)
	}
}

func TestAFailingModelRanksLastUntilItsCooldownPasses(t *testing.T) {
	routing := routingOf(t, RoutingCost, true, 0, 0, "local", "remote")
	failing := NewModelStats(observations(3, 0, -1, true, rankNow.Add(-time.Minute)))
	candidates := []RouteCandidate{
		{Model: "local", Local: true, Reachable: true, Stats: failing},
		{Model: "remote", Reachable: true, RatesKnown: true, PromptRate: 1e-6, CompletionRate: 5e-6},
	}
	if got := ranked(RankRoutes(routing, candidates, rankNow)); got != "remote,local" {
		t.Fatalf("ranked %s right after three failures", got)
	}
	if got := ranked(RankRoutes(routing, candidates, rankNow.Add(RouteCooldown))); got != "local,remote" {
		t.Fatalf("ranked %s after the cooldown: the local model is tried again", got)
	}
	// One failure among successes does not demote it.
	mixed := NewModelStats(append(observations(5, 2*time.Second, -1, false, rankNow.Add(-time.Hour)), observations(1, 0, -1, true, rankNow.Add(-time.Minute))...))
	candidates[0].Stats = mixed
	if got := ranked(RankRoutes(routing, candidates, rankNow)); got != "local,remote" {
		t.Fatalf("ranked %s after one failure", got)
	}
}

func TestErrorRateWeighsTheScore(t *testing.T) {
	routing := routingOf(t, RoutingLatency, true, 0, 0, "flaky", "steady")
	flaky := NewModelStats(append(observations(5, 2*time.Second, -1, false, rankNow.Add(-2*time.Hour)), append(observations(1, 0, -1, true, rankNow.Add(-90*time.Minute)), append(observations(1, 2*time.Second, -1, false, rankNow.Add(-80*time.Minute)), append(observations(2, 0, -1, true, rankNow.Add(-70*time.Minute)), observations(1, 2*time.Second, -1, false, rankNow.Add(-60*time.Minute))...)...)...)...))
	steady := NewModelStats(observations(8, 2500*time.Millisecond, -1, false, rankNow.Add(-time.Hour)))
	candidates := []RouteCandidate{{Model: "flaky", Reachable: true, Stats: flaky}, {Model: "steady", Reachable: true, Stats: steady}}
	if got := ranked(RankRoutes(routing, candidates, rankNow)); got != "steady,flaky" {
		t.Fatalf("ranked %s: a third of flaky's requests failed", got)
	}
}

func TestModelHistoryIsBoundedAndForgetsOldRequests(t *testing.T) {
	var history ModelHistory
	for i := 0; i < ModelHistorySamples+10; i++ {
		history.Add(ModelObservation{At: rankNow.Add(time.Duration(i) * time.Second), Provider: "p", Total: time.Second})
	}
	if got := len(history.Providers["p"]); got != ModelHistorySamples {
		t.Fatalf("history kept %d observations", got)
	}
	for i := 0; i < ModelHistoryProviders+3; i++ {
		history.Add(ModelObservation{At: rankNow.Add(time.Hour + time.Duration(i)*time.Second), Provider: string(rune('a' + i)), Total: time.Second})
	}
	if got := len(history.Providers); got != ModelHistoryProviders {
		t.Fatalf("history kept %d providers", got)
	}
	if _, kept := history.Providers["p"]; kept {
		t.Fatal("the provider least recently seen was kept")
	}
	if got := history.Stats(rankNow.Add(time.Hour + ModelHistoryAge + time.Minute)).Requests; got != 0 {
		t.Fatalf("stats counted %d requests older than a week", got)
	}
	if got := history.Stats(rankNow.Add(2 * time.Hour)).Requests; got != ModelHistoryProviders {
		t.Fatalf("stats counted %d recent requests", got)
	}
}

func TestObservationOfAnUnmeteredRequestHasNoCost(t *testing.T) {
	observation := ObservationOf(UsageSample{Provider: "p", PromptTokens: 10, Cost: 0.5, CostKnown: true, Unmetered: true}, true, rankNow)
	if observation.CostKnown || !observation.Failed {
		t.Fatalf("observation = %+v", observation)
	}
}
