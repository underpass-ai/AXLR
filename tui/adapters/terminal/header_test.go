package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestHeaderIsOneRowWithSessionPlugins(t *testing.T) {
	home, _ := os.UserHomeDir()
	s := domain.SessionState{ID: "0123456789abcdef0123456789abcdef", Model: "z-ai/glm", Workspace: domain.Workspace(filepath.Join(home, "work"))}
	for _, id := range []string{"made", "kmp", "made", "microsoft-docs-microsoft-learn", "zeta"} {
		s.ToolSnapshot = append(s.ToolSnapshot, domain.AvailableTool{Identity: domain.ToolIdentity{Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: root.PluginID(id), ToolName: "t"}}})
	}
	view := ansi.Strip(Header{State: s}.View(100, Theme{Monochrome: true}))
	if strings.Contains(view, "\n") || ansi.StringWidth(view) != 100 {
		t.Fatalf("header is not one full-width row: %q", view)
	}
	for _, want := range []string{"AXLR", "z-ai/glm", "~" + string(filepath.Separator) + "work", "kmp", "made", "microsoft-doc…", "+1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("header %q lacks %q", view, want)
		}
	}
	if strings.Count(view, "made") != 1 {
		t.Fatalf("plugins are not deduplicated: %q", view)
	}
}
