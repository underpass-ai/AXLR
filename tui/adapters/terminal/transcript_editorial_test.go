package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func editorialState(pendingLast bool) domain.SessionState {
	call := func(id, path string) root.ToolCall {
		args, _ := root.NewJSONObject([]byte(`{"path":"` + path + `"}`))
		return root.ToolCall{ID: root.ToolCallID(id), Name: "read", Arguments: args}
	}
	a, b, c := call("a", "x.md"), call("b", "y.md"), call("c", "z.md")
	s := domain.SessionState{
		Messages: []root.Message{
			{Role: root.RoleUser, Content: "lee los ficheros"},
			{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{a, b}},
			{Role: root.RoleTool, ToolCallID: "a", Content: `{"status":"completed","duration_ms":400}`},
			{Role: root.RoleTool, ToolCallID: "b", Content: `{"status":"failed","duration_ms":100}`},
			{Role: root.RoleAssistant, Content: "Uno ha fallado; reintento.", ToolCalls: []root.ToolCall{c}},
		},
		Activity: []domain.PendingTool{{Call: a, Decision: domain.DecisionAutoApprove}, {Call: b, Decision: domain.DecisionAutoApprove}, {Call: c}},
	}
	if !pendingLast {
		s.Activity[2].Decision = domain.DecisionAutoApprove
		s.Messages = append(s.Messages, root.Message{Role: root.RoleTool, ToolCallID: "c", Content: "ok"})
	}
	return s
}

func TestEditorialLayoutLabelsSpeakersAndSummarisesFinishedToolRuns(t *testing.T) {
	tr := NewTranscript()
	tr.SetWidth(100)
	tr.SetSession(editorialState(true), "", Theme{ID: domain.ThemeEditorial, Locale: Spanish})
	got := ansi.Strip(tr.Text())
	want := "Tú\nlee los ficheros\n\nAXLR\n✗ usó read ×2 · 77\u00a0B · 500\u00a0ms · fallidas: 1\n\nUno ha fallado; reintento.\n\n◌ read  z.md · esperando aprobación"
	if got != want {
		t.Fatalf("editorial transcript:\n%q\nwant:\n%q", got, want)
	}
}

func TestMarginLayoutIsUnchangedByTheEditorialCode(t *testing.T) {
	tr := NewTranscript()
	tr.SetWidth(100)
	tr.SetSession(editorialState(false), "", Theme{ID: domain.ThemeInk})
	got := ansi.Strip(tr.Text())
	if strings.Contains(got, "AXLR") || strings.Contains(got, "used") || !strings.Contains(got, "› lee los ficheros") || strings.Count(got, "read") != 3 {
		t.Fatalf("Margen layout changed:\n%s", got)
	}
}

func TestEditorialThemeIsSelectableAndValid(t *testing.T) {
	if err := domain.ThemeEditorial.Validate(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range NewThemePicker(domain.DefaultUIPreferences(), English).List.Items() {
		if choice, ok := item.(themeChoice); ok && choice.id == domain.ThemeEditorial {
			found = true
		}
	}
	if !found {
		t.Fatal("Editorial is not offered in /theme")
	}
}

func TestEditorialApprovalIsASheetWithClickableActions(t *testing.T) {
	m := approvalModel(t)
	m.Theme.ID = domain.ThemeEditorial
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "AXLR wants to run local write") || strings.Contains(plain, "Confirm tool") || !strings.Contains(plain, "a approve") {
		t.Fatalf("sheet missing:\n%s", plain)
	}
	n, c := click(t, m, "deny")
	m = drain(t, n.(AppModel), c)
	if m.deps.Session.Export().Activity[0].Decision != domain.DecisionDeny {
		t.Fatal("sheet action is not clickable")
	}
}

func TestEditorialKeepsConsecutivePromptsApart(t *testing.T) {
	tr := NewTranscript()
	tr.SetWidth(80)
	tr.SetSession(domain.SessionState{Messages: []root.Message{{Role: root.RoleUser, Content: "hola"}, {Role: root.RoleUser, Content: "otra"}}}, "", Theme{ID: domain.ThemeEditorial})
	if got := ansi.Strip(tr.Text()); got != "You\nhola\n\notra" {
		t.Fatalf("%q", got)
	}
}
