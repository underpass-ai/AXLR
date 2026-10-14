package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func execCallsModel(t *testing.T, mode domain.WorkMode, arguments ...string) AppModel {
	t.Helper()
	s := navSession(t)
	if err := s.SetMode(mode); err != nil {
		t.Fatal(err)
	}
	schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
	s.BeginTurn("run", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "local_exec", Description: "exec", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "exec"}}})
	var calls []root.ToolCall
	for i, raw := range arguments {
		args, _ := root.NewJSONValue([]byte(raw))
		calls = append(calls, root.ToolCall{ID: root.ToolCallID(string(rune('a' + i))), Name: "local_exec", Arguments: args})
	}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: calls}}); err != nil {
		t.Fatal(err)
	}
	m := navModel(t, &s)
	m.deps.Resolve.Store = m.deps.Store
	m.deps.Resolve.Tools = navTool{}
	m.deps.Resolve.Continue.Models = navStream{}
	return m
}

// S on a local command's card approves it and allows that exact command
// for the rest of the session, also in review mode, where it is the only
// allow; the same command with another argument still asks. A new session
// forgets it.
func TestSAllowsTheExactCommandForTheSession(t *testing.T) {
	m := execCallsModel(t, domain.ModeReview, `{"program":"git","args":["log"]}`, `{"program":"git","args":["log"],"cwd":".","timeout_ms":5000}`, `{"program":"git","args":["log","--output=x"]}`)
	defer m.zones.Close()
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "s allow this command in this session") || strings.Contains(view, "l always") {
		t.Fatalf("review card:\n%s", view)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = drain(t, next.(AppModel), cmd)
	activity := m.deps.Session.Export().Activity
	if activity[0].Decision != domain.DecisionApprove || activity[1].Decision != domain.DecisionAutoApprove || activity[2].Outcome != nil {
		t.Fatalf("error %q, activity %+v", m.Status.Error, activity)
	}
	if pending := m.deps.Session.Pending(); len(pending) != 1 || pending[0].Call.ID != "c" {
		t.Fatalf("another command was allowed: %+v", pending)
	}
	m.allowed.reset()
	if policy := m.deps.Agent.Approval.(application.CallApprovalPort); policy.ApprovesCall(m.Header.State.ID, domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "exec"}, activity[0].Call.Arguments) {
		t.Fatal("a reset kept the allow")
	}
	write := approvalModel(t)
	defer write.zones.Close()
	if view := ansi.Strip(write.View().Content); strings.Contains(view, "allow this command") {
		t.Fatalf("a write card offers S:\n%s", view)
	}
	if write = update(write, tea.KeyPressMsg{Code: 's', Text: "s"}); write.Busy {
		t.Fatal("S acted on a write")
	}
}

func TestExecAllowKeyIsExact(t *testing.T) {
	key := func(raw string) string { k, _ := execAllowKey([]byte(raw)); return k }
	base := key(`{"program":"go","args":["test","./..."]}`)
	for _, same := range []string{`{"args":["test","./..."],"program":"go","cwd":"./","timeout_ms":9,"max_output_bytes":10,"stdin":""}`, `{"program":"go","args":["test","./..."],"cwd":"."}`, `{"program":"go","args":["test","./..."],"why":"Run the test suite"}`, `{"program":"go","args":["test","./..."],"why":"Run the exact go test ./... command again"}`} {
		if key(same) != base {
			t.Fatalf("%s differs", same)
		}
	}
	for _, other := range []string{`{"program":"go","args":["test"]}`, `{"program":"go","args":["test","./..."],"cwd":"sub"}`, `{"program":"go","args":["test","./..."],"stdin":"x"}`, `{"program":"go","args":["test","./..."],"env":{}}`} {
		if key(other) == base {
			t.Fatalf("%s matches", other)
		}
	}
	if _, ok := execAllowKey([]byte(`{"args":["x"]}`)); ok {
		t.Fatal("a call without a program has a key")
	}
}
