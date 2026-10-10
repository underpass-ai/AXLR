package terminal

import (
	"strings"
	"testing"

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
	for _, want := range []string{"tui/config.go", "-     if !ok {", "-         return err", "+         return fmt.Errorf(\"%s: %w\", path, err)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("edit card lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `\n`) || strings.Contains(text, `\t`) || strings.Contains(text, "old_text") || strings.Contains(text, "\t") {
		t.Fatalf("edit card still shows escaped JSON:\n%s", text)
	}
	if d.Details.VisualLineCount() < 7 {
		t.Fatalf("edit card has %d lines", d.Details.VisualLineCount())
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
