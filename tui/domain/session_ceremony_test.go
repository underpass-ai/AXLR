package domain

import (
	"fmt"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func debugRun() CeremonyRun {
	return CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "axlr-1", Step: "reproduce", Iteration: 1, Fence: "f1", About: "ws:0123456789abcdef0123456789abcdef", Check: CheckCommand{Program: "python3", Args: []string{"-m", "unittest"}}}
}

func TestCeremonyRunSurvivesRestoreAndIsCopied(t *testing.T) {
	s := idleSession(t)
	if err := s.SetMode(ModeDebug); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCeremony(debugRun()); err != nil {
		t.Fatal(err)
	}
	run, ok := s.Ceremony()
	run.Check.Args[0] = "-c"
	again, _ := s.Ceremony()
	if !ok || again.Check.Args[0] != "-m" {
		t.Fatal("ceremony run leaked a mutable slice")
	}
	restored, err := RestoreSession(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := restored.Ceremony(); !ok || got.Instance != "axlr-1" || !got.Check.Equal(debugRun().Check) {
		t.Fatalf("restore lost the run: %+v", got)
	}
	broken := s.Export()
	broken.Ceremony.Step = ""
	if _, err := RestoreSession(broken); err == nil {
		t.Fatal("restored an invalid run")
	}
}

func TestFinishingACeremonyMidTurnReturnsToNormal(t *testing.T) {
	s := idleSession(t)
	if err := s.SetMode(ModeDebug); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn(axlr.Text("arregla"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCeremony(debugRun()); err != nil {
		t.Fatal(err)
	}
	s.FinishCeremony()
	if _, ok := s.Ceremony(); ok || s.Mode() != ModeNormal {
		t.Fatalf("ceremony not finished: mode %q", s.Mode())
	}
}

func TestCeremonyModesLimitNoLocalTool(t *testing.T) {
	write, _ := NewLocalToolIdentity("write")
	exec, _ := NewLocalToolIdentity("exec")
	args, _ := axlr.NewJSONObject([]byte(`{"path":"wc.py","content":"x"}`))
	for _, mode := range []WorkMode{ModeDebug, ModeDelivery} {
		for _, id := range []ToolIdentity{write, exec} {
			if verdict, _ := mode.Judge(id, args); verdict != VerdictAllow {
				t.Fatalf("%s limits %s", mode, id.LocalOperation)
			}
		}
		if !mode.StartsCeremony() {
			t.Fatalf("%s does not start a ceremony", mode)
		}
	}
	if ModeReview.StartsCeremony() || ModeNormal.StartsCeremony() {
		t.Fatal("non-ceremony mode starts a ceremony")
	}
	if _, err := NewHostToolIdentity(HostOperationStepDone); err != nil {
		t.Fatal(err)
	}
}

func TestCeremonyStepsGetAFreshCallBudgetThatSurvivesRestore(t *testing.T) {
	s := idleSession(t)
	tool := AvailableTool{Definition: axlr.ToolDefinition{Name: "local_read", Description: "x"}, Identity: ToolIdentity{Kind: ToolKindLocal, LocalOperation: "read"}}
	tool.Definition.Parameters, _ = axlr.NewJSONObject([]byte(`{"type":"object"}`))
	if err := s.BeginTurn(axlr.Text("depura"), []AvailableTool{tool}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCeremony(debugRun()); err != nil {
		t.Fatal(err)
	}
	args, _ := axlr.NewJSONObject([]byte(`{}`))
	call := func(n int) {
		calls := make([]axlr.ToolCall, n)
		for i := range calls {
			calls[i] = axlr.ToolCall{ID: axlr.ToolCallID(fmt.Sprintf("c%d-%d", s.Export().TurnCallCount, i)), Name: "local_read", Arguments: args}
		}
		if err := s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, ToolCalls: calls}}); err != nil {
			t.Fatal(err)
		}
		for _, c := range calls {
			if err := s.RecordToolOutcome(c.ID, DecisionAutoApprove, ToolOutcome{Content: "ok"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	call(30)
	s.RestartTurnBudget()
	call(30)
	if s.Export().TurnCallCount != 60 {
		t.Fatalf("count %d", s.Export().TurnCallCount)
	}
	restored, err := RestoreSession(s.Export())
	if err != nil {
		t.Fatalf("a long ceremony turn no longer restores: %v", err)
	}
	run, _ := restored.Ceremony()
	if run.BudgetBase != 30 {
		t.Fatalf("base %d", run.BudgetBase)
	}
}

func TestTheAnswerAfterALongCeremonyIsNotCountedAgainstItsSteps(t *testing.T) {
	s := idleSession(t)
	tool := AvailableTool{Definition: axlr.ToolDefinition{Name: "local_read", Description: "x"}, Identity: ToolIdentity{Kind: ToolKindLocal, LocalOperation: "read"}}
	tool.Definition.Parameters, _ = axlr.NewJSONObject([]byte(`{"type":"object"}`))
	if err := s.BeginTurn(axlr.Text("depura"), []AvailableTool{tool}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCeremony(debugRun()); err != nil {
		t.Fatal(err)
	}
	args, _ := axlr.NewJSONObject([]byte(`{}`))
	calls := make([]axlr.ToolCall, 30)
	for i := range calls {
		calls[i] = axlr.ToolCall{ID: axlr.ToolCallID(fmt.Sprintf("c%d", i)), Name: "local_read", Arguments: args}
	}
	if err := s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, ToolCalls: calls}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range calls {
		if err := s.RecordToolOutcome(c.ID, DecisionAutoApprove, ToolOutcome{Content: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	s.RestartTurnBudget()
	s.FinishCeremony()
	five := calls[:5]
	for i := range five {
		five[i].ID = axlr.ToolCallID(fmt.Sprintf("d%d", i))
	}
	if err := s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, ToolCalls: five}}); err != nil {
		t.Fatalf("the turn after the ceremony hit the limit: %v", err)
	}
}
