package domain

import (
	"errors"
	"time"
)

// ExecutionAccount is the execution's budget as the session keeps it, with
// what the execution had spent before this session's count began. The
// session's share is what its ledger and its calls added since the
// budget was set.
type ExecutionAccount struct {
	Budget ExecutionBudget
	// StartedAt is when the execution began: the seconds count from it,
	// in every session the execution spans.
	StartedAt time.Time
	// Carried is what the execution spent in the sessions before this
	// one, the predecessors a context cap left behind.
	Carried ExecutionSpend
	// Base is this session's own count when the budget was set: its
	// requests and cost from the ledger, and the calls it had run. A
	// session that takes a budget with history of its own counts from
	// here.
	Base ExecutionSpend
	// Stopped names the limit that ended the execution, "" while none
	// did: the result reads budget_exceeded from it, and the next turn or
	// resume clears it.
	Stopped BudgetLimit
}

func (a ExecutionAccount) Validate() error {
	if err := a.Budget.Validate(); err != nil {
		return err
	}
	if err := a.Carried.Validate(); err != nil {
		return err
	}
	if err := a.Base.Validate(); err != nil {
		return err
	}
	if a.Stopped != "" {
		return a.Stopped.Validate()
	}
	return nil
}

// Budget is the execution's account, if the session has one.
func (s Session) Budget() (ExecutionAccount, bool) {
	if s.state.Budget == nil {
		return ExecutionAccount{}, false
	}
	return *s.state.Budget, true
}

// SetBudget puts the session under an execution's budget from now on.
// startedAt is when the execution began and ledger the session's ledger
// at this moment, so a session with history of its own counts from here;
// a stop recorded under an earlier budget is lifted.
func (s *Session) SetBudget(budget ExecutionBudget, startedAt time.Time, ledger UsageLedger) error {
	if err := budget.Validate(); err != nil {
		return err
	}
	if startedAt.IsZero() {
		return errors.New("a budget needs the time the execution began")
	}
	s.state.Budget = &ExecutionAccount{Budget: budget, StartedAt: startedAt.UTC(), Base: ExecutionSpend{Requests: ledger.Requests, ToolCalls: s.ToolCallsRun(), USD: ledger.Cost}}
	return nil
}

// InheritBudget continues the predecessor's execution in this session, a
// successor a context cap opened: the same budget and start, with what the
// execution spent so far carried, and this session's count from zero.
func (s *Session) InheritBudget(predecessor ExecutionAccount, spent ExecutionSpend) error {
	if err := predecessor.Validate(); err != nil {
		return err
	}
	if err := spent.Validate(); err != nil {
		return err
	}
	s.state.Budget = &ExecutionAccount{Budget: predecessor.Budget, StartedAt: predecessor.StartedAt, Carried: ExecutionSpend{Requests: spent.Requests, ToolCalls: spent.ToolCalls, USD: spent.USD}}
	return nil
}

// RestoreBudget puts back an account a store kept.
func (s *Session) RestoreBudget(account ExecutionAccount) error {
	if err := account.Validate(); err != nil {
		return err
	}
	clone := account
	s.state.Budget = &clone
	return nil
}

// ClearBudget lifts the execution's budget.
func (s *Session) ClearBudget() { s.state.Budget = nil }

// BudgetSpend is what the execution has spent: what it carried into this
// session, plus this session's share since the budget was set, read from
// its ledger and its calls, with the clock at now.
func (s Session) BudgetSpend(ledger UsageLedger, now time.Time) ExecutionSpend {
	account := s.state.Budget
	if account == nil {
		return ExecutionSpend{}
	}
	spend := ExecutionSpend{
		Requests:  account.Carried.Requests + max(0, ledger.Requests-account.Base.Requests),
		ToolCalls: account.Carried.ToolCalls + max(0, s.ToolCallsRun()-account.Base.ToolCalls),
		USD:       account.Carried.USD + max(0, ledger.Cost-account.Base.USD),
	}
	if !account.StartedAt.IsZero() && now.After(account.StartedAt) {
		spend.Elapsed = now.Sub(account.StartedAt)
	}
	return spend
}

// ToolCallsRun counts the session's calls that ran: the ones approved, by
// a person or automatically. A call refused, by the policy, the mode, the
// turn's call limit or a denial, is no call run.
func (s Session) ToolCallsRun() int {
	ran := 0
	for _, record := range s.state.Activity {
		if record.Decision == DecisionApprove || record.Decision == DecisionAutoApprove {
			ran++
		}
	}
	return ran
}

// StopAtBudget records the limit that ended the execution, so the result
// reads budget_exceeded from the session alone.
func (s *Session) StopAtBudget(limit BudgetLimit) error {
	if s.state.Budget == nil {
		return errors.New("the session has no budget to stop at")
	}
	if err := limit.Validate(); err != nil {
		return err
	}
	account := *s.state.Budget
	account.Stopped = limit
	s.state.Budget = &account
	return nil
}

// BudgetStop is the error of the limit that ended the execution, nil while
// none did.
func (s Session) BudgetStop() *BudgetExceededError {
	if s.state.Budget == nil || s.state.Budget.Stopped == "" {
		return nil
	}
	return budgetStopOf(s.state.Budget.Stopped, s.state.Budget.Budget)
}

// liftBudgetStop clears the stop when a turn begins or resumes: the next
// request or call checks the budget again.
func liftBudgetStop(state *SessionState) {
	if state.Budget != nil && state.Budget.Stopped != "" {
		account := *state.Budget
		account.Stopped = ""
		state.Budget = &account
	}
}

// Correlation is the execution's correlation, if the session has one.
func (s Session) Correlation() (ExecutionCorrelation, bool) {
	if s.state.Correlation == nil {
		return ExecutionCorrelation{}, false
	}
	return *s.state.Correlation, true
}

// SetCorrelation names the execution the session runs for; an empty
// correlation clears it.
func (s *Session) SetCorrelation(correlation ExecutionCorrelation) error {
	if err := correlation.Validate(); err != nil {
		return err
	}
	if correlation.IsZero() {
		s.state.Correlation = nil
		return nil
	}
	clone := correlation
	s.state.Correlation = &clone
	return nil
}

// ClearCorrelation forgets the execution's correlation.
func (s *Session) ClearCorrelation() { s.state.Correlation = nil }

// Sandbox is the execution's sandbox, if the session has one.
func (s Session) Sandbox() (ExecutionSandbox, bool) {
	if s.state.Sandbox == nil {
		return ExecutionSandbox{}, false
	}
	return s.state.Sandbox.Clone(), true
}

// SetSandbox confines the session's commands from now on: this execution's
// and every later turn's or decision's.
func (s *Session) SetSandbox(sandbox ExecutionSandbox) error {
	if err := sandbox.Validate(); err != nil {
		return err
	}
	clone := sandbox.Clone()
	s.state.Sandbox = &clone
	return nil
}

// ClearSandbox lifts the execution's sandbox.
func (s *Session) ClearSandbox() { s.state.Sandbox = nil }

// Routing is the execution's routing, if the session has one.
func (s Session) Routing() (ExecutionRouting, bool) {
	if s.state.Routing == nil {
		return ExecutionRouting{}, false
	}
	return s.state.Routing.Clone(), true
}

// SetRouting chooses the model of each request of the session from now
// on, among the routing's candidates: this execution's and every later
// turn's or decision's.
func (s *Session) SetRouting(routing ExecutionRouting) error {
	if err := routing.Validate(); err != nil {
		return err
	}
	clone := routing.Clone()
	s.state.Routing = &clone
	return nil
}

// ClearRouting sends the session's requests to its own model again.
func (s *Session) ClearRouting() { s.state.Routing = nil }
