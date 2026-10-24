package domain

import (
	"context"
	"errors"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// The result of a finished execution: the answer, every call with what
// became of it, and the usage of each session it spanned, summed.
func TestExecutionResultReadsAFinishedSession(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one", "two", "three")))
	must(t, s.RecordToolOutcome("one", DecisionDeny, ToolOutcome{Content: "denied", IsError: true}))
	must(t, s.RecordToolOutcome("two", DecisionApprove, ToolOutcome{Content: "second", Uncertain: true}))
	must(t, s.RecordToolOutcome("three", DecisionAutoApprove, ToolOutcome{Content: "boom", IsError: true}))
	must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "final answer"}, FinishReason: "stop"}))
	result := NewExecutionResult(s, nil)
	if result.Status != ExecutionCompleted || result.Session != s.ID() || len(result.Sessions) != 1 || result.Answer != "final answer" || result.Pending != nil || result.Error != "" {
		t.Fatalf("%+v", result)
	}
	want := []ExecutionCall{
		{ID: "one", Tool: "read", Decision: DecisionDeny, Status: ExecutionCallDenied},
		{ID: "two", Tool: "read", Decision: DecisionApprove, Status: ExecutionCallUncertain},
		{ID: "three", Tool: "read", Decision: DecisionAutoApprove, Status: ExecutionCallFailed},
	}
	if len(result.Calls) != len(want) {
		t.Fatalf("calls %+v", result.Calls)
	}
	for i := range want {
		if result.Calls[i] != want[i] {
			t.Fatalf("call %d: %+v", i, result.Calls[i])
		}
	}
	first, second := UsageLedger{Requests: 2, PromptTokens: 100, CachedTokens: 10, CacheWriteTokens: 5, CompletionTokens: 20, ReasoningTokens: 8, Cost: 0.01, CostRequests: 2}, UsageLedger{Requests: 1, PromptTokens: 50, CompletionTokens: 5, Unmetered: 1}
	result.AddUsage(first)
	result.AddUsage(second)
	if result.Usage != (ExecutionUsage{Requests: 3, PromptTokens: 150, CachedTokens: 10, CacheWriteTokens: 5, CompletionTokens: 25, ReasoningTokens: 8, CostUSD: 0.01, CostRequests: 2, Unmetered: 1}) {
		t.Fatalf("usage %+v", result.Usage)
	}
	predecessor, _ := NewSessionID("fedcba9876543210fedcba9876543210")
	result.Spanned(predecessor)
	if len(result.Sessions) != 2 || result.Sessions[0] != predecessor || result.Sessions[1] != s.ID() {
		t.Fatalf("sessions %v", result.Sessions)
	}
}

// A pending call makes the result wait for a decision; a completed call
// with no outcome yet stays pending in the calls.
func TestExecutionResultWaitsForADecision(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one")))
	result := NewExecutionResult(s, nil)
	if result.Status != ExecutionNeedsDecision || result.Pending == nil || result.Pending.ID != "one" || result.Pending.Name != "read" || result.Answer != "answer" {
		t.Fatalf("%+v", result)
	}
	if len(result.Calls) != 1 || result.Calls[0].Status != ExecutionCallPending {
		t.Fatalf("calls %+v", result.Calls)
	}
}

// An error fails the execution, unless it is a cancellation or the session
// was left interrupted: then the execution can be resumed.
func TestExecutionResultTellsInterruptedFromFailed(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hello", catalog(t)))
	if result := NewExecutionResult(s, errors.New("model unreachable")); result.Status != ExecutionFailed || result.Error != "model unreachable" {
		t.Fatalf("%+v", result)
	}
	if result := NewExecutionResult(s, context.Canceled); result.Status != ExecutionInterrupted || result.Error != context.Canceled.Error() {
		t.Fatalf("%+v", result)
	}
	must(t, s.PauseTurn())
	if result := NewExecutionResult(s, nil); result.Status != ExecutionInterrupted || result.Error != "" {
		t.Fatalf("%+v", result)
	}
	if result := NewExecutionResult(s, errors.New("stopped")); result.Status != ExecutionInterrupted || result.Error != "stopped" {
		t.Fatalf("%+v", result)
	}
}
