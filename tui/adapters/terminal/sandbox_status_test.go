package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026, with exec_sandbox auto on a machine where bwrap
// could not create a sandbox, only stderr said commands ran unconfined,
// and the alternate screen hid it at once.
func TestTheHeaderMarksASandboxThatDoesNotConfine(t *testing.T) {
	s := domain.SessionState{ID: "0123456789abcdef0123456789abcdef", Model: "anthropic/claude-haiku-5.5", Workspace: "/srv/projects/underpass/AXLR/.claude/worktrees/agent-a4ccc4e234620d005"}
	for _, id := range []string{"kmp", "made"} {
		s.ToolSnapshot = append(s.ToolSnapshot, domain.AvailableTool{Identity: domain.ToolIdentity{Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: root.PluginID(id), ToolName: "t"}}})
	}
	theme := Theme{Monochrome: true}
	view := ansi.Strip(Header{State: s, SandboxOff: true}.View(80, theme))
	for _, want := range []string{"AXLR", "anthropic/claude-haiku-5.5", "sandbox off", "kmp", "made"} {
		if !strings.Contains(view, want) {
			t.Fatalf("80 columns: header %q lacks %q", view, want)
		}
	}
	view = ansi.Strip(Header{State: s, SandboxOff: true}.View(60, theme))
	if ansi.StringWidth(view) != 60 || !strings.Contains(view, "kmp") || !strings.Contains(view, "made") || !strings.Contains(view, "anthropic/claude-haiku-5.5") {
		t.Fatalf("60 columns: header lost the model or the plugins: %q", view)
	}
	if view := ansi.Strip(Header{State: s}.View(200, theme)); strings.Contains(view, "sandbox") {
		t.Fatalf("a confined or unconfigured sandbox is marked: %q", view)
	}
}

func TestInfoAndHelpSayWhyTheSandboxIsOff(t *testing.T) {
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	reason := "bwrap cannot create a sandbox here: exit status 1"
	m := update(New(Dependencies{Session: &session, Monochrome: true, Sandbox: SandboxStatus{Mode: "auto", Unavailable: reason}}), tea.WindowSizeMsg{Width: 120, Height: 40})
	defer m.zones.Close()
	if !strings.Contains(ansi.Strip(m.View().Content), "sandbox off") {
		t.Fatal("the console does not mark the inactive sandbox")
	}
	for _, intent := range []ControlIntent{"info", "help"} {
		view := ansi.Strip(update(m, intent).View().Content)
		if !strings.Contains(view, "exec_sandbox is auto") || !strings.Contains(view, "bwrap cannot create a sandbox here") || !strings.Contains(view, "run unsandboxed") {
			t.Fatalf("%s does not give the reason:\n%s", intent, view)
		}
	}
	required := SandboxStatus{Mode: "required", Unavailable: "bwrap is not installed"}
	if note := required.sandboxInfo(Theme{Locale: Spanish}); !strings.Contains(note, "se rechazan") {
		t.Fatalf("spanish required note: %q", note)
	}
	for _, status := range []SandboxStatus{{}, {Mode: "off", Unavailable: "x"}, {Mode: "auto"}} {
		if status.Off() {
			t.Fatalf("%+v reads as an inactive sandbox", status)
		}
	}
}
