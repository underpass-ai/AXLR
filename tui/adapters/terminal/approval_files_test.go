package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func pendingCall(t *testing.T, name, arguments string) domain.PendingTool {
	t.Helper()
	args, err := root.NewJSONValue([]byte(arguments))
	if err != nil {
		t.Fatal(err)
	}
	return domain.PendingTool{Call: root.ToolCall{ID: "one", Name: root.ToolName(name), Arguments: args}}
}

// On 10 October 2026 a local_edit card showed {"old_text": "\tif ...\n..."}
// with literal escapes: the person could not judge the edit they approved.
func TestAnEditCardShowsTheReplacementAsADiff(t *testing.T) {
	p := pendingCall(t, "local_edit", `{"path":"tui/config.go","old_text":"\tif !ok {\n\t\treturn err\n\t}","new_text":"\tif !ok {\n\t\treturn fmt.Errorf(\"%s: %w\", path, err)\n\t}","expected_sha256":"abc"}`)
	d := NewApprovalDialog(p, "target", Theme{Monochrome: true})
	d.Details.SetWidth(80)
	text := d.Details.Text()
	for _, want := range []string{"tui/config.go", "      if !ok {", "-         return err", "+         return fmt.Errorf(\"%s: %w\", path, err)", "      }"} {
		if !strings.Contains(text, want) {
			t.Fatalf("edit card lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `\n`) || strings.Contains(text, `\t`) || strings.Contains(text, "old_text") || strings.Contains(text, "\t") {
		t.Fatalf("edit card still shows escaped JSON:\n%s", text)
	}
	if strings.Contains(text, "-     if !ok {") || strings.Contains(text, "+     }") {
		t.Fatalf("edit card marks unchanged lines as changed:\n%s", text)
	}
	if d.Details.VisualLineCount() < 5 {
		t.Fatalf("edit card has %d lines", d.Details.VisualLineCount())
	}
}

// On 10 October 2026 an edit of "// comment\nconst X" to "// other\nconst X"
// drew the unchanged const line as "-" and as "+".
func TestAnEditCardDrawsUnchangedLinesOnceAsContext(t *testing.T) {
	p := pendingCall(t, "local_edit", `{"path":"a.go","old_text":"// comment\nconst X = 1","new_text":"// other\nconst X = 1"}`)
	d := NewApprovalDialog(p, "target", Theme{Monochrome: true})
	d.Details.SetWidth(80)
	text := d.Details.Text()
	if strings.Count(text, "const X = 1") != 1 || strings.Contains(text, "- const") || strings.Contains(text, "+ const") {
		t.Fatalf("the unchanged line is not context:\n%s", text)
	}
	for _, want := range []string{"- // comment", "+ // other"} {
		if !strings.Contains(text, want) {
			t.Fatalf("edit card lacks %q:\n%s", want, text)
		}
	}
	deletion := NewApprovalDialog(pendingCall(t, "local_edit", `{"path":"a.go","old_text":"gone\nalso gone","new_text":""}`), "target", Theme{Monochrome: true})
	if text := deletion.Details.Text(); !strings.Contains(text, "- gone") || !strings.Contains(text, "- also gone") || strings.Contains(text, "+ ") {
		t.Fatalf("a deletion is not all removed lines:\n%s", text)
	}
}

func TestAnEditCardColoursRemovedAndAddedLines(t *testing.T) {
	p := pendingCall(t, "local_edit", `{"path":"a.go","old_text":"old line","new_text":"new line"}`)
	theme := Theme{}
	d := NewApprovalDialog(p, "target", theme)
	d.Details.SetWidth(40)
	d.Details.Viewport.SetHeight(10)
	view := d.Details.View()
	removed, _, _ := strings.Cut(theme.rowText(transcriptRowDiffRemoved).Render("x"), "x")
	added, _, _ := strings.Cut(theme.rowText(transcriptRowDiffAdded).Render("x"), "x")
	if removed == added {
		t.Fatalf("removed and added lines share a colour %q", removed)
	}
	var sawRemoved, sawAdded bool
	for _, line := range strings.Split(view, "\n") {
		plain := strings.TrimSpace(ansi.Strip(line))
		switch {
		case strings.HasPrefix(plain, "- old line"):
			sawRemoved = strings.Contains(line, removed)
		case strings.HasPrefix(plain, "+ new line"):
			sawAdded = strings.Contains(line, added)
		}
	}
	if !sawRemoved || !sawAdded {
		t.Fatalf("diff lines are not coloured by kind (removed %v, added %v): %q", sawRemoved, sawAdded, view)
	}
}

func TestAWriteCardShowsThePathAndTheContent(t *testing.T) {
	p := pendingCall(t, "local_write", `{"path":"notes/plan.md","mode":"create","content":"# Plan\n\n\tstep one\n"}`)
	d := NewApprovalDialog(p, "target", Theme{Monochrome: true})
	d.Details.SetWidth(80)
	text := d.Details.Text()
	for _, want := range []string{"notes/plan.md", "create", "# Plan", "    step one"} {
		if !strings.Contains(text, want) {
			t.Fatalf("write card lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `\n`) || strings.Contains(text, `"content"`) {
		t.Fatalf("write card still shows escaped JSON:\n%s", text)
	}
}

func TestOtherToolsKeepTheirArgumentsAsJSON(t *testing.T) {
	for _, call := range [][2]string{
		{"local_exec", `{"program":"go","args":["test"]}`},
		{"local_edit", `{"unexpected":"shape"}`},
	} {
		d := NewApprovalDialog(pendingCall(t, call[0], call[1]), "target", Theme{Monochrome: true})
		if text := d.Details.Text(); !strings.HasPrefix(text, "{") {
			t.Fatalf("%s card is not JSON:\n%s", call[0], text)
		}
	}
}

func TestALongEditStaysScrollableInTheCard(t *testing.T) {
	m := approvalModel(t)
	old := strings.Repeat("old line\n", 30)
	m.Approval = NewApprovalDialog(pendingCall(t, "local_edit", `{"path":"a.go","old_text":`+jsonString(old)+`,"new_text":"x"}`), "target", m.Theme)
	m.sizeApproval()
	if m.Approval.Details.VisualLineCount() <= m.approvalDetailRows() || !strings.Contains(m.approvalCard(), m.Theme.T("approval.scroll")) {
		t.Fatal("a long edit does not offer to scroll")
	}
}

func jsonString(s string) string {
	return `"` + strings.ReplaceAll(s, "\n", `\n`) + `"`
}

// The model's why opens a write or edit card, before the path and the diff.
func TestFileCardsShowTheWhyFirst(t *testing.T) {
	p := pendingCall(t, "local_write", `{"path":"notes/plan.md","mode":"create","content":"# Plan\n","why":"keep the plan beside the code"}`)
	d := NewApprovalDialog(p, "target", Theme{Monochrome: true})
	d.Details.SetWidth(80)
	text := d.Details.Text()
	if !strings.HasPrefix(text, "why: keep the plan beside the code") || !strings.Contains(text, "notes/plan.md") {
		t.Fatalf("write card:\n%s", text)
	}
	e := NewApprovalDialog(pendingCall(t, "local_edit", `{"path":"a.go","old_text":"x","new_text":"y","why":"rename the field"}`), "target", Theme{Monochrome: true})
	e.Details.SetWidth(80)
	if text := e.Details.Text(); !strings.HasPrefix(text, "why: rename the field") {
		t.Fatalf("edit card:\n%s", text)
	}
}

// On 14 October 2026 a local_write replace card listed the whole new
// calc.go: over an existing file it now shows the diff against the file on
// disk and how many lines change; a new file still shows its content.
func TestAWriteOverAFileShowsItsDiffAgainstTheDisk(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "calc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc", "calc.go"), []byte("package calc\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	card := func(path, mode string) string {
		t.Helper()
		s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(dir), "model")
		if err != nil {
			t.Fatal(err)
		}
		schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
		s.BeginTurn("fix it", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "local_write", Description: "write", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "write"}}})
		args, _ := root.NewJSONValue([]byte(`{"path":"` + path + `","mode":"` + mode + `","content":"package calc\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n","why":"fix the sum"}`))
		if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "w", Name: "local_write", Arguments: args}}}}); err != nil {
			t.Fatal(err)
		}
		m := navModel(t, &s)
		defer m.zones.Close()
		m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
		return ansi.Strip(m.Approval.Details.Text())
	}
	replace := card("calc/calc.go", "replace")
	for _, want := range []string{"why: fix the sum", "calc/calc.go · replace", "against the file on disk: +1 -1 lines", "- ", "return a - b", "+ ", "return a + b"} {
		if !strings.Contains(replace, want) {
			t.Fatalf("replace card lacks %q:\n%s", want, replace)
		}
	}
	if strings.Count(replace, "func Add") != 1 {
		t.Fatalf("an unchanged line is not context once:\n%s", replace)
	}
	created := card("calc/new.go", "create")
	if strings.Contains(created, "against the file on disk") || !strings.Contains(created, "return a + b") {
		t.Fatalf("create card:\n%s", created)
	}
	if escape := card("../outside.go", "replace"); strings.Contains(escape, "against the file on disk") {
		t.Fatalf("a path outside the workspace was read:\n%s", escape)
	}
}
