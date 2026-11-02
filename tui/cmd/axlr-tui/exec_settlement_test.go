package main

import (
	"context"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type execGenerationCosts map[string]domain.GenerationCost

func (c execGenerationCosts) GenerationCost(_ context.Context, id string) (domain.GenerationCost, error) {
	if cost, ok := c[id]; ok {
		return cost, nil
	}
	return domain.GenerationCost{}, application.ErrGenerationPending
}

// Before a run reports, the exact cost of its cancelled requests is asked
// for, since the process ends with the run; a cancelled run asks once.
func TestExecSettlesItsUnmeteredRequestsBeforeItReports(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const id = domain.SessionID("0123456789abcdef0123456789abcdef")
	if _, err := store.UpdateUsage(context.Background(), id, func(l *domain.UsageLedger) {
		l.Record(domain.UsageSample{Provider: "Anthropic", PromptTokens: 100, Unmetered: true, GenerationID: "gen-1", Ended: time.Now()}, 0)
		l.Record(domain.UsageSample{Provider: "Anthropic", PromptTokens: 100, Unmetered: true, GenerationID: "gen-2", Ended: time.Now()}, 0)
	}); err != nil {
		t.Fatal(err)
	}
	continuation := application.ContinueTurnUseCase{Usage: store, Generations: execGenerationCosts{"gen-1": {Cost: 0.25}}}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	settleExecUsage(cancelled, continuation, []domain.SessionID{id})
	ledger, err := store.LoadUsage(context.Background(), id)
	if err != nil || ledger.Cost != 0.25 || ledger.Unmetered != 1 || len(ledger.Pending) != 1 || ledger.Pending[0].Lookups != 1 {
		t.Fatalf("ledger %+v %v", ledger, err)
	}
	continuation.Generations = execGenerationCosts{"gen-2": {Cost: 0.5}}
	settleExecUsage(context.Background(), continuation, []domain.SessionID{id})
	if ledger, err = store.LoadUsage(context.Background(), id); err != nil || ledger.Cost != 0.75 || ledger.Unmetered != 0 || ledger.Pending != nil {
		t.Fatalf("ledger %+v %v", ledger, err)
	}
}
