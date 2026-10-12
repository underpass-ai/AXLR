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
	for _, want := range []string{"AXLR", "z-ai/glm", "~" + string(filepath.Separator) + "work", "kmp", "made", "microsoft-docs-microsoft-learn", "+1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("header %q lacks %q", view, want)
		}
	}
	if strings.Count(view, "made") != 1 {
		t.Fatalf("plugins are not deduplicated: %q", view)
	}
}

// A short row keeps the workspace's last segments and its branch before it
// cuts plugin names; the path is written from the home directory.
func TestHeaderShortensThePathBeforeThePlugins(t *testing.T) {
	home, _ := os.UserHomeDir()
	sep := string(filepath.Separator)
	s := domain.SessionState{ID: "0123456789abcdef0123456789abcdef", Model: "anthropic/claude-haiku-5.5", Workspace: domain.Workspace(filepath.Join(home, "claude-1000", "-home-gx10a-Documents", "scratchpad", "ws"))}
	for _, id := range []string{"kmp", "microsoft-docs-microsoft-learn"} {
		s.ToolSnapshot = append(s.ToolSnapshot, domain.AvailableTool{Identity: domain.ToolIdentity{Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: root.PluginID(id), ToolName: "t"}}})
	}
	git := &GitState{Branch: "main", Dirty: true}
	theme := Theme{Monochrome: true}
	for _, tc := range []struct {
		width int
		want  []string
	}{
		{200, []string{"~" + sep + "claude-1000" + sep + "-home-gx10a-Documents" + sep + "scratchpad" + sep + "ws  git: main*", "[OK] microsoft-docs-microsoft-learn"}},
		{120, []string{"…" + sep + "scratchpad" + sep + "ws  git: main*", "[OK] microsoft-docs-microsoft-learn"}},
		{90, []string{"…" + sep + "ws  git: main*", "[OK] microsoft-doc…"}},
	} {
		view := ansi.Strip(Header{State: s, Git: git}.View(tc.width, theme))
		for _, want := range tc.want {
			if !strings.Contains(view, want) || ansi.StringWidth(view) != tc.width {
				t.Fatalf("width %d: %q lacks %q", tc.width, view, want)
			}
		}
	}
	if got := pathTail("/tmp/a", 2); got != "/tmp/a" {
		t.Fatalf("short path %q", got)
	}
}
