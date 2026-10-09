package terminal

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
)

func typeDraft(m AppModel, text string) AppModel {
	for _, r := range text {
		m = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestSlashSuggestionsListMatchingCommands(t *testing.T) {
	names := func(list []slashCommand) string {
		var out []string
		for _, c := range list {
			out = append(out, c.name)
		}
		return strings.Join(out, ",")
	}
	for draft, want := range map[string]string{
		"/":          "/model,/theme,/mcp,/plugin,/changes,/approvals",
		"/m":         "/model,/mcp",
		"/autonomy ": "/autonomy on,/autonomy off",
		"/autonomy":  "",
		"/model":     "",
		"hola /m":    "",
		"/m\nmore":   "",
	} {
		if got := names(slashSuggestions(draft)); got != want {
			t.Fatalf("slashSuggestions(%q) = %q; want %q", draft, got, want)
		}
	}
}

func TestSlashMenuShowsAboveTheComposerAndTabCompletes(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = typeDraft(m, "/m")
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "/model") || !strings.Contains(view, "choose the model") || strings.Index(view, "/mcp") > strings.LastIndex(view, "> /m") {
		t.Fatalf("menu missing or below the composer:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.Composer.Input.Value(); got != "/mcp" {
		t.Fatalf("Tab completed to %q; want /mcp", got)
	}
}

func TestEnterOnAPartialCommandRunsTheHighlightedOne(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = typeDraft(m, "/the")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.overlay != "theme" {
		t.Fatalf("overlay = %q; want theme", m.overlay)
	}
}

// The bare command shows the current state: a complete command never runs
// the suggestion it happens to be a prefix of.
func TestEnterOnBareAutonomyLeavesAutonomyUnchanged(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	settings, err := storage.NewApprovalSettings(filepath.Join(t.TempDir(), "approvals.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	m.deps.ApprovalSettings = settings
	m = typeDraft(m, "/autonomy")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if settings.Autonomous() || m.Status.Autonomous {
		t.Fatal("Enter on /autonomy turned full autonomy on")
	}
	if m.Composer.Input.Value() != "" || m.Status.Error != "" {
		t.Fatalf("bare /autonomy not handled: draft=%q error=%q", m.Composer.Input.Value(), m.Status.Error)
	}
}

func TestExitAndQuitCloseTheConsole(t *testing.T) {
	for _, command := range []string{"/exit", "/quit"} {
		m := sized()
		m.Composer.Input.SetValue(command)
		_, cmd := m.Update(ControlIntent("send"))
		if cmd == nil {
			t.Fatalf("%s returned no command", command)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s did not quit", command)
		}
	}
}

func TestUnknownSlashWordIsNotSentToTheModel(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Composer.Input.SetValue("/foo")
	m = update(m, ControlIntent("send"))
	if m.Busy || m.Composer.Input.Value() != "/foo" || !strings.Contains(m.Status.Error, "Unknown command /foo") {
		t.Fatalf("unknown command handled wrongly: busy=%v draft=%q error=%q", m.Busy, m.Composer.Input.Value(), m.Status.Error)
	}
	if unknownSlashWord("/home/u/file is broken") || unknownSlashWord("/tmp/x") {
		t.Fatal("a path or sentence was treated as a command")
	}
}
