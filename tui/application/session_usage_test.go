package application

import (
	"context"
	"errors"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type usageLedgers map[domain.SessionID]domain.UsageLedger

func (u usageLedgers) LoadUsage(_ context.Context, id domain.SessionID) (domain.UsageLedger, error) {
	return u[id], nil
}

func (u usageLedgers) UpdateUsage(_ context.Context, id domain.SessionID, update func(*domain.UsageLedger)) (domain.UsageLedger, error) {
	ledger := u[id]
	update(&ledger)
	u[id] = ledger
	return ledger, nil
}

func pricedAnswer(provider string, cost float64, known bool) streamFunc {
	return func(ctx context.Context, _ root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
		NotifyProviderActivity(ctx, domain.ProviderReasoning)
		time.Sleep(time.Millisecond)
		if err := onText("done"); err != nil {
			return root.CompletionResult{}, err
		}
		result := assistant("done")
		result.Provider = provider
		result.Usage = &root.TokenUsage{PromptTokens: 7448, CachedTokens: 100, CacheWriteTokens: 50, CompletionTokens: 2567, ReasoningTokens: 2590, Cost: cost, CostKnown: known}
		return result, nil
	}
}

// Each answered request lands in the session's ledger with who served it,
// its tokens, its cost when known and its timing, and the console is told
// the new totals; the activity still reaches the console as before.
func TestContinueTurnRecordsEachRequestInTheSessionLedger(t *testing.T) {
	s := turnSession(t)
	id := s.Export().ID
	store, ledgers := &memoryStore{}, usageLedgers{}
	var usage []domain.UsageLedger
	activity := 0
	emit := func(e Event) error {
		if e.Kind == EventUsage {
			usage = append(usage, *e.SessionUsage)
		}
		if e.Kind == EventProviderActivity {
			activity++
		}
		return nil
	}
	for _, model := range []streamFunc{pricedAnswer("Relace", 0.00158142, true), pricedAnswer("", 0, false)} {
		u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: model, Store: store, Usage: ledgers}}
		if err := u.Execute(context.Background(), &s, "hello", emit); err != nil {
			t.Fatal(err)
		}
	}
	l := ledgers[id]
	if l.Requests != 2 || l.PromptTokens != 2*7448 || l.CachedTokens != 200 || l.CacheWriteTokens != 100 || l.CompletionTokens != 2*2567 || l.ReasoningTokens != 2*2590 {
		t.Fatalf("totals %+v", l)
	}
	// A server that names no provider is listed by the model it serves.
	if l.Cost != 0.00158142 || l.CostRequests != 1 || l.Providers["Relace"].Cost != 0.00158142 || l.Providers["test/model"].Requests != 1 || l.Providers["test/model"].CostRequests != 0 {
		t.Fatalf("cost %+v", l)
	}
	if l.FirstByte.Count != 2 || l.Total.Count != 2 || l.FirstByte.Max <= 0 || l.FirstByte.Max > l.Total.Max {
		t.Fatalf("latency first %+v total %+v", l.FirstByte, l.Total)
	}
	if len(usage) != 2 || usage[1].Requests != 2 || activity != 2 {
		t.Fatalf("usage events %d (last %+v), activity events %d", len(usage), usage[len(usage)-1], activity)
	}
}

// A session whose known cost reached max_session_usd sends no request: the
// turn is interrupted with the typed error, which carries the limit, the
// spend and the raise, and goes on once the person raises the limit.
func TestContinueTurnRefusesARequestOnceTheSessionBudgetIsSpent(t *testing.T) {
	s := turnSession(t)
	id := s.Export().ID
	store := &memoryStore{}
	ledgers := usageLedgers{id: {Requests: 3, Cost: 1.25, CostRequests: 3}}
	calls := 0
	model := streamFunc(func(ctx context.Context, req root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
		calls++
		return pricedAnswer("Relace", 0.01, true)(ctx, req, onText)
	})
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: model, Store: store, Usage: ledgers, MaxSessionUSD: 1.25}}
	err := u.Execute(context.Background(), &s, "hello", ignoreEvent)
	var budget *domain.SessionBudgetError
	if !errors.As(err, &budget) || budget.Limit != 1.25 || budget.Spent != 1.25 || budget.Raise != 1.25 {
		t.Fatalf("error %v", err)
	}
	if calls != 0 || s.Status() != domain.StatusInterrupted || store.states[len(store.states)-1].Status != domain.StatusInterrupted {
		t.Fatalf("calls %d, status %s", calls, s.Status())
	}
	if _, err := ledgers.UpdateUsage(context.Background(), id, func(l *domain.UsageLedger) { l.Raise(1.25) }); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, "go on", ignoreEvent); err != nil || calls != 1 || ledgers[id].Cost != 1.26 {
		t.Fatalf("after the raise: %v, calls %d, cost %v", err, calls, ledgers[id].Cost)
	}
	// Without max_session_usd nothing is refused, whatever was spent.
	u.Continue.MaxSessionUSD = 0
	ledgers[id] = domain.UsageLedger{Cost: 1000}
	if err := u.Execute(context.Background(), &s, "again", ignoreEvent); err != nil || calls != 2 {
		t.Fatalf("no limit: %v, calls %d", err, calls)
	}
}

// Past 80 % of the budget the console is warned once, on the request that
// crossed it, not on every request after it.
func TestTheBudgetWarningComesOnce(t *testing.T) {
	s := turnSession(t)
	store, ledgers := &memoryStore{}, usageLedgers{}
	var warnings []int
	request := 0
	emit := func(e Event) error {
		if e.Kind == EventUsage {
			request++
			if e.BudgetWarning {
				warnings = append(warnings, request)
			}
		}
		return nil
	}
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: pricedAnswer("Relace", 0.3, true), Store: store, Usage: ledgers, MaxSessionUSD: 1}}
	for range 4 {
		if err := u.Execute(context.Background(), &s, "hello", emit); err != nil {
			t.Fatal(err)
		}
	}
	if len(warnings) != 1 || warnings[0] != 3 {
		t.Fatalf("warnings on requests %v; want [3]", warnings)
	}
	var budget *domain.SessionBudgetError
	if err := u.Execute(context.Background(), &s, "hello", emit); !errors.As(err, &budget) {
		t.Fatalf("fifth request at $%.2f: %v", ledgers[s.Export().ID].Cost, err)
	}
}
