package application

import (
	"context"
	"errors"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The execution's budget (docs/runtime-contract.md, "Budget per
// execution") is enforced here, outside the model: the seconds as a
// deadline on the turn's context, the requests and the dollars before each
// model request, the tool calls before each call that would run. A limit
// reached ends the execution with the session marked and left resumable,
// its pending state intact; the result reads budget_exceeded from the
// mark.

// executionDeadline derives the turn's context from the execution's
// budget: past its seconds the context is cancelled with the budget's
// error as the cause, which budgetCause reads back.
func executionDeadline(ctx context.Context, s domain.Session) (context.Context, context.CancelFunc) {
	account, has := s.Budget()
	if !has {
		return ctx, func() {}
	}
	deadline, ok := account.Budget.Deadline(account.StartedAt)
	if !ok {
		return ctx, func() {}
	}
	return context.WithDeadlineCause(ctx, deadline, &domain.BudgetExceededError{Limit: domain.BudgetSeconds, Allowed: float64(account.Budget.Seconds())})
}

// budgetCause is the budget error behind err: the error itself, or the
// cause of a context the budget's deadline cancelled; nil for any other
// error.
func budgetCause(ctx context.Context, err error) *domain.BudgetExceededError {
	var stop *domain.BudgetExceededError
	if errors.As(err, &stop) {
		return stop
	}
	if err != nil && errors.Is(err, context.DeadlineExceeded) && ctx != nil && errors.As(context.Cause(ctx), &stop) {
		return stop
	}
	return nil
}

// executionSpend is what the execution has spent, from the session and
// its ledger. A ledger that cannot be read refuses, since the limit cannot
// be kept.
func (u ContinueTurnUseCase) executionSpend(ctx context.Context, s domain.Session) (domain.ExecutionSpend, error) {
	var ledger domain.UsageLedger
	if u.Usage != nil {
		loaded, err := u.Usage.LoadUsage(ctx, s.ID())
		if err != nil {
			return domain.ExecutionSpend{}, err
		}
		ledger = loaded
	}
	return s.BudgetSpend(ledger, time.Now()), nil
}

// budgetRefusalOf is the budget's verdict on the named limits: the
// BudgetExceededError of the first one reached, or the ledger's error.
func (u ContinueTurnUseCase) budgetRefusalOf(ctx context.Context, s domain.Session, limits ...domain.BudgetLimit) error {
	account, has := s.Budget()
	if !has || account.Budget.IsZero() {
		return nil
	}
	spend, err := u.executionSpend(ctx, s)
	if err != nil {
		return err
	}
	return account.Budget.Exceeded(spend, limits...)
}

// requestRefusal refuses the next model request once the execution's
// seconds, requests or dollars are spent.
func (u ContinueTurnUseCase) requestRefusal(ctx context.Context, s domain.Session) error {
	return u.budgetRefusalOf(ctx, s, domain.BudgetSeconds, domain.BudgetRequests, domain.BudgetUSD)
}

// toolCallRefusal refuses the next tool call once the execution's seconds
// or tool calls are spent.
func (u ContinueTurnUseCase) toolCallRefusal(ctx context.Context, s domain.Session) error {
	return u.budgetRefusalOf(ctx, s, domain.BudgetSeconds, domain.BudgetToolCalls)
}

// requestsLeft reports whether the execution may make another model
// request: a console follow-up (a reminder, Jev's check) is skipped when
// it may not, so an answered prompt ends completed rather than at the
// budget.
func (u ContinueTurnUseCase) requestsLeft(ctx context.Context, s domain.Session) bool {
	return u.requestRefusal(ctx, s) == nil
}

// pauseAtBudget ends the execution at a limit: the session is marked with
// it, its turn interrupted with the pending state intact, saved, and the
// state told; the error goes back to end the execution. A session already
// paused is marked and saved as it is.
func pauseAtBudget(ctx context.Context, store SessionStorePort, s *domain.Session, stop *domain.BudgetExceededError, emit func(Event) error) error {
	if s == nil || store == nil {
		return stop
	}
	next := *s
	if err := next.StopAtBudget(stop.Limit); err != nil {
		return errors.Join(stop, err)
	}
	switch next.Status() {
	case domain.StatusStreaming:
		if err := next.InterruptDraft(""); err != nil {
			return errors.Join(stop, err)
		}
	case domain.StatusApproval:
		if err := next.PauseTurn(); err != nil {
			return errors.Join(stop, err)
		}
	}
	if err := store.Save(context.WithoutCancel(ctx), next); err != nil {
		return errors.Join(stop, err)
	}
	*s = next
	if emit == nil {
		return stop
	}
	return errors.Join(stop, emit(Event{Kind: EventState, State: next.Status()}))
}

// settleBudget is the use cases' last word on an error: when the budget's
// deadline ended the turn on a path that did not mark the session (a save
// the deadline refused, a cancelled tool), the session is marked and saved
// here, so the result reads budget_exceeded on every path.
func settleBudget(ctx context.Context, store SessionStorePort, s *domain.Session, err error) error {
	stop := budgetCause(ctx, err)
	if stop == nil {
		return err
	}
	if s != nil && s.BudgetStop() == nil {
		err = errors.Join(err, pauseAtBudget(ctx, store, s, stop, nil))
	}
	var named *domain.BudgetExceededError
	if errors.As(err, &named) {
		return err
	}
	return errors.Join(stop, err)
}
