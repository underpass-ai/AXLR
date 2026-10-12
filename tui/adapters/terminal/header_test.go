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

// The header's logo is each letter of "AXLR" on a tile of its wordmark
// colour; a monochrome terminal writes it.
func TestHeaderLogoTilesEachLetterInItsWordmarkColour(t *testing.T) {
	brand := Theme{ID: domain.ThemeInk}.Brand()
	if got := ansi.Strip(brand); got != " A  X  L  R " {
		t.Fatalf("logo reads %q", got)
	}
	for _, sgr := range []string{"48;2;184;92;56m", "48;2;217;123;45m", "48;2;107;74;46m", "48;2;201;176;140m"} {
		if !strings.Contains(brand, sgr) {
			t.Fatalf("logo %q lacks tile %q", brand, sgr)
		}
	}
	if got := (Theme{Monochrome: true}).Brand(); got != ">> AXLR" {
		t.Fatalf("monochrome logo is %q", got)
	}
}

// A wide row labels the model and the workspace; a short one drops the
// labels before it cuts plugin names.
func TestHeaderLabelsTheModelAndTheWorkspace(t *testing.T) {
	s := domain.SessionState{ID: "0123456789abcdef0123456789abcdef", Model: "z-ai/glm", Workspace: "/srv/ws"}
	if view := ansi.Strip(Header{State: s}.View(100, Theme{Monochrome: true})); !strings.Contains(view, "model: z-ai/glm  workspace: /srv/ws") {
		t.Fatalf("header %q lacks the labels", view)
	}
	es := Theme{Monochrome: true, Locale: Spanish}
	if view := ansi.Strip(Header{State: s}.View(100, es)); !strings.Contains(view, "modelo: z-ai/glm  espacio de trabajo: /srv/ws") {
		t.Fatalf("header %q lacks the Spanish labels", view)
	}
	if view := ansi.Strip(Header{State: s}.View(30, Theme{Monochrome: true})); strings.Contains(view, "model:") || !strings.Contains(view, "z-ai/glm  /srv/ws") {
		t.Fatalf("short header %q kept the labels", view)
	}
}
