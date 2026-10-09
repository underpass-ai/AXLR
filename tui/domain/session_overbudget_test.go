package domain

import (
	"fmt"
	"strings"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// On 10 October 2026 claude-haiku-5.5 listed a directory (1 call) and then
// sent 93 local_read calls in one answer; the 32-call budget refused the
// whole answer, so none ran and the turn count jumped to 94. The calls that
// fit now run in order and only the overflow is refused, as each reaches the
// head of the queue; the count keeps the calls admitted.
func TestABatchOverTheBudgetRunsWhatFitsAndRefusesTheRest(t *testing.T) {
	const limit = 8
	s := session(t)
	must(t, s.BeginTurn("open every file", catalog(t)))
	must(t, s.CompleteAssistantWithin(completion(t, "list"), limit))
	must(t, s.RecordToolOutcome("list", DecisionAutoApprove, ToolOutcome{Content: "93 files"}))
	ids := make([]string, 12)
	for i := range ids {
		ids[i] = fmt.Sprintf("r%d", i)
	}
	if e := s.CompleteAssistantWithin(completion(t, ids...), limit); e != nil {
		t.Fatalf("a batch that partly fits was refused whole: %v", e)
	}
	if s.Status() != StatusApproval || len(s.Pending()) != 12 {
		t.Fatalf("status %s, pending %d", s.Status(), len(s.Pending()))
	}
	for i := 0; i < limit-1; i++ {
		if s.HeadOverBudget(limit) {
			t.Fatalf("call %d of the budget was refused", i+2)
		}
		must(t, s.RecordToolOutcome(axlr.ToolCallID(ids[i]), DecisionAutoApprove, ToolOutcome{Content: "file"}))
		if _, e := RestoreSession(s.Export()); e != nil {
			t.Fatalf("mid-batch state does not restore: %v", e)
		}
	}
	for i := limit - 1; i < len(ids); i++ {
		if !s.HeadOverBudget(limit) {
			t.Fatalf("call %d past the budget would run", i+2)
		}
		refused, e := s.RefuseOverBudget(limit)
		must(t, e)
		if !refused {
			t.Fatal("over-budget head not refused")
		}
		if _, e := RestoreSession(s.Export()); e != nil {
			t.Fatalf("a partly refused batch does not restore: %v", e)
		}
	}
	if refused, _ := s.RefuseOverBudget(limit); refused || s.Status() != StatusStreaming {
		t.Fatalf("refused past the queue; status %s", s.Status())
	}
	activity := s.Export().Activity
	last := activity[len(activity)-1].Outcome
	if activity[len(activity)-1].Decision != DecisionDeny || !last.IsError || !strings.HasPrefix(string(last.Content), OverBudgetOutcomePrefix) || !strings.Contains(string(last.Content), "7 of this answer's 12 calls") || !strings.Contains(string(last.Content), "8 calls") {
		t.Fatalf("overflow outcome %+v", last)
	}
	if count := s.Export().TurnCallCount; count != limit {
		t.Fatalf("turn count %d counts calls that never ran", count)
	}
	if left, _ := s.CallsLeft(limit); left != 0 {
		t.Fatalf("left %d", left)
	}
	restored, e := RestoreSession(s.Export())
	if e != nil || restored.Export().TurnCallCount != limit {
		t.Fatalf("restore: %v", e)
	}
}

// With no call left the whole answer is refused at once and the turn
// pauses, as before; the refusals carry the same prefix.
func TestAnAnswerWithNoCallLeftIsRefusedWhole(t *testing.T) {
	const limit = 8
	s := session(t)
	must(t, s.BeginTurn("go", catalog(t)))
	first := make([]string, limit)
	for i := range first {
		first[i] = fmt.Sprintf("a%d", i)
	}
	must(t, s.CompleteAssistantWithin(completion(t, first...), limit))
	for _, id := range first {
		must(t, s.RecordToolOutcome(axlr.ToolCallID(id), DecisionApprove, ToolOutcome{Content: "ok"}))
	}
	if e := s.CompleteAssistantWithin(completion(t, "b0", "b1"), limit); e != ErrToolCallLimit || s.Status() != StatusInterrupted {
		t.Fatalf("%v %s", e, s.Status())
	}
	last := s.Export().Activity[len(s.Export().Activity)-1].Outcome
	if !strings.HasPrefix(string(last.Content), OverBudgetOutcomePrefix) || !strings.Contains(string(last.Content), "none of this answer's 2 calls") {
		t.Fatalf("%q", last.Content)
	}
	if s.Export().TurnCallCount != limit {
		t.Fatalf("turn count %d", s.Export().TurnCallCount)
	}
	if _, e := RestoreSession(s.Export()); e != nil {
		t.Fatal(e)
	}
}
