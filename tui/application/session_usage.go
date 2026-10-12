package application

import (
	"context"
	"errors"
	"sync"
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
// activity or its first text, whichever comes first, and to its end. It
// also counts the bytes the request streamed, text and tool-call
// arguments, which size a request that ends without a usage report.
type usageMeter struct {
	started time.Time
	first   atomic.Int64

	mu        sync.Mutex
	text      int
	toolDone  int
	toolName  string
	toolBytes int
}

func (m *usageMeter) addText(bytes int) {
	m.mu.Lock()
	m.text += bytes
	m.mu.Unlock()
}

// toolProgress takes a tool call's cumulative argument bytes; a new call,
// or a count that went back, closes the previous one.
func (m *usageMeter) toolProgress(name string, bytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name != m.toolName || bytes < m.toolBytes {
		m.toolDone += m.toolBytes
	}
	m.toolName, m.toolBytes = name, bytes
}

func (m *usageMeter) streamed() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text + m.toolDone + m.toolBytes
}

func (m *usageMeter) mark() {
	m.first.CompareAndSwap(0, max(1, int64(time.Since(m.started))))
}

// watch marks the provider's first activity and counts the tool-call
// bytes, and passes both on to the observers already in ctx.
func (m *usageMeter) watch(ctx context.Context) context.Context {
	next, _ := ctx.Value(providerActivityKey{}).(ProviderActivityObserver)
	ctx = WithProviderActivity(ctx, func(phase domain.ProviderPhase) {
		m.mark()
		if next != nil {
			next(phase)
		}
	})
	progress, _ := ctx.Value(toolCallProgressKey{}).(ToolCallProgressObserver)
	return WithToolCallProgress(ctx, func(name string, bytes int) {
		m.toolProgress(name, bytes)
		if progress != nil {
			progress(name, bytes)
		}
	})
}

// bytesPerToken is the ratio a calibration that knows one applies to a
// model; the console's TokenCalibration does.
type bytesPerToken interface {
	BytesPerToken(root.ModelID) (int, bool)
}

// recordUsage adds a request to the session's ledger, whether it finished,
// failed or was cancelled, and tells the console the new totals, and once
// that they passed 80 % of the budget. A request that ended without a usage
// report is unmetered when the provider may have billed it: it streamed
// something, or it was cancelled or timed out once sent. Its prompt tokens
// are estimated from the body's size at the model's bytes per token, its
// completion tokens from the bytes it streamed. A request the provider
// refused before answering counts as a request and nothing more. A ledger
// that cannot be saved loses one request's numbers, never the turn.
func (u ContinueTurnUseCase) recordUsage(ctx context.Context, session domain.Session, model root.ModelID, result root.CompletionResult, streamErr error, meter *usageMeter, emit func(Event) error) {
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
	} else if streamErr != nil {
		streamed := meter.streamed()
		stopped := errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, context.DeadlineExceeded)
		if streamed > 0 || sample.FirstByte > 0 || (stopped && result.RequestBytes > 0) {
			ratio := 0
			if calibration, ok := u.Calibration.(bytesPerToken); ok {
				ratio, _ = calibration.BytesPerToken(model)
			}
			sample.Unmetered = true
			sample.PromptTokens = domain.EstimatedTokens(result.RequestBytes, ratio)
			sample.CompletionTokens = domain.EstimatedTokens(streamed, ratio)
		}
	}
	warned := false
	ledger, err := u.Usage.UpdateUsage(context.WithoutCancel(ctx), session.Export().ID, func(l *domain.UsageLedger) { warned = l.Record(sample, u.MaxSessionUSD) })
	if err == nil {
		_ = emit(Event{Kind: EventUsage, SessionUsage: &ledger, BudgetWarning: warned})
	}
}
