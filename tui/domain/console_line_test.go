package domain

import (
	"reflect"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func finishedPlan() PlanRecord {
	return PlanRecord{ID: "add-a-func-dce953", Session: "0123456789abcdef0123456789abcdef", Status: PlanPartial,
		Tasks: []PlanTask{
			{ID: "short", Status: TaskDone, Session: "11111111111111111111111111111111", Handback: &TaskHandback{Changed: []string{"buildinfo/version.go", "buildinfo/version_test.go"}}},
			{ID: "docs", Status: TaskDone, Handback: &TaskHandback{Changed: []string{"buildinfo/version.go", "docs/console.md"}}},
			{ID: "cli", Status: TaskBlocked},
			{ID: "flag", Status: TaskSkipped},
		},
		Syncs: []SyncRecord{{Wave: 1, Verdict: "green"}, {Wave: 2, Verdict: "blocked"}},
	}
}

func TestAPlanOutcomeSummarisesTheRecord(t *testing.T) {
	got := NewPlanOutcome(finishedPlan())
	want := PlanOutcome{PlanID: "add-a-func-dce953", Status: PlanPartial, Tasks: 4, Done: 2, Blocked: 1, Skipped: 1,
		Syncs:   []SyncVerdict{{Wave: 1, Verdict: "green"}, {Wave: 2, Verdict: "blocked"}},
		Changed: []string{"buildinfo/version.go", "buildinfo/version_test.go", "docs/console.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outcome %+v", got)
	}
	for status, reported := range map[PlanStatus]bool{PlanDone: true, PlanPartial: true, PlanBlocked: true, PlanDeclined: false, PlanInterrupted: false, PlanRunning: false} {
		if PlanOutcomeReported(status) != reported {
			t.Fatalf("%s reported: %v", status, !reported)
		}
	}
}

func TestAPlanOutcomeIsAConsoleLineOutsideTheModelsHistory(t *testing.T) {
	s := idleSession(t)
	if err := s.BeginTurn("plan it", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "It's waiting for your approval on the plan card."}}); err != nil {
		t.Fatal(err)
	}
	messages := s.Messages()
	outcome := NewPlanOutcome(finishedPlan())
	if added, err := s.AddPlanOutcome(outcome); !added || err != nil {
		t.Fatalf("add: %v %v", added, err)
	}
	if added, err := s.AddPlanOutcome(outcome); added || err != nil {
		t.Fatalf("a plan's outcome is added once: %v %v", added, err)
	}
	if !reflect.DeepEqual(s.Messages(), messages) || s.Status() != StatusComplete {
		t.Fatal("a console line changed the model's history or the session's state")
	}
	lines := s.ConsoleLines()
	if len(lines) != 1 || lines[0].AfterMessage != 2 || lines[0].At.IsZero() || lines[0].Plan.PlanID != "add-a-func-dce953" {
		t.Fatalf("lines %+v", lines)
	}
	lines[0].Plan.Changed[0] = "changed"
	if s.ConsoleLines()[0].Plan.Changed[0] != "buildinfo/version.go" {
		t.Fatal("console lines leaked a mutable slice")
	}
	restored, err := RestoreSession(s.Export())
	if err != nil || !reflect.DeepEqual(restored.ConsoleLines(), s.ConsoleLines()) {
		t.Fatalf("restore: %v", err)
	}
	// The next turn keeps the line where it was.
	if err := restored.BeginTurn("thanks", nil); err != nil || restored.ConsoleLines()[0].AfterMessage != 2 {
		t.Fatalf("next turn: %v", err)
	}
	bad := s.Export()
	bad.ConsoleLines[0].AfterMessage = 3
	if _, err := RestoreSession(bad); err == nil {
		t.Fatal("a line past the transcript was restored")
	}
	if _, err := s.AddPlanOutcome(PlanOutcome{PlanID: "x", Status: PlanRunning}); err == nil {
		t.Fatal("an unfinished plan was added")
	}
	busy := idleSession(t)
	if err := busy.BeginTurn("go", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := busy.AddPlanOutcome(outcome); err == nil {
		t.Fatal("a console line was added during a turn")
	}
}

// A failed request leaves a console line that survives a restore; it needs
// a message on one line, and is refused during a turn.
func TestATurnFailureIsAConsoleLine(t *testing.T) {
	s := idleSession(t)
	if err := s.BeginTurn("hola", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTurnFailure(TurnFailure{Message: "OpenRouter invalid_request (HTTP 400): not a valid model ID"}); err == nil {
		t.Fatal("a failure was added during a turn")
	}
	if err := s.InterruptDraft(""); err != nil {
		t.Fatal(err)
	}
	messages := s.Messages()
	failure := TurnFailure{Message: "OpenRouter invalid_request (HTTP 400): not a valid model ID", Hint: "choose_model"}
	if err := s.AddTurnFailure(failure); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []TurnFailure{{}, {Message: " "}, {Message: "two\nlines"}, {Message: string(make([]rune, MaxTurnFailureRunes+1))}} {
		if err := s.AddTurnFailure(bad); err == nil {
			t.Fatalf("added %q", bad.Message)
		}
	}
	lines := s.ConsoleLines()
	if !reflect.DeepEqual(s.Messages(), messages) || len(lines) != 1 || lines[0].AfterMessage != 1 || *lines[0].Failure != failure {
		t.Fatalf("lines %+v", lines)
	}
	restored, err := RestoreSession(s.Export())
	if err != nil || !reflect.DeepEqual(restored.ConsoleLines(), lines) {
		t.Fatalf("restore: %v", err)
	}
	two := s.Export()
	two.ConsoleLines[0].Plan = &PlanOutcome{PlanID: "add-a-func-dce953", Status: PlanDone}
	if _, err := RestoreSession(two); err == nil {
		t.Fatal("a line with two kinds was restored")
	}
}
