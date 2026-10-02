package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func execApprovalModel(t *testing.T, mode domain.WorkMode, arguments string) AppModel {
	t.Helper()
	s := navSession(t)
	if err := s.SetMode(mode); err != nil {
		t.Fatal(err)
	}
	args, _ := root.NewJSONValue([]byte(arguments))
	schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
	s.BeginTurn("run", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "local_exec", Description: "exec", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "exec"}}})
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "one", Name: "local_exec", Arguments: args}}}}); err != nil {
		t.Fatal(err)
	}
	return navModel(t, &s)
}

func TestTheCardSaysWhenAnExecRunsCodeTheArgumentsHide(t *testing.T) {
	m := execApprovalModel(t, domain.ModeNormal, `{"program":"python3","args":["-"],"stdin":"import os\nos.remove('x')\n"}`)
	defer m.zones.Close()
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "runs 2 lines of code from stdin") {
		t.Fatalf("stdin payload not flagged:\n%s", plain)
	}
	inline := execApprovalModel(t, domain.ModeNormal, `{"program":"python3","args":["-c","print(1)"]}`)
	defer inline.zones.Close()
	if plain := ansi.Strip(inline.View().Content); !strings.Contains(plain, "runs inline code") {
		t.Fatalf("inline code not flagged:\n%s", plain)
	}
	plain := execApprovalModel(t, domain.ModeNormal, `{"program":"ls","args":["-la"]}`)
	defer plain.zones.Close()
	if strings.Contains(ansi.Strip(plain.View().Content), "runs ") {
		t.Fatal("an ordinary command was flagged")
	}
}

func TestAModeThatAsksDoesNotOfferToStopAsking(t *testing.T) {
	m := execApprovalModel(t, domain.ModeReview, `{"program":"python3","args":["-m","unittest"]}`)
	defer m.zones.Close()
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "a approve") || strings.Contains(view, "l always") || strings.Contains(view, "f ") {
		t.Fatalf("review mode card offers always-allow or autonomy:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.Busy || len(m.deps.Session.Pending()) != 1 {
		t.Fatal("l still acted under a mode that asks for each call")
	}
	normal := execApprovalModel(t, domain.ModeNormal, `{"program":"python3","args":["-m","unittest"]}`)
	defer normal.zones.Close()
	if !strings.Contains(ansi.Strip(normal.View().Content), "l always") {
		t.Fatal("normal mode lost always-allow")
	}
}
