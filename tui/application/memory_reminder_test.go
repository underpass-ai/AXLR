package application

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// memoryTools is a session snapshot with local tools, the host tools and
// KMP's wake and write tools.
func memoryTools(t *testing.T) []domain.AvailableTool {
	t.Helper()
	return append(repairTools(t), hostPlugin(t, "kmp_write_memory", "kmp", "kmp_write_memory"))
}

func memoryRequest(t *testing.T, calls ...root.ToolCall) []root.Message {
	t.Helper()
	request := []root.Message{{Role: root.RoleUser, Content: "fix the parser"}}
	if len(calls) > 0 {
		request = append(request, root.Message{Role: root.RoleAssistant, ToolCalls: calls})
	}
	return append(request, root.Message{Role: root.RoleAssistant, Content: "done"})
}

func toolCall(t *testing.T, id string, name root.ToolName, arguments string) root.ToolCall {
	t.Helper()
	return root.ToolCall{ID: root.ToolCallID(id), Name: name, Arguments: mustObject(t, arguments)}
}

func TestMemoryReminderNeedsDurableWorkAndNoWrite(t *testing.T) {
	edit := toolCall(t, "e1", "local_edit", `{}`)
	reads := []root.ToolCall{}
	for _, id := range []string{"r1", "r2", "r3", "r4", "r5"} {
		reads = append(reads, toolCall(t, id, "local_read", `{}`))
	}
	for _, tc := range []struct {
		name    string
		request []root.Message
		want    bool
	}{
		{"a file change", memoryRequest(t, edit), true},
		{"five calls", memoryRequest(t, reads...), true},
		{"a question", memoryRequest(t, reads[:2]...), false},
		{"bridged write", memoryRequest(t, edit, toolCall(t, "w1", HostCallToolName, `{"name":"kmp_write_memory","arguments":{}}`)), false},
		{"direct write", memoryRequest(t, edit, toolCall(t, "w1", "kmp_write_memory", `{}`)), false},
		{"a ceremony", memoryRequest(t, edit, toolCall(t, "s1", HostStepDoneName, `{}`)), false},
		{"already reminded", append(memoryRequest(t, edit), root.Message{Role: root.RoleUser, Content: memoryReminder}, root.Message{Role: root.RoleAssistant, Content: "nothing durable"}), false},
	} {
		if got := needsMemoryReminder(tc.request, "kmp_write_memory"); got != tc.want {
			t.Fatalf("%s: got %v", tc.name, got)
		}
	}
	messages := []root.Message{{Role: root.RoleUser, Content: "first"}, {Role: root.RoleAssistant, Content: "a"}, {Role: root.RoleUser, Content: "second\n\n[AXLR] Self-repair r merged."}, {Role: root.RoleUser, Content: "[AXLR · Jev] doubt"}}
	if personRequest(messages) != 2 {
		t.Fatal("a console note is not the person's request; a note appended to a prompt is")
	}
}

func TestModelIsToldToRecordOnlyWhereItRecordsMemory(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("fix the parser", memoryTools(t)); err != nil {
		t.Fatal(err)
	}
	text := string(modelHostGuidance(&s).Content)
	if !strings.Contains(text, "Record what the work settles") || !strings.Contains(text, "kmp_write_memory = kmp_write_memory") {
		t.Fatalf("normal mode guidance: %s", text)
	}
	if err := s.CompleteAssistant(assistant("done")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeIncident); err != nil {
		t.Fatal(err)
	}
	if modelRecordsMemory(s) || strings.Contains(string(modelHostGuidance(&s).Content), "Record what the work settles") {
		t.Fatal("incident mode refuses the model's memory writes; it must not be told to record")
	}
	noKMP := turnSession(t)
	if err := noKMP.BeginTurn("fix the parser", repairTools(t)); err != nil {
		t.Fatal(err)
	}
	if modelRecordsMemory(noKMP) {
		t.Fatal("without kmp_write_memory there is nothing to remind")
	}
}

func TestMemoryReminderAndJevEachActOncePerRequest(t *testing.T) {
	stub := &judgeStub{verdict: JudgementVerdict{Yes: yes(0.2)}}
	var lasts []string
	u := agentWith(&Judge{Port: stub, FinalCheck: true}, func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		last := string(r.Messages[len(r.Messages)-1].Content)
		lasts = append(lasts, last)
		switch len(lasts) {
		case 1:
			return assistant("", root.ToolCall{ID: "e1", Name: "local_edit", Arguments: mustObject(t, `{}`)}), nil
		case 2:
			return assistant("Done: the parser splits on whitespace."), nil
		case 3:
			if !strings.HasPrefix(last, jevFinalPrefix) {
				t.Fatalf("third request should carry Jev's doubt: %q", last)
			}
			return assistant("Done, verified."), nil
		case 4:
			if !strings.HasPrefix(last, memoryReminderPrefix) {
				t.Fatalf("fourth request should carry the memory reminder: %q", last)
			}
			return assistant("Nothing durable to record: a one-line fix."), nil
		}
		t.Fatalf("an extra model turn after %q", last)
		return root.CompletionResult{}, nil
	})
	u.Approval = approvesAll{}
	u.Tools = toolFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		return domain.ToolOutcome{Content: `{"status":"completed","output":{}}`}, nil
	})
	s := turnSession(t)
	if err := s.BeginTurn("fix the parser", memoryTools(t)); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if len(lasts) != 4 || len(stub.questions) != 1 || s.Status() != domain.StatusComplete {
		t.Fatalf("turns=%d judged=%d status=%s", len(lasts), len(stub.questions), s.Status())
	}
	if !strings.Contains(stub.questions[0].State, "fix the parser") {
		t.Fatalf("Jev judged another request: %s", stub.questions[0].State)
	}
}
