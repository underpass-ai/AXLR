package terminal

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
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
	// On 10 October 2026, under full autonomy, git status raised a card in
	// review mode and nothing said why.
	if view := ansi.Strip(update(m, tea.WindowSizeMsg{Width: 80, Height: 24}).View().Content); !strings.Contains(view, "review mode asks before every command") {
		t.Fatalf("the card does not say the mode asks:\n%s", view)
	}
	normal := execApprovalModel(t, domain.ModeNormal, `{"program":"python3","args":["-m","unittest"]}`)
	defer normal.zones.Close()
	if view := ansi.Strip(normal.View().Content); !strings.Contains(view, "l always") || strings.Contains(view, "asks before every command") {
		t.Fatal("normal mode lost always-allow or says a mode asks")
	}
}

func forgeApprovalModel(t *testing.T, name root.ToolName, operation, arguments string) AppModel {
	t.Helper()
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(arguments))
	schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
	s.BeginTurn("forge", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: name, Description: "forged", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindHost, LocalOperation: operation}}})
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "one", Name: name, Arguments: args}}}}); err != nil {
		t.Fatal(err)
	}
	return navModel(t, &s)
}

// Seen on 9 Oct 2026: the forge card read "read-only host forge_tool", showed
// the code as one escaped JSON string and offered an always-allow that a
// host tool cannot save.
func TestTheForgeCardShowsWhatItWritesAsCode(t *testing.T) {
	m := forgeApprovalModel(t, "axlr_forge_tool", domain.HostOperationForgeTool, `{"name":"count","description":"Count lines.","input_schema":{"type":"object"},"program":"python3","args":[".axlr/tools/count/main.py"],"files":[{"path":"main.py","content":"import sys\nprint(len(sys.stdin.read().splitlines()))\n"}]}`)
	defer m.zones.Close()
	view := ansi.Strip(m.View().Content)
	card := m.Approval.Target + "\n" + m.Approval.Details.Text()
	for _, want := range []string{"forge tool count: writes its files under .axlr/tools/count", "$ python3 .axlr/tools/count/main.py", "── main.py ──", "import sys\nprint(len("} {
		if !strings.Contains(card, want) {
			t.Fatalf("card lacks %q:\n%s", want, card)
		}
	}
	if strings.Contains(card, "read-only") || strings.Contains(card, `\n`) || strings.Contains(view, "l always") {
		t.Fatalf("card:\n%s\n%s", card, view)
	}
	m = update(m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.Busy || len(m.deps.Session.Pending()) != 1 {
		t.Fatal("l acted on a host tool")
	}
	run := forgeApprovalModel(t, "axlr_run_tool", domain.HostOperationRunTool, `{"name":"count","arguments":{}}`)
	defer run.zones.Close()
	if !strings.Contains(run.Approval.Target, "run forged tool count") {
		t.Fatalf("run card: %s", run.Approval.Target)
	}
	remove := forgeApprovalModel(t, "axlr_remove_tool", domain.HostOperationRemoveTool, `{"name":"count"}`)
	defer remove.zones.Close()
	if !strings.Contains(remove.Approval.Target, "remove forged tool count: deletes .axlr/tools/count") {
		t.Fatalf("remove card: %s", remove.Approval.Target)
	}
}

// An exec card is the command as it runs, with cwd, timeout, output limit
// and stdin only when the call sets them, and no JSON repeating it; the
// title names the tool without the workspace path. On 12 October 2026 the
// card showed "$ go vet ./buildinfo/..." and the same arguments as JSON
// under a title that repeated the absolute workspace path.
func TestTheExecCardShowsTheCommandOnce(t *testing.T) {
	m := execApprovalModel(t, domain.ModeNormal, `{"program":"go","args":["vet","./buildinfo/..."]}`)
	defer m.zones.Close()
	if got := m.Approval.Details.Text(); got != "$ go vet ./buildinfo/..." {
		t.Fatalf("card %q", got)
	}
	if m.Approval.Target != "local exec" {
		t.Fatalf("title %q", m.Approval.Target)
	}
	full := execApprovalModel(t, domain.ModeNormal, `{"program":"python3","args":["-"],"cwd":"tools","timeout_ms":120000,"max_output_bytes":2048,"stdin":"print(1)\nprint(2)\n"}`)
	defer full.zones.Close()
	want := "! runs 2 lines of code from stdin; read \"stdin\" below before approving\n\n$ python3 -\nin tools\ntimeout 120.0 s\noutput up to 2.0 KB\nstdin:\n  print(1)\n  print(2)"
	if got := full.Approval.Details.Text(); got != want {
		t.Fatalf("card\n%q\nwant\n%q", got, want)
	}
	odd := execApprovalModel(t, domain.ModeNormal, `{"program":"ls","args":[],"env":{"A":"1"}}`)
	defer odd.zones.Close()
	if got := odd.Approval.Details.Text(); !strings.HasPrefix(got, "$ ls\n\n{") || !strings.Contains(got, `"env"`) {
		t.Fatalf("unknown field hidden: %q", got)
	}
}

// The card may take 60 % of the screen before its details scroll.
func TestTheApprovalCardGrowsToSixtyPercent(t *testing.T) {
	m := execApprovalModel(t, domain.ModeNormal, `{"program":"python3","args":["-"],"stdin":"`+strings.Repeat(`print(1)\n`, 40)+`"}`)
	defer m.zones.Close()
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 50})
	if rows := m.approvalDetailRows(); rows != 27 {
		t.Fatalf("detail rows %d at 50 rows", rows)
	}
	if card := lipglossHeight(m.approvalCard()); card != 30 {
		t.Fatalf("card height %d", card)
	}
}

func lipglossHeight(s string) int { return strings.Count(s, "\n") + 1 }

// The model's why leads the exec card, right under the command line.
func TestTheExecCardShowsTheWhy(t *testing.T) {
	m := execApprovalModel(t, domain.ModeNormal, `{"program":"go","args":["test","./parser/..."],"why":"run the parser tests after the fix"}`)
	defer m.zones.Close()
	if got := m.Approval.Details.Text(); got != "$ go test ./parser/...\nwhy: run the parser tests after the fix" {
		t.Fatalf("card %q", got)
	}
}

// A destructive card hides L and F; on 14 October 2026 review found the
// keys still answered it: L saved a rule for the whole tool and F turned
// autonomy on, both approving the command.
func TestADestructiveCardIgnoresAlwaysAllowAndAutonomy(t *testing.T) {
	m := execApprovalModel(t, domain.ModeNormal, `{"program":"git","args":["push","--force","origin","main"],"why":"publish"}`)
	defer m.zones.Close()
	settings, err := storage.NewApprovalSettings(filepath.Join(t.TempDir(), "approvals.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	m.deps.ApprovalSettings = settings
	m.deps.Resolve.Approval = settings
	m.deps.Resolve.Tools = navTool{}
	m.deps.Resolve.Continue.Models = navStream{}
	if m.destructiveRule(m.Approval.Pending) == "" {
		t.Fatal("git push --force is not destructive")
	}
	for _, key := range []rune{'l', 'f'} {
		next, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		got := drain(t, next.(AppModel), cmd)
		if p := got.deps.Session.Export().Activity[0]; p.Decision != "" || settings.Autonomous() || len(settings.Allowed()) != 0 {
			t.Fatalf("%c answered a destructive card: decision %q, autonomous %v, allowed %v", key, p.Decision, settings.Autonomous(), settings.Allowed())
		}
	}
}

// On 14 October 2026 glm-5.3-flash deleted calc/calc.go "to test path
// resolution" and its card looked like any command: a command that
// deletes, moves or truncates files says so, and that /undo cannot bring
// them back.
func TestTheCardWarnsWhenACommandRemovesFiles(t *testing.T) {
	for _, args := range []string{`{"program":"rm","args":["calc/calc.go"]}`, `{"program":"sudo","args":["-u","me","mv","a.go","b.go"]}`, `{"program":"truncate","args":["-s","0","log.txt"]}`} {
		m := execApprovalModel(t, domain.ModeNormal, args)
		if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "deletes or moves files; /undo cannot restore") {
			t.Fatalf("%s not flagged:\n%s", args, plain)
		}
		m.zones.Close()
	}
	for _, args := range []string{`{"program":"rm","args":["--help"]}`, `{"program":"git","args":["rm","x"]}`, `{"program":"ls","args":["calc"]}`} {
		m := execApprovalModel(t, domain.ModeNormal, args)
		if strings.Contains(ansi.Strip(m.View().Content), "deletes or moves files") {
			t.Fatalf("%s was flagged", args)
		}
		m.zones.Close()
	}
}
