package domain

import (
	"fmt"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func budgetSession(t *testing.T) Session {
	t.Helper()
	s, err := NewSession("0123456789abcdef0123456789abcdef", Workspace(t.TempDir()), "test/model")
	must(t, err)
	must(t, s.BeginTurn("go", catalog(t)))
	return s
}

// answerCalls completes one answer with n calls and records their results.
func answerCalls(t *testing.T, s *Session, limit int, prefix string, n int) error {
	t.Helper()
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	if err := s.CompleteAssistantWithin(completion(t, ids...), limit); err != nil {
		return err
	}
	for _, id := range ids {
		must(t, s.RecordToolOutcome(axlr.ToolCallID(id), DecisionApprove, ToolOutcome{Content: "ok"}))
	}
	return nil
}

func TestTheTurnLimitIsTheConfiguredOne(t *testing.T) {
	s := budgetSession(t)
	must(t, answerCalls(t, &s, 10, "a", 9))
	if left, limit := s.CallsLeft(10); left != 1 || limit != 10 {
		t.Fatalf("left %d of %d", left, limit)
	}
	must(t, answerCalls(t, &s, 10, "b", 1))
	if err := s.CompleteAssistantWithin(completion(t, "c0", "c1"), 10); err != ErrToolCallLimit {
		t.Fatalf("over the configured limit: %v", err)
	}
	// A compact step keeps its smaller budget under a larger turn limit, and
	// a smaller turn limit bounds it.
	run := CeremonyRun{Compact: true}
	if run.StepCallLimit(64) != CompactStepCalls || run.StepCallLimit(10) != 10 || (CeremonyRun{}).StepCallLimit(64) != 64 {
		t.Fatal("step limits")
	}
}

// A budget note joins the running turn without restarting the budget, and a
// saved transcript with one replays to the same count.
func TestABudgetNoteJoinsTheTurnWithoutRestartingTheBudget(t *testing.T) {
	s := budgetSession(t)
	must(t, answerCalls(t, &s, MaxTurnToolCalls, "a", 3))
	if err := s.Note("plain text"); err == nil {
		t.Fatal("a note that is not a budget note was accepted")
	}
	must(t, s.Note(BudgetNotePrefix+" 2 tool calls left in this turn."))
	must(t, answerCalls(t, &s, MaxTurnToolCalls, "b", 2))
	if count := s.Export().TurnCallCount; count != 5 {
		t.Fatalf("turn count %d after a note", count)
	}
	restored, err := RestoreSession(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	if restored.Export().TurnCallCount != 5 {
		t.Fatalf("restored count %d", restored.Export().TurnCallCount)
	}
	must(t, s.CompleteAssistantWithin(completion(t, "c0"), MaxTurnToolCalls))
	if err := s.Note(BudgetNotePrefix + " late"); err == nil {
		t.Fatal("a note was accepted with calls pending")
	}
}

func TestACeremonyStepResumesAtTheLimitTwicePerStep(t *testing.T) {
	s := budgetSession(t)
	must(t, s.SetCeremony(CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "i", Step: "repair", Iteration: 1}))
	if _, ok, err := s.ResumeStepAtLimit(2); ok || err != nil {
		t.Fatalf("a streaming turn was resumed: %v", err)
	}
	// Spend the step's budget, then answer once more: the whole answer is
	// refused and the turn pauses.
	overrun := func(prefix string) {
		t.Helper()
		must(t, answerCalls(t, &s, 2, prefix+"a", 2))
		if err := s.CompleteAssistantWithin(completion(t, prefix+"0", prefix+"1", prefix+"2"), 2); err != ErrToolCallLimit {
			t.Fatalf("limit: %v", err)
		}
	}
	for want := 1; want <= 2; want++ {
		overrun(fmt.Sprintf("x%d-", want))
		resumes, ok, err := s.ResumeStepAtLimit(2)
		if !ok || err != nil || resumes != want || s.Status() != StatusStreaming {
			t.Fatalf("resume %d: %d %v %v %s", want, resumes, ok, err, s.Status())
		}
		if left, _ := s.CallsLeft(2); left != 2 {
			t.Fatalf("resume %d left %d calls", want, left)
		}
	}
	overrun("y")
	if resumes, ok, _ := s.ResumeStepAtLimit(2); ok || resumes != 2 || s.Status() != StatusInterrupted {
		t.Fatalf("third resume: %d %v %s", resumes, ok, s.Status())
	}
	// The next attempt of the step gets its own two.
	run, _ := s.Ceremony()
	run.Iteration = 2
	must(t, s.SetCeremony(run))
	if resumes, ok, err := s.ResumeStepAtLimit(2); !ok || err != nil || resumes != 1 {
		t.Fatalf("next attempt: %d %v %v", resumes, ok, err)
	}
	// A step that waits for the person is never resumed by the console.
	overrun("z")
	run, _ = s.Ceremony()
	run.Incident = &IncidentRun{Awaiting: "approval"}
	must(t, s.SetCeremony(run))
	if _, ok, _ := s.ResumeStepAtLimit(2); ok {
		t.Fatal("a step awaiting the person was resumed")
	}
}
