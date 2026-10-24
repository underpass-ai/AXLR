package application

import (
	"context"
	"errors"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// budgetSession is a session under budget, its execution begun at
// startedAt, with a turn begun on the generic "read" tool.
func executionSession(t *testing.T, budget domain.ExecutionBudget, startedAt time.Time) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.SetBudget(budget, startedAt, domain.UsageLedger{}); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("go", turnTools()); err != nil {
		t.Fatal(err)
	}
	return s
}

// budgetAgent runs the loop with every call approved, counting the model's
// requests and the calls that ran; the model answers each request with
// one call of "read", so a turn goes on until a limit stops it.
func executionAgent(model streamFunc, usage SessionUsagePort, executed *int) AgentTurnUseCase {
	return AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Usage: usage, Validation: argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { return nil }), Models: model}, Approval: approvalFunc(func(domain.ToolIdentity) bool { return true }), Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		*executed++
		return domain.ToolOutcome{Content: "ran"}, nil
	})}
}

func callingModel(requests *int, calls int, cost float64) streamFunc {
	return func(_ context.Context, _ root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		*requests++
		var toolCalls []root.ToolCall
		for i := 0; i < calls; i++ {
			toolCalls = append(toolCalls, call(root.ToolCallID(string(rune('a'+i))+string(rune('0'+*requests))), "read"))
		}
		result := assistant("", toolCalls...)
		if len(toolCalls) == 0 {
			result = assistant("done")
		}
		if cost > 0 {
			result.Usage = &root.TokenUsage{PromptTokens: 10, CompletionTokens: 2, Cost: cost, CostKnown: true}
		}
		return result, nil
	}
}

func stoppedAt(t *testing.T, err error) *domain.BudgetExceededError {
	t.Helper()
	var stop *domain.BudgetExceededError
	if !errors.As(err, &stop) {
		t.Fatalf("not a budget stop: %v", err)
	}
	return stop
}

// The requests limit refuses the request past it, before it is sent: the
// model was asked exactly the allowed number of times, the session is
// interrupted and resumable, and its result says which limit.
func TestBudgetRefusesTheRequestPastTheLimit(t *testing.T) {
	budget, _ := domain.NewExecutionBudget(0, 2, 0, 0)
	s := executionSession(t, budget, time.Now())
	requests, executed := 0, 0
	u := executionAgent(callingModel(&requests, 1, 0), usageLedgers{}, &executed)
	err := u.Execute(context.Background(), &s, ignoreEvent)
	if stop := stoppedAt(t, err); stop.Limit != domain.BudgetRequests || err.Error() != "budget: requests 2 reached" && stop.Error() != "budget: requests 2 reached" {
		t.Fatalf("%v", err)
	}
	if requests != 2 || executed != 2 || s.Status() != domain.StatusInterrupted || len(s.Pending()) != 0 {
		t.Fatalf("requests %d executed %d status %s pending %d", requests, executed, s.Status(), len(s.Pending()))
	}
	result := domain.NewExecutionResult(s, err)
	if result.Status != domain.ExecutionBudgetExceeded || result.Error != "budget: requests 2 reached" || len(result.Calls) != 2 {
		t.Fatalf("%+v", result)
	}
	if err := s.ResumeTurn(); err != nil || s.BudgetStop() != nil {
		t.Fatalf("not resumable: %v %v", err, s.BudgetStop())
	}
	// Resumed under the same budget, the next request is refused again.
	err = u.Execute(context.Background(), &s, ignoreEvent)
	if stoppedAt(t, err); requests != 2 {
		t.Fatalf("a request was sent past the limit: %d", requests)
	}
}

// The tool_calls limit refuses the call past it before it runs: the call
// stays pending, so a run under a new budget can take it.
func TestBudgetRefusesTheToolCallPastTheLimit(t *testing.T) {
	budget, _ := domain.NewExecutionBudget(0, 0, 1, 0)
	s := executionSession(t, budget, time.Now())
	requests, executed := 0, 0
	u := executionAgent(callingModel(&requests, 2, 0), usageLedgers{}, &executed)
	var states []domain.SessionStatus
	err := u.Execute(context.Background(), &s, func(e Event) error {
		if e.Kind == EventState {
			states = append(states, e.State)
		}
		return nil
	})
	if stop := stoppedAt(t, err); stop.Limit != domain.BudgetToolCalls || stop.Error() != "budget: tool_calls 1 reached" {
		t.Fatalf("%v", err)
	}
	if requests != 1 || executed != 1 || s.Status() != domain.StatusInterrupted || len(s.Pending()) != 1 || s.Pending()[0].Call.ID != "b1" {
		t.Fatalf("requests %d executed %d status %s pending %+v", requests, executed, s.Status(), s.Pending())
	}
	if len(states) == 0 || states[len(states)-1] != domain.StatusInterrupted {
		t.Fatalf("the stop was not told: %v", states)
	}
	result := domain.NewExecutionResult(s, err)
	if result.Status != domain.ExecutionBudgetExceeded || result.Error != "budget: tool_calls 1 reached" || result.Pending == nil || result.Pending.ID != "b1" || result.Calls[1].Status != domain.ExecutionCallPending {
		t.Fatalf("%+v", result)
	}
	// A decision taken under the spent budget cannot run the call either.
	if err := s.ResumePending(); err != nil {
		t.Fatal(err)
	}
	resolver := ResolveToolUseCase{Tools: u.Tools, Store: u.Continue.Store, Validation: u.Continue.Validation, Approval: u.Approval, Continue: u.Continue}
	err = resolver.Execute(context.Background(), &s, "b1", domain.DecisionApprove, ignoreEvent)
	if stoppedAt(t, err); executed != 1 || len(s.Pending()) != 1 || s.Status() != domain.StatusInterrupted {
		t.Fatalf("executed %d pending %d status %s", executed, len(s.Pending()), s.Status())
	}
	// Under a new budget the pending call runs.
	wider, _ := domain.NewExecutionBudget(0, 0, 10, 0)
	if err := s.SetBudget(wider, time.Now(), domain.UsageLedger{}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResumePending(); err != nil {
		t.Fatal(err)
	}
	requests, executed = 1, 0
	u.Continue.Models = callingModel(&requests, 0, 0)
	resolver.Continue = u.Continue
	if err := resolver.Execute(context.Background(), &s, "b1", domain.DecisionApprove, ignoreEvent); err != nil || executed != 1 || s.Status() != domain.StatusComplete {
		t.Fatalf("%v executed %d status %s", err, executed, s.Status())
	}
}

// The usd limit is checked against the ledger's known cost before each
// request.
func TestBudgetRefusesTheRequestPastTheDollars(t *testing.T) {
	budget, _ := domain.NewExecutionBudget(0, 0, 0, 0.01)
	s := executionSession(t, budget, time.Now())
	requests, executed := 0, 0
	u := executionAgent(callingModel(&requests, 1, 0.01), usageLedgers{}, &executed)
	err := u.Execute(context.Background(), &s, ignoreEvent)
	if stop := stoppedAt(t, err); stop.Limit != domain.BudgetUSD || stop.Error() != "budget: usd 0.01 reached" || requests != 1 || executed != 1 {
		t.Fatalf("%v requests %d executed %d", err, requests, executed)
	}
	if result := domain.NewExecutionResult(s, err); result.Status != domain.ExecutionBudgetExceeded || result.Error != "budget: usd 0.01 reached" {
		t.Fatalf("%+v", result)
	}
}

// The seconds limit is a deadline on the turn's context, counted from
// when the execution began: a deadline already past stops the turn before
// any request, and one that falls during a request ends the request, the
// session interrupted and resumable, the result budget_exceeded.
func TestBudgetDeadlineEndsTheTurn(t *testing.T) {
	budget, _ := domain.NewExecutionBudget(1, 0, 0, 0)
	s := executionSession(t, budget, time.Now().Add(-2*time.Second))
	requests, executed := 0, 0
	u := executionAgent(callingModel(&requests, 1, 0), usageLedgers{}, &executed)
	err := u.Execute(context.Background(), &s, ignoreEvent)
	if stop := stoppedAt(t, err); stop.Limit != domain.BudgetSeconds || stop.Error() != "budget: seconds 1 reached" || requests != 0 || s.Status() != domain.StatusInterrupted {
		t.Fatalf("%v requests %d status %s", err, requests, s.Status())
	}
	if result := domain.NewExecutionResult(s, err); result.Status != domain.ExecutionBudgetExceeded || result.Error != "budget: seconds 1 reached" {
		t.Fatalf("%+v", result)
	}
	// A request in flight at the deadline: the model waits for the
	// context, as a provider's stream would, and the turn ends there.
	s = executionSession(t, budget, time.Now().Add(-time.Second+100*time.Millisecond))
	slow := streamFunc(func(ctx context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
		requests++
		_ = emit("partial")
		<-ctx.Done()
		return root.CompletionResult{}, ctx.Err()
	})
	u = executionAgent(slow, usageLedgers{}, &executed)
	err = u.Execute(context.Background(), &s, ignoreEvent)
	if stop := stoppedAt(t, err); stop.Limit != domain.BudgetSeconds || requests != 1 || s.Status() != domain.StatusInterrupted || s.Export().Draft != "partial" {
		t.Fatalf("%v requests %d status %s draft %q", err, requests, s.Status(), s.Export().Draft)
	}
	if result := domain.NewExecutionResult(s, err); result.Status != domain.ExecutionBudgetExceeded || result.Error != "budget: seconds 1 reached" {
		t.Fatalf("%+v", result)
	}
	if err := s.ResumeTurn(); err != nil {
		t.Fatalf("not resumable: %v", err)
	}
	// A session without a budget, or with one without seconds, keeps the
	// caller's context.
	plain := turnSession(t)
	if ctx, cancel := executionDeadline(context.Background(), plain); ctx != context.Background() {
		cancel()
		t.Fatal("a deadline without a budget")
	}
	none, _ := domain.NewExecutionBudget(0, 5, 0, 0)
	if err := plain.SetBudget(none, time.Now(), domain.UsageLedger{}); err != nil {
		t.Fatal(err)
	}
	if ctx, cancel := executionDeadline(context.Background(), plain); ctx != context.Background() {
		cancel()
		t.Fatal("a deadline without seconds")
	}
}

// A console follow-up is skipped once the execution has no request left,
// so an answered prompt ends completed.
func TestBudgetLeavesNoRequestForAFollowUp(t *testing.T) {
	budget, _ := domain.NewExecutionBudget(0, 1, 0, 0)
	s := executionSession(t, budget, time.Now())
	u := ContinueTurnUseCase{Usage: usageLedgers{s.ID(): {Requests: 0}}}
	if !u.requestsLeft(context.Background(), s) {
		t.Fatal("a request left was refused")
	}
	u.Usage = usageLedgers{s.ID(): {Requests: 1}}
	if u.requestsLeft(context.Background(), s) {
		t.Fatal("no request left, yet a follow-up allowed")
	}
	if !(ContinueTurnUseCase{}).requestsLeft(context.Background(), turnSession(t)) {
		t.Fatal("a session without a budget refused a follow-up")
	}
}

// The execution's correlation goes on every trace record of the loop and
// on the turn's record and its journal labels, unchanged, and never into
// the model's request.
func TestCorrelationReachesTracesAndRecords(t *testing.T) {
	s := turnSession(t)
	correlation := domain.ExecutionCorrelation{BotID: "server-incident-investigator", TaskID: "task-789", ExecutionID: "exec-1", TraceID: "trace-abc"}
	if err := s.SetCorrelation(correlation); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("go", turnTools()); err != nil {
		t.Fatal(err)
	}
	trace := &diagnosticCollector{}
	requests, executed := 0, 0
	var bodies []root.CompletionRequest
	model := streamFunc(func(ctx context.Context, req root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
		bodies = append(bodies, req)
		return callingModel(&requests, 1-requests, 0)(ctx, req, emit)
	})
	u := executionAgent(model, usageLedgers{}, &executed)
	u.Continue.Diagnostics = trace
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil || s.Status() != domain.StatusComplete || executed != 1 {
		t.Fatalf("%v %s executed %d", err, s.Status(), executed)
	}
	if len(trace.events) == 0 {
		t.Fatal("no trace records")
	}
	for _, event := range trace.events {
		if event.Correlation == nil || *event.Correlation != (DiagnosticCorrelation{BotID: "server-incident-investigator", TaskID: "task-789", ExecutionID: "exec-1", TraceID: "trace-abc"}) {
			t.Fatalf("record %s without the correlation: %+v", event.Stage, event.Correlation)
		}
	}
	for _, req := range bodies {
		for _, message := range req.Messages {
			for _, id := range []string{"server-incident-investigator", "task-789", "exec-1", "trace-abc"} {
				if containsText(string(message.Content), id) {
					t.Fatalf("the model read the correlation: %s", message.Content)
				}
			}
		}
	}
	record := BuildTurnRecord(s, 0, 1, "", nil)
	if record.Correlation["bot_id"] != "server-incident-investigator" || record.Correlation["task_id"] != "task-789" || record.Correlation["execution_id"] != "exec-1" || record.Correlation["trace_id"] != "trace-abc" || len(record.Correlation) != 4 {
		t.Fatalf("%+v", record.Correlation)
	}
	labels := labelCorrelation(map[string][]string{"session": {string(s.ID())}}, s)
	if len(labels) != 5 || labels["bot_id"][0] != "server-incident-investigator" || labels["trace_id"][0] != "trace-abc" {
		t.Fatalf("%+v", labels)
	}
	// Only the given ids are labelled.
	partial := turnSession(t)
	_ = partial.SetCorrelation(domain.ExecutionCorrelation{ExecutionID: "exec-2"})
	if labels := correlationLabels(partial); len(labels) != 1 || labels["execution_id"] != "exec-2" {
		t.Fatalf("%+v", labels)
	}
	if BuildTurnRecord(turnSession(t), 0, 1, "", nil).Correlation != nil {
		t.Fatal("a session without a correlation labelled its record")
	}
	// Without a correlation the port is left as it is; a record of its own
	// keeps its correlation.
	if CorrelatedDiagnostics(trace, domain.ExecutionCorrelation{}) != DiagnosticPort(trace) {
		t.Fatal("an empty correlation wrapped the port")
	}
	if !(DiagnosticCorrelation{TraceID: "t"}).Valid() || (DiagnosticCorrelation{}).Valid() || (DiagnosticCorrelation{BotID: "bot one"}).Valid() {
		t.Fatal("correlation validity")
	}
}

func containsText(text, part string) bool {
	return len(part) > 0 && len(text) >= len(part) && (text == part || indexOf(text, part) >= 0)
}

func indexOf(text, part string) int {
	for i := 0; i+len(part) <= len(text); i++ {
		if text[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
