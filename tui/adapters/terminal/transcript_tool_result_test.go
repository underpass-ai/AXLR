package terminal

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// resultSession has a command, a read, an edit, a plugin call and a denied
// call, each with what it returned.
func resultSession(t *testing.T) domain.SessionState {
	t.Helper()
	value := func(raw string) root.JSONValue { v, _ := root.NewJSONValue([]byte(raw)); return v }
	tools := []domain.AvailableTool{}
	for _, op := range []string{"exec", "read", "edit"} {
		tools = append(tools, domain.AvailableTool{Definition: root.ToolDefinition{Name: root.ToolName("local_" + op)}, Identity: domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: op}})
	}
	tools = append(tools, domain.AvailableTool{Definition: root.ToolDefinition{Name: "kmp_ask"}, Identity: domain.ToolIdentity{Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: "kmp", ToolName: "kmp_ask"}}})
	calls := []root.ToolCall{
		{ID: "exec", Name: "local_exec", Arguments: value(`{"program":"go","args":["test","./..."]}`)},
		{ID: "read", Name: "local_read", Arguments: value(`{"path":"a.go"}`)},
		{ID: "edit", Name: "local_edit", Arguments: value(`{"path":"a.go","old_text":"one","new_text":"two"}`)},
		{ID: "ask", Name: "kmp_ask", Arguments: value(`{"question":"why"}`)},
		{ID: "deny", Name: "local_exec", Arguments: value(`{"program":"rm","args":["x"]}`)},
	}
	change := domain.FileChange{Path: "a.go", Before: "package a\n\nconst X = \"one\"\n", After: "package a\n\nconst X = \"two\"\n"}
	return domain.SessionState{ID: "0123456789abcdef0123456789abcdef", ToolSnapshot: tools,
		Messages: []root.Message{
			{Role: root.RoleUser, Content: "check"},
			{Role: root.RoleAssistant, ToolCalls: calls},
			{Role: root.RoleTool, ToolCallID: "exec", Content: `{"status":"completed","duration_ms":1400,"output":{"exit_code":1,"stdout":"--- FAIL: TestX\nFAIL","stderr":"exit status 1\n","truncated":false}}`},
			{Role: root.RoleTool, ToolCallID: "read", Content: `{"status":"completed","output":{"content":"package a\n\tconst X = 1\n","next_offset_bytes":24}}`},
			{Role: root.RoleTool, ToolCallID: "edit", Content: `{"status":"completed","output":{"replacements":1}}`},
			{Role: root.RoleTool, ToolCallID: "ask", Content: `{"answer":"because\nit was decided","refs":["a","b"],"meta":{"n":2}}`},
			{Role: root.RoleTool, ToolCallID: "deny", Content: "tool call denied by user: not rm"},
		},
		Activity: []domain.PendingTool{
			{Call: calls[0], Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{}},
			{Call: calls[1], Decision: domain.DecisionAutoApprove, Outcome: &domain.ToolOutcome{}},
			{Call: calls[2], Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{Change: &change}},
			{Call: calls[3], Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{}},
			{Call: calls[4], Decision: domain.DecisionDeny, Outcome: &domain.ToolOutcome{Content: "tool call denied by user: not rm", IsError: true}},
		},
	}
}

// A click on a tool row opens what the call returned under it, as text;
// another folds it.
func TestToolRowsOpenTheirResultAsText(t *testing.T) {
	state := resultSession(t)
	tr := NewTranscript()
	tr.SetWidth(100)
	theme := Theme{Locale: English, Monochrome: true}
	tr.SetSession(state, "", theme)
	for _, id := range []string{"exec", "read", "edit", "ask", "deny"} {
		tr.ToggleNote(toolNoteKey(root.ToolCallID(id)))
	}
	tr.SetSession(state, "", theme)
	text := tr.Text()
	for _, want := range []string{
		"exit 1 · 1.4\u00a0s", "stdout:", "    --- FAIL: TestX", "    FAIL", "stderr:", "    exit status 1",
		"package a\n", "    const X = 1",
		"── a.go ──", "  - const X = \"one\"", "  + const X = \"two\"",
		"answer:\n", "    because\n", "    it was decided", "refs:\n", "    - a", "meta:\n", "    n: 2",
		"tool call denied by user: not rm",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expanded results lack %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `\n`) || strings.Contains(text, `"exit_code"`) {
		t.Fatalf("escaped JSON in the transcript:\n%s", text)
	}
	tr.ToggleNote(toolNoteKey("exec"))
	tr.SetSession(state, "", theme)
	if strings.Contains(tr.Text(), "--- FAIL") {
		t.Fatal("a second toggle did not fold the result")
	}
}

// A long result opens its first lines and says where the rest is.
func TestALongResultOpensItsFirstLines(t *testing.T) {
	rows := toolResultRows(domain.SessionState{}, root.ToolCall{ID: "x", Name: "plugin"}, nil, &root.Message{Content: root.Text(strings.Repeat("line\n", 260))}, Theme{Locale: English})
	if len(rows) != maxExpandedResultLines+1 || !strings.Contains(rows[len(rows)-1].Text, "61 more lines · Ctrl+P → I") {
		t.Fatalf("%d rows, last %q", len(rows), rows[len(rows)-1].Text)
	}
}

// The console opens a tool row on a plain click, keeping the row as a
// tool row that wraps like before.
func TestClickingAToolRowOpensIt(t *testing.T) {
	now := time.Now()
	selectionNow = func() time.Time { now = now.Add(time.Second); return now }
	defer func() { selectionNow = time.Now }()
	m := sized()
	defer m.zones.Close()
	m.Header.State = resultSession(t)
	m.refreshTranscript()
	lines := strings.Split(ansi.Strip(m.Transcript.Viewport.GetContent()), "\n")
	for i, line := range lines {
		if strings.Contains(line, "go test ./...") {
			y := 1 + i - m.Transcript.Viewport.YOffset()
			m = update(m, tea.MouseClickMsg{X: 6, Y: y, Button: tea.MouseLeft})
			m = update(m, tea.MouseReleaseMsg{X: 6, Y: y, Button: tea.MouseLeft})
			if !strings.Contains(m.Transcript.Text(), "--- FAIL: TestX") || m.Transcript.Selection.Set {
				t.Fatalf("the click did not open the result:\n%s", m.Transcript.Text())
			}
			return
		}
	}
	t.Fatalf("no row shows the command:\n%s", strings.Join(lines, "\n"))
}

// Info shows each result unescaped too.
func TestReadableResult(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"a":"x\ny","b":[],"c":{},"d":null,"e":[{"f":1}]}`, "a:\n  x\n  y\nb: []\nc: {}\nd: null\ne:\n  -\n    f: 1"},
		{`plain text`, "plain text"},
		{`[1,"two"]`, "- 1\n- two"},
		{`{"a":1} trailing`, `{"a":1} trailing`},
	} {
		if got := readableResult(tc.in); got != tc.want {
			t.Fatalf("%s:\n%q\nwant\n%q", tc.in, got, tc.want)
		}
	}
}
