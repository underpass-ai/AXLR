package domain

import (
	"testing"
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// A session under a budget counts from the moment the budget was set: its
// earlier requests, cost and calls are its base, and the calls that ran
// since count while refused ones do not.
func TestSessionBudgetCountsFromWhenItWasSet(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one")))
	must(t, s.RecordToolOutcome("one", DecisionAutoApprove, ToolOutcome{Content: "ran"}))
	budget, _ := NewExecutionBudget(900, 20, 60, 0.5)
	started := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	must(t, s.SetBudget(budget, started, UsageLedger{Requests: 3, Cost: 0.2}))
	account, has := s.Budget()
	if !has || account.Budget != budget || !account.StartedAt.Equal(started) || account.Base != (ExecutionSpend{Requests: 3, ToolCalls: 1, USD: 0.2}) || account.Carried != (ExecutionSpend{}) {
		t.Fatalf("%+v %v", account, has)
	}
	must(t, s.CompleteAssistant(completion(t, "two", "three", "four")))
	must(t, s.RecordToolOutcome("two", DecisionApprove, ToolOutcome{Content: "ran"}))
	must(t, s.RecordToolOutcome("three", DecisionDeny, ToolOutcome{Content: "refused", IsError: true}))
	must(t, s.RecordToolOutcome("four", DecisionAutoApprove, ToolOutcome{Content: "ran"}))
	spend := s.BudgetSpend(UsageLedger{Requests: 5, Cost: 0.35}, started.Add(90*time.Second))
	if spend.Elapsed != 90*time.Second || spend.Requests != 2 || spend.ToolCalls != 2 || !nearUSD(spend.USD, 0.15) {
		t.Fatalf("%+v", spend)
	}
	// A ledger that went backwards never counts as negative spend.
	if spend := s.BudgetSpend(UsageLedger{}, started); spend.Requests != 0 || spend.USD != 0 || spend.Elapsed != 0 {
		t.Fatalf("%+v", spend)
	}
	if err := s.SetBudget(budget, time.Time{}, UsageLedger{}); err == nil {
		t.Fatal("a budget without a start")
	}
	s.ClearBudget()
	if _, has := s.Budget(); has {
		t.Fatal("cleared budget stays")
	}
}

// nearUSD compares dollars to the tenth of a cent.
func nearUSD(a, b float64) bool { return a-b < 0.0005 && b-a < 0.0005 }

// A successor carries what the execution spent before it and counts its
// own share from zero; the start is the execution's.
func TestSessionInheritsAnExecutionBudget(t *testing.T) {
	predecessor := session(t)
	budget, _ := NewExecutionBudget(900, 20, 60, 0.5)
	started := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	must(t, predecessor.SetBudget(budget, started, UsageLedger{}))
	must(t, predecessor.BeginTurn("hello", catalog(t)))
	must(t, predecessor.CompleteAssistant(completion(t, "one")))
	must(t, predecessor.RecordToolOutcome("one", DecisionAutoApprove, ToolOutcome{Content: "ran"}))
	account, _ := predecessor.Budget()
	spent := predecessor.BudgetSpend(UsageLedger{Requests: 4, Cost: 0.1}, started.Add(time.Minute))
	successor, err := NewSession("fedcba9876543210fedcba9876543210", predecessor.Export().Workspace, "test/model")
	must(t, err)
	must(t, successor.InheritBudget(account, spent))
	inherited, has := successor.Budget()
	if !has || inherited.Budget != budget || !inherited.StartedAt.Equal(started) || inherited.Carried != (ExecutionSpend{Requests: 4, ToolCalls: 1, USD: 0.1}) || inherited.Base != (ExecutionSpend{}) {
		t.Fatalf("%+v", inherited)
	}
	spend := successor.BudgetSpend(UsageLedger{Requests: 2, Cost: 0.05}, started.Add(2*time.Minute))
	if spend.Elapsed != 2*time.Minute || spend.Requests != 6 || spend.ToolCalls != 1 || !nearUSD(spend.USD, 0.15) {
		t.Fatalf("%+v", spend)
	}
	if err := successor.InheritBudget(account, ExecutionSpend{Requests: -1}); err == nil {
		t.Fatal("negative spend inherited")
	}
}

// The limit that ended the execution is kept on the session, so the result
// reads budget_exceeded from the session alone, with the pending call it
// left; a new turn or a resume lifts the stop.
func TestSessionBudgetStopEndsTheResult(t *testing.T) {
	s := session(t)
	if err := s.StopAtBudget(BudgetRequests); err == nil {
		t.Fatal("a stop without a budget")
	}
	budget, _ := NewExecutionBudget(0, 2, 1, 0)
	must(t, s.SetBudget(budget, time.Now(), UsageLedger{}))
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one")))
	must(t, s.PauseTurn())
	must(t, s.StopAtBudget(BudgetToolCalls))
	if err := s.StopAtBudget("minutes"); err == nil {
		t.Fatal("an unknown limit")
	}
	if stop := s.BudgetStop(); stop == nil || stop.Error() != "budget: tool_calls 1 reached" {
		t.Fatalf("%v", stop)
	}
	result := NewExecutionResult(s, nil)
	if result.Status != ExecutionBudgetExceeded || result.Error != "budget: tool_calls 1 reached" || result.Pending == nil || result.Pending.ID != "one" || len(result.Calls) != 1 || result.Calls[0].Status != ExecutionCallPending {
		t.Fatalf("%+v", result)
	}
	// The stop wins over the error the loop returned and over the
	// interrupted state.
	if result := NewExecutionResult(s, &BudgetExceededError{Limit: BudgetToolCalls, Allowed: 1}); result.Status != ExecutionBudgetExceeded {
		t.Fatalf("%+v", result)
	}
	restored, err := RestoreSession(s.Export())
	must(t, err)
	if restored.BudgetStop() == nil {
		t.Fatal("the stop was lost in a restore")
	}
	must(t, s.ResumePending())
	if s.BudgetStop() != nil {
		t.Fatal("a resume keeps the stop")
	}
	must(t, s.StopAtBudget(BudgetRequests))
	must(t, s.RecordToolOutcome("one", DecisionDeny, ToolOutcome{Content: "no", IsError: true}))
	must(t, s.PauseTurn())
	must(t, s.ResumeTurn())
	if s.BudgetStop() != nil {
		t.Fatal("a resumed turn keeps the stop")
	}
	must(t, s.StopAtBudget(BudgetRequests))
	must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "done"}}))
	// A completed turn is completed whatever an old stop says.
	if result := NewExecutionResult(s, nil); result.Status != ExecutionCompleted {
		t.Fatalf("%+v", result)
	}
	must(t, s.BeginTurn("again", catalog(t)))
	if s.BudgetStop() != nil {
		t.Fatal("a new turn keeps the stop")
	}
	// A new budget lifts a stop too.
	must(t, s.StopAtBudget(BudgetUSD))
	must(t, s.SetBudget(budget, time.Now(), UsageLedger{}))
	if s.BudgetStop() != nil {
		t.Fatal("a new budget keeps the stop")
	}
}

// The correlation lives on the session and in its result, unchanged.
func TestSessionCorrelationReachesTheResult(t *testing.T) {
	s := session(t)
	if _, has := s.Correlation(); has {
		t.Fatal("a new session has a correlation")
	}
	correlation := ExecutionCorrelation{BotID: "server-incident-investigator", TaskID: "task-789", ExecutionID: "exec-1", TraceID: "trace-abc"}
	must(t, s.SetCorrelation(correlation))
	if err := s.SetCorrelation(ExecutionCorrelation{BotID: "bot 1"}); err == nil {
		t.Fatal("an unsafe id")
	}
	kept, has := s.Correlation()
	if !has || kept != correlation {
		t.Fatalf("%+v", kept)
	}
	restored, err := RestoreSession(s.Export())
	must(t, err)
	if kept, has := restored.Correlation(); !has || kept != correlation {
		t.Fatalf("%+v", kept)
	}
	state := s.Export()
	state.Correlation = &ExecutionCorrelation{BotID: "bad id"}
	if _, err := RestoreSession(state); err == nil {
		t.Fatal("an invalid correlation restored")
	}
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "done"}}))
	result := NewExecutionResult(s, nil)
	if result.Correlation == nil || *result.Correlation != correlation {
		t.Fatalf("%+v", result)
	}
	must(t, s.SetCorrelation(ExecutionCorrelation{}))
	if _, has := s.Correlation(); has {
		t.Fatal("an empty correlation stays")
	}
}
