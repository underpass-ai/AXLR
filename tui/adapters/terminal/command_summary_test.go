package terminal

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func execRowState(arguments string) domain.SessionState {
	args, _ := root.NewJSONObject([]byte(arguments))
	call := root.ToolCall{ID: "c1", Name: "local_exec", Arguments: args}
	return domain.SessionState{
		Messages: []root.Message{{Role: root.RoleUser, Content: "vet it"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call}}, {Role: root.RoleTool, ToolCallID: "c1", Content: `{"status":"completed","duration_ms":59}`}},
		Activity: []domain.PendingTool{{Call: call, Decision: domain.DecisionAutoApprove}},
	}
}

// On 10 October 2026 `go vet ./buildinfo/...` read "go timeout_ms=120000"
// in the transcript: the arguments array was dropped and the timeout kept.
func TestALocalExecRowShowsTheCommandAndItsArguments(t *testing.T) {
	cases := []struct {
		arguments string
		want      string
		unwanted  []string
	}{
		{`{"program":"go","args":["vet","./buildinfo/..."],"timeout_ms":120000}`, "go vet ./buildinfo/... (timeout 120.0 s)", []string{"timeout_ms"}},
		{`{"program":"go","args":["test","./..."],"timeout_ms":30000}`, "go test ./...", []string{"timeout"}},
		{`{"program":"git","args":["commit","-m","fix the row"],"cwd":"."}`, `git commit -m "fix the row"`, []string{"cwd"}},
		{`{"program":"ls","cwd":"docs"}`, "ls (cwd docs)", nil},
	}
	for _, tc := range cases {
		row := renderedToolRow(t, execRowState(tc.arguments))
		if !strings.Contains(row, tc.want) {
			t.Fatalf("row %q lacks %q", row, tc.want)
		}
		for _, unwanted := range tc.unwanted {
			if strings.Contains(row, unwanted) {
				t.Fatalf("row %q shows %q", row, unwanted)
			}
		}
	}
	long := renderedToolRow(t, execRowState(`{"program":"go","args":["test","-run","TestALocalExecRowShowsTheCommandAndItsArguments","./tui/adapters/terminal/"]}`))
	if !strings.Contains(long, "go test -run TestALocal") || !strings.Contains(long, "…") {
		t.Fatalf("a long command is not cut to the row's argument width: %q", long)
	}
}

func TestTheExecCardLeadsWithTheCommandLine(t *testing.T) {
	m := execApprovalModel(t, domain.ModeNormal, `{"program":"go","args":["vet","./buildinfo/..."],"timeout_ms":120000}`)
	defer m.zones.Close()
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "$ go vet ./buildinfo/... (timeout 120.0") {
		t.Fatalf("exec card does not lead with the command:\n%s", view)
	}
	if !strings.Contains(m.Approval.Details.Text(), `"args"`) {
		t.Fatal("the card lost the call's JSON")
	}
	warned := execApprovalModel(t, domain.ModeNormal, `{"program":"python3","args":["-c","print(1)"]}`)
	defer warned.zones.Close()
	if text := warned.Approval.Details.Text(); !strings.Contains(text, "runs inline code") || !strings.Contains(text, `$ python3 -c "print(1)"`) {
		t.Fatalf("the warning and the command do not both lead the card:\n%s", text)
	}
}
