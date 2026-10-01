package terminal

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func toolRowState(decision domain.ToolDecision, result string, outcome *domain.ToolOutcome) domain.SessionState {
	args, _ := root.NewJSONObject([]byte(`{"path":"README.md"}`))
	call := root.ToolCall{ID: "c1", Name: "read", Arguments: args}
	s := domain.SessionState{
		Messages: []root.Message{{Role: root.RoleUser, Content: "read it"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call}}},
		Activity: []domain.PendingTool{{Call: call, Decision: decision, Outcome: outcome}},
	}
	if result != "" {
		s.Messages = append(s.Messages, root.Message{Role: root.RoleTool, ToolCallID: "c1", Content: root.Text(result)})
	}
	return s
}

func renderedToolRow(t *testing.T, s domain.SessionState) string {
	t.Helper()
	tr := NewTranscript()
	tr.SetWidth(120)
	tr.SetSession(s, "", Theme{ID: domain.ThemeInk})
	lines := strings.Split(ansi.Strip(tr.Text()), "\n")
	return lines[len(lines)-1]
}

func TestToolRowMergesCallDecisionAndResult(t *testing.T) {
	cases := []struct {
		name     string
		state    domain.SessionState
		want     []string
		unwanted []string
	}{
		{"automatic approval is silent", toolRowState(domain.DecisionAutoApprove, `{"status":"completed","duration_ms":1400}`, nil), []string{"✓ read", "README.md", "41\u00a0B", "1.4\u00a0s"}, []string{"auto", "approved", "path", "{"}},
		{"human approval is shown", toolRowState(domain.DecisionApprove, `ok`, nil), []string{"✓ read", "2\u00a0B", "approved by you"}, nil},
		{"denied", toolRowState(domain.DecisionDeny, "", nil), []string{"✗ read", "denied"}, []string{"✓"}},
		{"failed result", toolRowState(domain.DecisionAutoApprove, `{"status":"failed","duration_ms":4}`, nil), []string{"✗ read", "4\u00a0ms"}, nil},
		{"error outcome", toolRowState(domain.DecisionAutoApprove, `boom`, &domain.ToolOutcome{IsError: true}), []string{"✗ read"}, nil},
		{"awaiting approval", toolRowState("", "", nil), []string{"◌ read", "waiting for approval"}, nil},
		{"running", toolRowState(domain.DecisionAutoApprove, "", nil), []string{"◌ read", "running"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := renderedToolRow(t, tc.state)
			for _, want := range tc.want {
				if !strings.Contains(row, want) {
					t.Fatalf("row %q lacks %q", row, want)
				}
			}
			for _, unwanted := range tc.unwanted {
				if strings.Contains(row, unwanted) {
					t.Fatalf("row %q shows %q", row, unwanted)
				}
			}
		})
	}
}

func TestToolRowsStayTogetherAndInspectOnlyTheHeadOfLargeResults(t *testing.T) {
	s := toolRowState(domain.DecisionAutoApprove, `{"status":"completed"}`+strings.Repeat(" ", 700_000)+`"duration_ms":9`, nil)
	second := root.ToolCall{ID: "c2", Name: "read"}
	s.Messages[1].ToolCalls = append(s.Messages[1].ToolCalls, second)
	s.Activity = append(s.Activity, domain.PendingTool{Call: second, Decision: domain.DecisionAutoApprove})
	tr := NewTranscript()
	tr.SetWidth(120)
	tr.SetSession(s, "", Theme{Monochrome: true})
	text := tr.Text()
	if strings.Contains(text, "9\u00a0ms") || !strings.Contains(text, "683.6\u00a0KB") {
		t.Fatalf("large result not summarised from its head: %q", text)
	}
	if !strings.Contains(text, "KB\n[WAIT] read") {
		t.Fatalf("consecutive tool rows are separated: %q", text)
	}
}

func TestUserRowsHangContinuationLinesUnderTheText(t *testing.T) {
	tr := NewTranscript()
	tr.SetWidth(20)
	tr.SetSession(domain.SessionState{Messages: []root.Message{{Role: root.RoleUser, Content: "una pregunta bastante larga\nsegunda línea"}}}, "", Theme{})
	lines := strings.Split(ansi.Strip(tr.Viewport.GetContent()), "\n")
	if !strings.HasPrefix(lines[0], "› ") {
		t.Fatalf("first line lacks the marker: %q", lines)
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "  ") || ansi.StringWidth(line) > 20 {
			t.Fatalf("continuation not hung under the text: %q", lines)
		}
	}
	if tr.VisualLineCount() != len(lines) {
		t.Fatalf("visual rows %d != lines %d", tr.VisualLineCount(), len(lines))
	}
}
