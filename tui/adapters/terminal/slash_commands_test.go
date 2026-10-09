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
		"/":          "/model,/theme,/mcp,/plugin,/changes,/approvals,/autonomy on,/autonomy off,/update,/normal,/review,/writer,/research,/debug,/delivery,/incident,/repair,/improve,/plan,/jobs,/stop-ceremony,/copy,/usage,/exit",
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

// On 10 October 2026 typing "/" listed six of 24 commands and nothing said
// more existed; ↓ stopped at the sixth.
func TestTheSlashMenuScrollsThroughEveryCommand(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = typeDraft(m, "/")
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, m.Theme.Tf("slash.more", len(slashCommands)-slashSuggestionLimit)) || strings.Contains(view, "/exit") {
		t.Fatalf("the menu does not say more commands exist:\n%s", view)
	}
	for range len(slashCommands) - 1 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "> /exit") || strings.Contains(view, "  /model ") {
		t.Fatalf("the menu did not scroll to the last command:\n%s", view)
	}
	for range 3 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	}
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "> /stop-ceremony") || !strings.Contains(view, "/exit") {
		t.Fatalf("moving up inside the window scrolled it:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.Composer.Input.Value(); got != "/stop-ceremony" {
		t.Fatalf("Tab completed to %q; want /stop-ceremony", got)
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	for _, locale := range []Locale{English, Spanish} {
		theme := Theme{Locale: locale, Monochrome: true}
		m := sized()
		view := ansi.Strip(HelpOverlay{}.View(theme, m.zones, m.prefix, 200, 60))
		m.zones.Close()
		for _, c := range slashCommands {
			name, _, _ := strings.Cut(c.name, " ")
			if !strings.Contains(view, name) {
				t.Fatalf("%s help omits %s:\n%s", locale, name, view)
			}
		}
	}
}
