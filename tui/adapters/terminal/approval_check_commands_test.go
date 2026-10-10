package terminal

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const decomposeHandback = `{"summary":"two tasks","tasks":[` +
	`{"id":"short","goal":"Add Short()","scope":["buildinfo/version.go"],"context":[{"path":"buildinfo/version.go","line":3,"quote":"var Version = \"dev\""}],"unit_check":{"program":"go","args":["test","./buildinfo/..."]},"test_first":true},` +
	`{"id":"docs","goal":"Document Short()","scope":["docs/version.md"],"unit_check":{"program":"go","args":["test","./buildinfo/..."]}},` +
	`{"id":"cli","goal":"Print it","scope":["cmd/main.go"],"unit_check":{"program":"go","args":["test","-run","Test Short","./cmd/..."]}}],` +
	`"e2e_check":{"program":"go","args":["test","./..."]},"interfaces":"func Short() string"}`

// On 10 October 2026 the decompose hand-back card was the whole
// axlr_step_done JSON, ~60 lines, with the commands it approved at the end.
func TestTheCheckCommandCardListsTheDistinctCommandsFirst(t *testing.T) {
	m := forgeApprovalModel(t, application.HostStepDoneName, domain.HostOperationStepDone, decomposeHandback)
	defer m.zones.Close()
	text := m.Approval.Details.Text()
	lines := strings.Split(text, "\n")
	want := []string{
		m.Theme.T("approval.checkCommandsLead"),
		"$ go test ./...   · end-to-end check",
		"$ go test ./buildinfo/...   · tasks short, docs",
		`$ go test -run "Test Short" ./cmd/...   · task cli`,
		"",
		m.Theme.T("approval.checkArguments"),
	}
	if len(lines) < len(want) {
		t.Fatalf("card:\n%s", text)
	}
	for i, line := range want {
		if lines[i] != line {
			t.Fatalf("line %d is %q, want %q:\n%s", i, lines[i], line, text)
		}
	}
	if !strings.Contains(text, `"interfaces": "func Short() string"`) {
		t.Fatalf("the hand-back's JSON is no longer reachable below the commands:\n%s", text)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "3 ceremony check commands") || !strings.Contains(view, "$ go test ./...") {
		t.Fatalf("the card's first rows are not the commands:\n%s", view)
	}
	brief := forgeApprovalModel(t, application.HostStepDoneName, domain.HostOperationStepDone, `{"summary":"x","check_command":{"program":"go","args":["test","./a/..."]}}`)
	defer brief.zones.Close()
	if text := brief.Approval.Details.Text(); !strings.HasPrefix(text, brief.Theme.T("approval.checkCommandsLead")+"\n$ go test ./a/...\n") || brief.Approval.Target != brief.Theme.T("approval.checkCommand") {
		t.Fatalf("a single check command card:\n%s\ntitle %q", text, brief.Approval.Target)
	}
}
