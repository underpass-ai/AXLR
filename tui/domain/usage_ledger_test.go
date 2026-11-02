package domain

import (
	"fmt"
	"testing"
	"time"
)

func TestUsageLedgerKeepsTotalsSubtotalsAndLatency(t *testing.T) {
	var l UsageLedger
	l.Record(UsageSample{Provider: "Relace", PromptTokens: 7448, CachedTokens: 100, CacheWriteTokens: 50, CompletionTokens: 2567, ReasoningTokens: 2590, Cost: 0.0016, CostKnown: true, FirstByte: time.Second, Total: 4 * time.Second}, 0)
	l.Record(UsageSample{Provider: "local/qwen3.8-27b", PromptTokens: 1000, CompletionTokens: 10, Total: 2 * time.Second}, 0)
	if l.Requests != 2 || l.PromptTokens != 8448 || l.CachedTokens != 100 || l.CacheWriteTokens != 50 || l.CompletionTokens != 2577 || l.ReasoningTokens != 2590 {
		t.Fatalf("totals %+v", l)
	}
	if l.Cost != 0.0016 || l.CostRequests != 1 || l.Providers["Relace"].Cost != 0.0016 || l.Providers["local/qwen3.8-27b"].CostRequests != 0 || l.Providers["local/qwen3.8-27b"].Requests != 1 {
		t.Fatalf("cost %+v", l)
	}
	if l.FirstByte.Count != 1 || l.Total.Count != 2 || l.Total.Average() != 3*time.Second || l.Total.Max != 4*time.Second || (LatencyStats{}).Average() != 0 {
		t.Fatalf("latency %+v %+v", l.FirstByte, l.Total)
	}
	// A negative cost is not money spent: it never lowers the total.
	l.Record(UsageSample{Provider: "Relace", Cost: -1, CostKnown: true}, 0)
	if l.Cost != 0.0016 || l.CostRequests != 1 {
		t.Fatalf("negative cost counted: %+v", l)
	}
	for i := range MaxLedgerProviders + 4 {
		l.Record(UsageSample{Provider: fmt.Sprintf("p%d", i), Cost: 1, CostKnown: true}, 0)
	}
	if len(l.Providers) != MaxLedgerProviders+1 || l.Providers[OtherProvider].Requests != 6 || l.Cost != 20.0016 {
		t.Fatalf("providers not bounded: %d, other %+v, cost %v", len(l.Providers), l.Providers[OtherProvider], l.Cost)
	}
}

// The warning comes once, from the request with a cost that passes 80 % of
// the limit; a request without a cost neither counts nor warns, and a raise
// allows another max_session_usd and arms the warning again.
func TestUsageLedgerBudget(t *testing.T) {
	var l UsageLedger
	if l.Limit(0) != 0 || l.Exhausted(0) || l.Near(0) {
		t.Fatal("no max_session_usd is no limit")
	}
	warnings := 0
	for _, cost := range []float64{0.5, 0.29} {
		if l.Record(UsageSample{Cost: cost, CostKnown: true}, 1) {
			warnings++
		}
	}
	l.Cost = 0.79
	if l.Record(UsageSample{Cost: 0.5}, 1) || l.Near(1) || warnings != 0 {
		t.Fatalf("a request without a cost warned or counted: %+v", l)
	}
	if !l.Record(UsageSample{Cost: 0.02, CostKnown: true}, 1) || l.Record(UsageSample{Cost: 0.1, CostKnown: true}, 1) || !l.Warned {
		t.Fatalf("warning not given exactly once: %+v", l)
	}
	l.Record(UsageSample{Cost: 0.2, CostKnown: true}, 1)
	if !l.Exhausted(1) || l.Limit(1) != 1 {
		t.Fatalf("not exhausted at %v", l.Cost)
	}
	l.Raise(1)
	if l.Exhausted(1) || l.Limit(1) != 2 || l.Warned || l.Raised != 1 {
		t.Fatalf("raise: %+v", l)
	}
	l.Raise(0)
	if l.Raised != 1 {
		t.Fatal("a raise without max_session_usd changed the limit")
	}
	if got := (&SessionBudgetError{Limit: 2, Spent: 2.04, Raise: 1}).Error(); got != "this session reached its $2.00 budget (max_session_usd) with $2.04 spent" {
		t.Fatal(got)
	}
}

// An unmetered request counts as a request with estimated tokens, adds no
// cost and never warns; the ledger says how many there were.
func TestUsageLedgerCountsUnmeteredRequests(t *testing.T) {
	var l UsageLedger
	if warned := l.Record(UsageSample{Provider: "Anthropic", PromptTokens: EstimatedTokens(24400, 0), CompletionTokens: EstimatedTokens(4000, 400), Unmetered: true}, 0.01); warned {
		t.Fatal("an unmetered request warned")
	}
	if l.Requests != 1 || l.Unmetered != 1 || l.PromptTokens != 10000 || l.CompletionTokens != 1000 || l.Cost != 0 || l.CostRequests != 0 {
		t.Fatalf("ledger %+v", l)
	}
	if EstimatedTokens(0, 244) != 0 || EstimatedTokens(1, 244) != 1 || EstimatedTokens(3, 244) != 2 {
		t.Fatal("estimates do not round up")
	}
}

// An unmetered OpenRouter request waits in Pending until its exact cost
// replaces the estimate: its tokens take the estimated ones' place, its
// cost joins the known cost and the warning, and it is no longer
// unmetered. One never settled is given up after its lookups or its wait,
// keeping its estimate.
func TestUsageLedgerSettlesAnUnmeteredRequest(t *testing.T) {
	ended := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	var l UsageLedger
	l.Record(UsageSample{Provider: "Anthropic", PromptTokens: 10000, CompletionTokens: 159, Unmetered: true, GenerationID: "gen-1", Ended: ended}, 1)
	l.Record(UsageSample{Provider: "Anthropic", PromptTokens: 500, CompletionTokens: 0, Unmetered: true, GenerationID: "gen-2", Ended: ended}, 1)
	l.Record(UsageSample{Provider: "Relace", PromptTokens: 100, Unmetered: true}, 1)
	if l.Unmetered != 3 || len(l.Pending) != 2 || l.Pending[0].Provider != "Anthropic" || l.Pending[0].PromptTokens != 10000 {
		t.Fatalf("pending %+v", l)
	}
	shown := l
	if settled, warned := l.Settle("gen-1", GenerationCost{Cost: 0.85, PromptTokens: 9800, CompletionTokens: 900, TokensKnown: true}, 1); !settled || !warned {
		t.Fatalf("settled %v, warned %v", settled, warned)
	}
	if l.Unmetered != 2 || l.Cost != 0.85 || l.CostRequests != 1 || l.PromptTokens != 10400 || l.CompletionTokens != 900 || len(l.Pending) != 1 || l.Pending[0].ID != "gen-2" {
		t.Fatalf("after settling %+v", l)
	}
	if p := l.Providers["Anthropic"]; p.Cost != 0.85 || p.CostRequests != 1 || p.PromptTokens != 10300 || p.CompletionTokens != 900 || p.Requests != 2 {
		t.Fatalf("provider %+v", p)
	}
	if len(shown.Pending) != 2 || shown.Pending[0].ID != "gen-1" {
		t.Fatalf("a copy of the ledger changed: %+v", shown.Pending)
	}
	if settled, _ := l.Settle("gen-1", GenerationCost{Cost: 1}, 1); settled {
		t.Fatal("a request was settled twice")
	}
	if settled, _ := l.Settle("gen-2", GenerationCost{Cost: -1}, 1); settled {
		t.Fatal("a negative cost settled a request")
	}
	now := ended.Add(time.Minute)
	if !l.Pending[0].Due(now) {
		t.Fatal("a request never asked for is not due")
	}
	for range MaxGenerationLookups - 1 {
		l.Missed("gen-2", now)
		if l.Pending[0].Due(now) || !l.Pending[0].Due(now.Add(GenerationLookupSpacing)) {
			t.Fatalf("spacing %+v", l.Pending[0])
		}
		now = now.Add(GenerationLookupSpacing)
	}
	l.Missed("gen-2", now)
	if len(l.Pending) != 0 || l.Unmetered != 2 || l.PromptTokens != 10400 {
		t.Fatalf("given up %+v", l)
	}
	l.Record(UsageSample{Provider: "Anthropic", Unmetered: true, GenerationID: "gen-3", Ended: ended}, 1)
	l.Expire(ended.Add(MaxGenerationWait))
	if len(l.Pending) != 0 {
		t.Fatalf("an old request stayed pending: %+v", l.Pending)
	}
	for i := range MaxPendingGenerations + 2 {
		l.Record(UsageSample{Unmetered: true, GenerationID: fmt.Sprintf("gen-x%d", i), Ended: ended}, 0)
	}
	if len(l.Pending) != MaxPendingGenerations {
		t.Fatalf("pending not bounded: %d", len(l.Pending))
	}
}

// An unmetered request with a known rate counts at its estimate towards
// the limit and the warning, apart from the known cost; settling it swaps
// the estimate for the exact cost. Without a rate, or with a negative
// estimate, it counts nothing.
func TestUsageLedgerCountsAnEstimateTowardsTheLimit(t *testing.T) {
	var l UsageLedger
	if l.Record(UsageSample{Unmetered: true, EstimatedCost: 0.5}, 1) || l.Record(UsageSample{Unmetered: true, EstimatedCost: -1, EstimateKnown: true}, 1) || l.Counted() != 0 || l.Estimated != 0 {
		t.Fatalf("an unknown or negative estimate counted: %+v", l)
	}
	if !l.Record(UsageSample{Unmetered: true, GenerationID: "gen-1", EstimatedCost: 0.85, EstimateKnown: true}, 1) || l.Cost != 0 || l.Counted() != 0.85 || l.Estimated != 1 || !l.Pending[0].Estimated {
		t.Fatalf("estimate not counted: %+v", l)
	}
	l.Record(UsageSample{Cost: 0.2, CostKnown: true}, 1)
	if !l.Exhausted(1) {
		t.Fatalf("not exhausted at %v", l.Counted())
	}
	if settled, _ := l.Settle("gen-1", GenerationCost{Cost: 0.3}, 1); !settled || l.EstimatedCost != 0 || l.Estimated != 0 || l.Cost != 0.5 || l.Exhausted(1) {
		t.Fatalf("settled %+v", l)
	}
}
