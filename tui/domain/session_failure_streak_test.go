package domain

import (
	"errors"
	"strings"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// failing answers with one call of read whose about is about, and records
// failure, the console's refusal of it, as its outcome.
func failing(t *testing.T, s *Session, id, about, failure string) {
	t.Helper()
	arguments, err := axlr.NewJSONObject([]byte(`{"about":"` + about + `","links":[],"n":7}`))
	must(t, err)
	must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, ToolCalls: []axlr.ToolCall{{ID: axlr.ToolCallID(id), Name: "read", Arguments: arguments}}}, FinishReason: "tool_calls"}))
	must(t, s.RecordToolOutcome(axlr.ToolCallID(id), DecisionAutoApprove, ToolOutcome{Content: axlr.Text(failure), IsError: true}))
}

// Refusals that differ only in the call's id and the values the call
// itself sent are the same failure; another failure, a call that worked
// or the person's word ends the streak, and a budget note does not.
func TestToolFailureStreakCountsAlikeFailuresOfTheTurn(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hello", catalog(t)))
	if streak := s.ToolFailureStreak(); streak.Count != 0 {
		t.Fatalf("%+v", streak)
	}
	refusal := func(id, about string) string {
		return `{"call_id":"` + id + `","error":"name about as one of project:dev.a: ` + about + ` is another's"}`
	}
	failing(t, &s, "c1", "project:dev.void", refusal("c1", "project:dev.void"))
	failing(t, &s, "c2", "project:dev.void2", refusal("c2", "project:dev.void2"))
	must(t, s.Note(BudgetNotePrefix+" 6 tool calls left of this turn's 32."))
	failing(t, &s, "c3", "project:dev.void3", refusal("c3", "project:dev.void3"))
	streak := s.ToolFailureStreak()
	if streak.Count != 3 || streak.Tool != "read" || streak.Failure != "name about as one of project:dev.a: project:dev.void3 is another's" {
		t.Fatalf("%+v", streak)
	}
	// Another failure starts a new streak.
	failing(t, &s, "c4", "project:dev.void4", `{"call_id":"c4","error":"KMP is unreachable"}`)
	if streak := s.ToolFailureStreak(); streak.Count != 1 || streak.Failure != "KMP is unreachable" {
		t.Fatalf("%+v", streak)
	}
	// A runtime envelope is compared without its id and times; plain text
	// as it is.
	envelope := func(id, at string) string {
		return `{"protocol_version":1,"request_id":"` + id + `","started_at":"` + at + `","duration_ms":3,"output":{"exit_code":1},"error":null}`
	}
	failing(t, &s, "c5", "x", envelope("c5", "2026-11-12T20:00:00Z"))
	failing(t, &s, "c6", "x", envelope("c6", "2026-11-12T20:00:09Z"))
	if streak := s.ToolFailureStreak(); streak.Count != 2 || !strings.Contains(streak.Failure, `"exit_code":1`) || strings.Contains(streak.Failure, "started_at") {
		t.Fatalf("%+v", streak)
	}
	failing(t, &s, "c7", "x", "plain trouble")
	failing(t, &s, "c8", "x", " plain trouble ")
	if streak := s.ToolFailureStreak(); streak.Count != 2 || streak.Failure != "plain trouble" {
		t.Fatalf("%+v", streak)
	}
	// A call that worked ends it.
	must(t, s.CompleteAssistant(completion(t, "ok")))
	must(t, s.RecordToolOutcome("ok", DecisionAutoApprove, ToolOutcome{Content: "fine"}))
	if streak := s.ToolFailureStreak(); streak.Count != 0 {
		t.Fatalf("%+v", streak)
	}
	// The person's next message starts a turn without a streak.
	failing(t, &s, "c9", "x", "plain trouble")
	must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "stuck"}, FinishReason: "stop"}))
	must(t, s.BeginTurn("try again", catalog(t)))
	if streak := s.ToolFailureStreak(); streak.Count != 0 {
		t.Fatalf("%+v", streak)
	}
}

// The tool loop fails the execution even though its turn is interrupted
// and kept, and the receipt says why it ended as the result does.
func TestToolFailureStreakErrorFailsTheExecution(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.PauseTurn())
	stop := &ToolFailureStreakError{Tool: "axlr_remember", Count: 3, Failure: "leave about out"}
	result := NewExecutionResult(s, errors.Join(stop, errors.New("saved")))
	if result.Status != ExecutionFailed || !strings.HasPrefix(result.Error, "tool loop: axlr_remember failed 3 times in a row with the same error, so the run stopped: leave about out") {
		t.Fatalf("%+v", result)
	}
	if receipt := NewExecutionReceipt(result, []ReceiptSession{{Session: s}}); receipt.Status != ExecutionFailed || receipt.Error != result.Error {
		t.Fatalf("%+v", receipt)
	}
}
