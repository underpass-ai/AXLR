package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026, at 80 columns, the footer read "… $0.0057 turn · $…"
// with the session cost cut and the key hints kept, and the header dropped
// "● kmp ● made" for a long workspace path.
func TestAt80ColumnsTheFooterKeepsTheStateAndTheSessionCost(t *testing.T) {
	for _, width := range []int{80, 60} {
		m := update(sized(), tea.WindowSizeMsg{Width: width, Height: 24})
		m.Header.State.Mode = domain.ModeReview
		m.Status.Autonomous = true
		m.tokens, m.cached = 54_716, 89
		m.cost = costView{ledger: domain.UsageLedger{Cost: 0.0531, CostRequests: 50}, turnCost: 0.0474, turnRequests: 48}
		footer := ansi.Strip(m.footerView())
		if ansi.StringWidth(footer) != width || strings.Contains(footer, "…") {
			t.Fatalf("width %d: footer is cut: %q", width, footer)
		}
		for _, want := range []string{m.Theme.T("status.idle"), m.Theme.Tf("footer.costSession", "$0.05")} {
			if !strings.Contains(footer, want) {
				t.Fatalf("width %d: footer lost %q: %q", width, want, footer)
			}
		}
		m.zones.Close()
	}
	m := update(sized(), tea.WindowSizeMsg{Width: 200, Height: 24})
	defer m.zones.Close()
	m.tokens, m.cached = 54_716, 89
	m.cost = costView{ledger: domain.UsageLedger{Cost: 0.0531, CostRequests: 50}, turnCost: 0.0474, turnRequests: 48}
	if footer := ansi.Strip(m.footerView()); !strings.Contains(footer, "enter send") || !strings.Contains(footer, "54.7k tok") || !strings.Contains(footer, "turn") {
		t.Fatalf("a wide footer dropped something: %q", footer)
	}
}

func TestAt80ColumnsTheHeaderKeepsTheConnectedPlugins(t *testing.T) {
	s := domain.SessionState{ID: "0123456789abcdef0123456789abcdef", Model: "anthropic/claude-haiku-5.5", Workspace: "/srv/projects/underpass/AXLR/.claude/worktrees/agent-a4ccc4e234620d005"}
	for _, id := range []string{"kmp", "made"} {
		s.ToolSnapshot = append(s.ToolSnapshot, domain.AvailableTool{Identity: domain.ToolIdentity{Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: root.PluginID(id), ToolName: "t"}}})
	}
	view := ansi.Strip(Header{State: s}.View(80, Theme{Monochrome: true}))
	if ansi.StringWidth(view) != 80 {
		t.Fatalf("header is not one full row: %q", view)
	}
	for _, want := range []string{"AXLR", "anthropic/claude-haiku-5.5", "kmp", "made"} {
		if !strings.Contains(view, want) {
			t.Fatalf("header %q lacks %q", view, want)
		}
	}
}
