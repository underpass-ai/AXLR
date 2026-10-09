package application

import (
	"context"
	"sync/atomic"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// SessionUsagePort keeps each session's usage ledger. UpdateUsage applies
// one change to the saved ledger atomically, because a turn records its
// requests while the person may raise the budget from /usage.
type SessionUsagePort interface {
	LoadUsage(ctx context.Context, id domain.SessionID) (domain.UsageLedger, error)
	UpdateUsage(ctx context.Context, id domain.SessionID, update func(*domain.UsageLedger)) (domain.UsageLedger, error)
}

// budgetRefusal refuses the next request of a session whose known cost
// reached its limit. Each session keeps its own ledger and limit: a repair
// or plan worker session counts its own requests, not its parent's. A
// ledger that cannot be read refuses too, since the limit cannot be kept.
func (u ContinueTurnUseCase) budgetRefusal(ctx context.Context, session domain.Session) error {
	if u.Usage == nil || u.MaxSessionUSD <= 0 {
		return nil
	}
	ledger, err := u.Usage.LoadUsage(ctx, session.Export().ID)
	if err != nil {
		return err
	}
	if ledger.Exhausted(u.MaxSessionUSD) {
		return &domain.SessionBudgetError{Limit: ledger.Limit(u.MaxSessionUSD), Spent: ledger.Cost, Raise: u.MaxSessionUSD}
	}
	return nil
}

// usageMeter times one model request: to the provider's first sign, its
// activity or its first text, whichever comes first, and to its end.
type usageMeter struct {
	started time.Time
	first   atomic.Int64
}

func (m *usageMeter) mark() {
	m.first.CompareAndSwap(0, max(1, int64(time.Since(m.started))))
}

// watch marks the provider's first activity and passes it on to the
// observer already in ctx.
func (m *usageMeter) watch(ctx context.Context) context.Context {
	next, _ := ctx.Value(providerActivityKey{}).(ProviderActivityObserver)
	return WithProviderActivity(ctx, func(phase domain.ProviderPhase) {
		m.mark()
		if next != nil {
			next(phase)
		}
	})
}

// recordUsage adds a finished request to the session's ledger and tells the
// console the new totals, and once that they passed 80 % of the budget. A
// ledger that cannot be saved loses one request's numbers, never the turn.
func (u ContinueTurnUseCase) recordUsage(ctx context.Context, session domain.Session, model root.ModelID, result root.CompletionResult, meter *usageMeter, emit func(Event) error) {
	if u.Usage == nil {
		return
	}
	sample := domain.UsageSample{Provider: result.Provider, FirstByte: time.Duration(meter.first.Load()), Total: time.Since(meter.started)}
	if sample.Provider == "" {
		sample.Provider = string(model)
	}
	if usage := result.Usage; usage != nil {
		sample.PromptTokens, sample.CachedTokens, sample.CacheWriteTokens = usage.PromptTokens, usage.CachedTokens, usage.CacheWriteTokens
		sample.CompletionTokens, sample.ReasoningTokens = usage.CompletionTokens, usage.ReasoningTokens
		sample.Cost, sample.CostKnown = usage.Cost, usage.CostKnown
	}
	warned := false
	ledger, err := u.Usage.UpdateUsage(context.WithoutCancel(ctx), session.Export().ID, func(l *domain.UsageLedger) { warned = l.Record(sample, u.MaxSessionUSD) })
	if err == nil {
		_ = emit(Event{Kind: EventUsage, SessionUsage: &ledger, BudgetWarning: warned})
	}
}
