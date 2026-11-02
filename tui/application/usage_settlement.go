package application

import (
	"context"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// generationLookupWindow bounds the lookups made before one request: the
// turn waits no longer for them, and what is left is asked at a later one.
const generationLookupWindow = 3 * time.Second

// The exact cost of an unmetered request is settled lazily. A request
// cancelled or failed without a usage report is recorded at once with its
// estimate and, when OpenRouter served it, its generation id in the
// ledger's Pending list; the stream's end is no place to wait, since the
// person just pressed Esc and some providers keep generating, and billing,
// after a cancel. Before each later request's budget check, and once more
// before an --exec run reports, the pending requests that are due are
// looked up within generationLookupWindow; a cost found replaces the
// estimate (UsageLedger.Settle), a miss is counted, and a request past its
// lookups or its wait keeps its estimate and stays unmetered.

// settleUsage settles what it can of the session's pending requests and
// tells the console the new totals when any changed.
func (u ContinueTurnUseCase) settleUsage(ctx context.Context, session domain.Session, emit func(Event) error) {
	if ledger, changed, warned := u.settle(ctx, session.Export().ID); changed {
		_ = emit(Event{Kind: EventUsage, SessionUsage: &ledger, BudgetWarning: warned})
	}
}

// SettleUsage looks up the exact cost of the session's pending requests
// that are due, within a short window, and returns the ledger and whether
// it changed. Without a generation-cost port or a ledger it does nothing.
func (u ContinueTurnUseCase) SettleUsage(ctx context.Context, id domain.SessionID) (domain.UsageLedger, bool) {
	ledger, changed, _ := u.settle(ctx, id)
	return ledger, changed
}

func (u ContinueTurnUseCase) settle(ctx context.Context, id domain.SessionID) (domain.UsageLedger, bool, bool) {
	if u.Usage == nil || u.Generations == nil {
		return domain.UsageLedger{}, false, false
	}
	ledger, err := u.Usage.LoadUsage(ctx, id)
	if err != nil || len(ledger.Pending) == 0 {
		return ledger, false, false
	}
	type lookup struct {
		id   string
		cost domain.GenerationCost
		err  error
	}
	lookupCtx, cancel := context.WithTimeout(ctx, generationLookupWindow)
	defer cancel()
	now := time.Now()
	var lookups []lookup
	expired := false
	for _, pending := range ledger.Pending {
		if pending.Expired(now) {
			expired = true
			continue
		}
		if !pending.Due(now) {
			continue
		}
		cost, err := u.Generations.GenerationCost(lookupCtx, pending.ID)
		if err != nil && lookupCtx.Err() != nil {
			// The window closed, or the turn was cancelled: not a lookup
			// the request is charged with.
			break
		}
		lookups = append(lookups, lookup{id: pending.ID, cost: cost, err: err})
	}
	if len(lookups) == 0 && !expired {
		return ledger, false, false
	}
	warned := false
	var settled []domain.UsageSample
	updated, err := u.Usage.UpdateUsage(context.WithoutCancel(ctx), id, func(l *domain.UsageLedger) {
		settled = nil
		for _, each := range lookups {
			if each.err != nil {
				l.Missed(each.id, now)
				continue
			}
			pending := l.Pending
			done, warn := l.Settle(each.id, each.cost, u.MaxSessionUSD)
			warned = warned || warn
			if done {
				settled = append(settled, settledSample(pending, each.id, each.cost))
			}
		}
		l.Expire(now)
	})
	if err != nil {
		return ledger, false, false
	}
	// The receipt journals each settlement, so an execution's receipt adds
	// up to its ledger.
	if u.Receipts != nil {
		for _, sample := range settled {
			_ = u.Receipts.AppendReceipt(context.WithoutCancel(ctx), id, domain.ReceiptEntry{Kind: domain.ReceiptRequestSettled, At: now.UTC(), Usage: sample})
		}
	}
	return updated, true, warned
}

// settledSample is what the receipt says of a settled request: its
// generation id, the tokens that replaced its estimate and its cost.
func settledSample(pending []domain.PendingGeneration, id string, cost domain.GenerationCost) domain.UsageSample {
	sample := domain.UsageSample{GenerationID: id, Cost: cost.Cost, CostKnown: true, PromptTokens: cost.PromptTokens, CompletionTokens: cost.CompletionTokens}
	if !cost.TokensKnown {
		for _, each := range pending {
			if each.ID == id {
				sample.PromptTokens, sample.CompletionTokens = each.PromptTokens, each.CompletionTokens
			}
		}
	}
	return sample
}
