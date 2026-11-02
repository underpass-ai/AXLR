package application

import (
	"context"
	"errors"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// generationCosts answers lookups from a table; an id it lacks is not
// known yet. It counts the lookups by id.
type generationCosts struct {
	costs  map[string]domain.GenerationCost
	errs   map[string]error
	asked  map[string]int
	before func(context.Context)
}

func (g *generationCosts) GenerationCost(ctx context.Context, id string) (domain.GenerationCost, error) {
	if g.asked == nil {
		g.asked = map[string]int{}
	}
	g.asked[id]++
	if g.before != nil {
		g.before(ctx)
	}
	if err := g.errs[id]; err != nil {
		return domain.GenerationCost{}, err
	}
	cost, ok := g.costs[id]
	if !ok {
		return domain.GenerationCost{}, ErrGenerationPending
	}
	return cost, nil
}

// A cancelled OpenRouter request waits with its estimate and generation
// id; before the session's next request its exact cost replaces the
// estimate, and the console is told the new totals. On 12 October 2026 an
// essay cancelled after ten seconds stayed unmetered for good.
func TestTheNextRequestSettlesACancelledRequestsCost(t *testing.T) {
	s := turnSession(t)
	id := s.Export().ID
	store, ledgers := &memoryStore{}, usageLedgers{}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	model := streamFunc(func(ctx context.Context, req root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
		calls++
		if calls == 1 {
			_ = onText("palabra palabra")
			time.Sleep(2 * time.Millisecond)
			cancel()
			return root.CompletionResult{RequestBytes: 4100, Provider: "Anthropic", GenerationID: "gen-1"}, context.Canceled
		}
		return pricedAnswer("Anthropic", 0.01, true)(ctx, req, onText)
	})
	costs := &generationCosts{}
	receipts := &memoryReceipts{}
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: model, Store: store, Usage: ledgers, Generations: costs, Calibration: pinnedRatio{410}, Receipts: receipts}}
	if err := u.Execute(ctx, &s, "write an essay", ignoreEvent); !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v", err)
	}
	if l := ledgers[id]; l.Unmetered != 1 || len(l.Pending) != 1 || l.Pending[0].ID != "gen-1" || l.Pending[0].PromptTokens != 1000 || costs.asked["gen-1"] != 0 {
		t.Fatalf("cancelled: %+v, asked %v", l, costs.asked)
	}
	// OpenRouter has no stats yet: the miss is counted and the request
	// keeps its estimate.
	if _, changed := u.Continue.SettleUsage(context.Background(), id); !changed || ledgers[id].Pending[0].Lookups != 1 || ledgers[id].Unmetered != 1 {
		t.Fatalf("miss: %+v", ledgers[id])
	}
	// Too soon to ask again.
	if _, changed := u.Continue.SettleUsage(context.Background(), id); changed || costs.asked["gen-1"] != 1 {
		t.Fatalf("asked again at once: %v", costs.asked)
	}
	pending := ledgers[id]
	pending.Pending[0].Tried = time.Now().Add(-domain.GenerationLookupSpacing)
	ledgers[id] = pending
	costs.costs = map[string]domain.GenerationCost{"gen-1": {Cost: 0.2, PromptTokens: 980, CompletionTokens: 600, TokensKnown: true}}
	var usage []domain.UsageLedger
	emit := func(e Event) error {
		if e.Kind == EventUsage {
			usage = append(usage, *e.SessionUsage)
		}
		return nil
	}
	if err := u.Execute(context.Background(), &s, "again", emit); err != nil {
		t.Fatal(err)
	}
	l := ledgers[id]
	if l.Unmetered != 0 || len(l.Pending) != 0 || l.CostRequests != 2 || !sameCost(l.Cost, 0.21) || l.PromptTokens != 980+7448 || l.CompletionTokens != 600+2567 {
		t.Fatalf("settled: %+v", l)
	}
	if len(usage) != 2 || usage[0].Unmetered != 0 || !sameCost(usage[0].Cost, 0.2) {
		t.Fatalf("usage events %+v", usage)
	}
	settled := 0
	for _, entry := range receipts.entries {
		if entry.Kind == domain.ReceiptRequestEnded && entry.Usage.Unmetered && entry.Usage.GenerationID != "gen-1" {
			t.Fatalf("ended without its generation: %+v", entry)
		}
		if entry.Kind == domain.ReceiptRequestSettled && entry.Usage.GenerationID == "gen-1" && entry.Usage.Cost == 0.2 && entry.Usage.CompletionTokens == 600 {
			settled++
		}
	}
	if settled != 1 {
		t.Fatalf("receipt %+v", receipts.entries)
	}
}

func sameCost(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

// A lookup that keeps failing gives the request up after its lookups; a
// request past its wait is given up without one; and the lookups before a
// request stop when their window closes, without charging the request.
func TestSettlementGivesUpAndKeepsToItsWindow(t *testing.T) {
	id := turnSession(t).Export().ID
	ledgers := usageLedgers{}
	ended := time.Now()
	ledgers.UpdateUsage(context.Background(), id, func(l *domain.UsageLedger) {
		l.Record(domain.UsageSample{Provider: "Anthropic", PromptTokens: 100, Unmetered: true, GenerationID: "gen-bad", Ended: ended}, 0)
		l.Record(domain.UsageSample{Provider: "Anthropic", PromptTokens: 100, Unmetered: true, GenerationID: "gen-old", Ended: ended.Add(-domain.MaxGenerationWait)}, 0)
	})
	costs := &generationCosts{errs: map[string]error{"gen-bad": errors.New("OpenRouter generation lookup failed (HTTP 401)")}}
	u := ContinueTurnUseCase{Usage: ledgers, Generations: costs}
	for range domain.MaxGenerationLookups {
		u.SettleUsage(context.Background(), id)
		l := ledgers[id]
		if len(l.Pending) > 0 {
			l.Pending[0].Tried = time.Time{}
			ledgers[id] = l
		}
	}
	if l := ledgers[id]; len(l.Pending) != 0 || l.Unmetered != 2 || costs.asked["gen-old"] != 0 || costs.asked["gen-bad"] != domain.MaxGenerationLookups {
		t.Fatalf("given up: %+v, asked %v", l, costs.asked)
	}
	// The window closes during the first lookup: it is not counted.
	ledgers.UpdateUsage(context.Background(), id, func(l *domain.UsageLedger) {
		l.Record(domain.UsageSample{Unmetered: true, GenerationID: "gen-slow", Ended: time.Now()}, 0)
	})
	ctx, cancel := context.WithCancel(context.Background())
	costs.before = func(context.Context) { cancel() }
	if _, changed := u.SettleUsage(ctx, id); changed || ledgers[id].Pending[0].Lookups != 0 {
		t.Fatalf("a cancelled lookup was counted: %+v", ledgers[id])
	}
	// Nothing to look up without the port.
	if _, changed := (ContinueTurnUseCase{Usage: ledgers}).SettleUsage(context.Background(), id); changed {
		t.Fatal("settled without a port")
	}
}
