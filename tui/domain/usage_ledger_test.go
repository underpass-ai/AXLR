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
