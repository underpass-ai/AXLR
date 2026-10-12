package terminal

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
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
		"/":          "/new,/model,/theme,/mcp,/plugin,/changes,/approvals,/autonomy on,/autonomy off,/update,/normal,/review,/writer,/research,/debug,/delivery,/incident,/repair,/improve,/plan,/jobs,/stop-ceremony,/copy,/export,/usage,/exit",
		"/m":         "/model,/mcp,/improve,/theme,/autonomy on,/autonomy off,/normal,/stop-ceremony",
		"/trab":      "/jobs",
		"/ceremony":  "/stop-ceremony",
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
	// On 12 October 2026 bare /autonomy cleared the composer and said nothing.
	if m.Status.Notice != m.Theme.T("notice.autonomyOff") || !strings.Contains(ansi.Strip(m.View().Content), "Full autonomy is off") {
		t.Fatalf("bare /autonomy did not say the state: %q", m.Status.Notice)
	}
	m = typeDraft(m, "/autonomy on")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = typeDraft(m, "/autonomy")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !settings.Autonomous() || m.Status.Notice != m.Theme.T("notice.autonomyOn") {
		t.Fatalf("bare /autonomy after on said %q", m.Status.Notice)
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
	limit := m.slashLimit()
	if limit != slashSuggestionMax {
		t.Fatalf("a 30-row console shows %d suggestions; want %d", limit, slashSuggestionMax)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, m.Theme.Tf("slash.more", len(slashCommands)-limit)) || strings.Contains(view, "/exit") {
		t.Fatalf("the menu does not say more commands exist:\n%s", view)
	}
	for range len(slashCommands) - 1 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "> /exit") || strings.Contains(view, "  /model ") {
		t.Fatalf("the menu did not scroll to the last command:\n%s", view)
	}
	// On 10 October 2026 the count read "↑↓ 18 more" wherever the list was.
	if above := len(slashCommands) - limit; !strings.Contains(view, m.Theme.Tf("slash.above", above)) || strings.Contains(view, "below") {
		t.Fatalf("at the end the menu does not count what is above:\n%s", view)
	}
	for range 3 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	}
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "> "+slashCommands[len(slashCommands)-4].name+" ") || !strings.Contains(view, "/exit") {
		t.Fatalf("moving up inside the window scrolled it:\n%s", view)
	}
	for range limit {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	}
	view = ansi.Strip(m.View().Content)
	top := slashWindow(m.slashTop, m.slashSelected, len(slashCommands), limit)
	if want := m.Theme.Tf("slash.aboveBelow", top, len(slashCommands)-top-limit); !strings.Contains(view, want) {
		t.Fatalf("in the middle the menu does not count both sides (%q):\n%s", want, view)
	}
	want := slashCommands[m.slashSelected].name
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.Composer.Input.Value(); got != want {
		t.Fatalf("Tab completed to %q; want %s", got, want)
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

// On 12 October 2026 "/Usage" started a paid turn ("/usage es un comando
// de la consola…"); "/EXIT", "/autonomy ON", "/autonomy  on", "/review the
// auth module" and "/model gpt-x" would have too.
func TestSlashCommandsIgnoreCaseAndSpacingAndNeverSendTheirText(t *testing.T) {
	sent := func(t *testing.T, m AppModel) bool {
		t.Helper()
		messages := m.Header.State.Messages
		return m.Busy && len(messages) > 0 && messages[len(messages)-1].Role == "user"
	}
	t.Run("case and spacing", func(t *testing.T) {
		for draft, want := range map[string]string{
			"/Usage": "/usage", "/EXIT": "/exit", "/autonomy ON": "/autonomy on", "/autonomy  on": "/autonomy on",
			"/Autonomy Off": "/autonomy off", "/REVISAR": "/review", "/Copy": "/copy",
		} {
			if got, exact := isSlashCommand(draft); !exact || got != want {
				t.Fatalf("%q read as %q (exact %v); want %q", draft, got, exact, want)
			}
		}
		if got := slashSuggestions("/Us"); len(got) != 1 || got[0].name != "/usage" {
			t.Fatalf("/Us suggests %v", got)
		}
	})
	t.Run("/Usage opens the usage panel", func(t *testing.T) {
		session := navSession(t)
		m, _ := selectionModel(t, &session)
		m.Composer.Input.SetValue("/Usage")
		m = update(m, ControlIntent("send"))
		if sent(t, m) || m.overlay != "usage" {
			t.Fatalf("/Usage: busy=%v overlay=%q", m.Busy, m.overlay)
		}
	})
	t.Run("/EXIT quits", func(t *testing.T) {
		m := sized()
		m.Composer.Input.SetValue("/EXIT")
		_, cmd := m.Update(ControlIntent("send"))
		if cmd == nil {
			t.Fatal("/EXIT did nothing")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("/EXIT did not quit")
		}
	})
	t.Run("/autonomy ON and /autonomy  on", func(t *testing.T) {
		for _, draft := range []string{"/autonomy ON", "/autonomy  on"} {
			m := sized()
			settings, err := storage.NewApprovalSettings(filepath.Join(t.TempDir(), "approvals.json"), nil)
			if err != nil {
				t.Fatal(err)
			}
			m.deps.ApprovalSettings = settings
			m.Composer.Input.SetValue(draft)
			m = update(m, ControlIntent("send"))
			if !settings.Autonomous() || m.Busy || m.Composer.Input.Value() != "" {
				t.Fatalf("%q: autonomous=%v busy=%v draft=%q", draft, settings.Autonomous(), m.Busy, m.Composer.Input.Value())
			}
			m.zones.Close()
		}
	})
	t.Run("/review with a prompt selects the mode and sends the prompt", func(t *testing.T) {
		for _, draft := range []string{"/review the auth module", "/Revisar the auth module"} {
			session := navSession(t)
			m, _ := selectionModel(t, &session)
			m.Composer.Input.SetValue(draft)
			m = update(m, ControlIntent("send"))
			if m.deps.Session.Mode() != domain.ModeReview || m.Header.State.Mode != domain.ModeReview {
				t.Fatalf("%q: mode = %q", draft, m.deps.Session.Mode())
			}
			if !sent(t, m) || m.Header.State.Messages[len(m.Header.State.Messages)-1].Content != "the auth module" || m.Composer.Input.Value() != "" {
				t.Fatalf("%q: prompt not sent: busy=%v messages=%v", draft, m.Busy, m.Header.State.Messages)
			}
		}
	})
	t.Run("/review with a prompt while busy keeps the draft", func(t *testing.T) {
		session := navSession(t)
		m, _ := selectionModel(t, &session)
		m.Busy = true
		m.Composer.Input.SetValue("/review the auth module")
		m = update(m, ControlIntent("send"))
		if m.deps.Session.Mode() == domain.ModeReview || m.Composer.Input.Value() != "/review the auth module" || m.Status.Error == "" {
			t.Fatalf("mode=%q draft=%q error=%q", m.deps.Session.Mode(), m.Composer.Input.Value(), m.Status.Error)
		}
	})
	t.Run("/model with text opens the picker filtered", func(t *testing.T) {
		session := navSession(t)
		m, _ := selectionModel(t, &session)
		m.Composer.Input.SetValue("/model gpt-x")
		next, cmd := m.Update(ControlIntent("send"))
		m = drain(t, next.(AppModel), cmd)
		if m.overlay != "models" || m.Models.Input.Value() != "gpt-x" || m.Composer.Input.Value() != "" || len(m.Header.State.Messages) != 0 {
			t.Fatalf("overlay=%q filter=%q draft=%q", m.overlay, m.Models.Input.Value(), m.Composer.Input.Value())
		}
		if _, ok := m.Models.SelectedModel(); ok {
			t.Fatal("the filter did not apply: provider/chosen is listed for gpt-x")
		}
	})
	t.Run("other commands with text show their usage", func(t *testing.T) {
		for draft, usage := range map[string]string{"/copy extra": "/copy", "/usage now": "/usage", "/autonomy maybe": "/autonomy [on|off]", "/plan add a thing": "/plan"} {
			session := navSession(t)
			m, _ := selectionModel(t, &session)
			m.Composer.Input.SetValue(draft)
			m = update(m, ControlIntent("send"))
			if sent(t, m) || m.Busy || m.overlay != "" || m.Status.Notice != m.Theme.Tf("notice.slashUsage", usage) || m.Composer.Input.Value() != draft {
				t.Fatalf("%q: busy=%v overlay=%q notice=%q draft=%q", draft, m.Busy, m.overlay, m.Status.Notice, m.Composer.Input.Value())
			}
		}
	})
	t.Run("unknown words", func(t *testing.T) {
		session := navSession(t)
		m, _ := selectionModel(t, &session)
		m.Composer.Input.SetValue("/Halp")
		m = update(m, ControlIntent("send"))
		if m.Busy || !strings.Contains(m.Status.Error, "/Halp") {
			t.Fatalf("/Halp: busy=%v error=%q", m.Busy, m.Status.Error)
		}
		m.Status.Error = ""
		m.Composer.Input.SetValue("/tmp/foo explain")
		m = update(m, ControlIntent("send"))
		if !sent(t, m) || m.Header.State.Messages[len(m.Header.State.Messages)-1].Content != "/tmp/foo explain" {
			t.Fatalf("/tmp/foo explain was not sent as a prompt: busy=%v error=%q", m.Busy, m.Status.Error)
		}
	})
}

// On 12 October 2026 "/trab" + Enter said "Unknown command" although
// /trabajos is /jobs' alias, the menu hid every alias and showed six rows.
func TestTheSlashMenuMatchesAliasesAndShowsThem(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = typeDraft(m, "/trab")
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "> /jobs  /trabajos") {
		t.Fatalf("the alias is not shown after the command:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	// Without MADE the jobs panel says why it cannot open: /jobs ran.
	if m.Status.Error != m.Theme.T("jobs.unavailable") || m.Busy {
		t.Fatalf("/trab + Enter did not run /jobs: error=%q busy=%v", m.Status.Error, m.Busy)
	}
	short := update(sized(), tea.WindowSizeMsg{Width: 100, Height: 15})
	defer short.zones.Close()
	if limit := short.slashLimit(); limit >= slashSuggestionMax || limit < 1 {
		t.Fatalf("a 15-row console shows %d suggestions", limit)
	}
	short = typeDraft(short, "/")
	rows := 0
	for _, line := range strings.Split(ansi.Strip(short.View().Content), "\n") {
		for _, c := range slashCommands {
			if strings.HasPrefix(line, "  "+c.name+" ") || strings.HasPrefix(line, "> "+c.name+" ") {
				rows++
				break
			}
		}
	}
	if rows != short.slashLimit() {
		t.Fatalf("a short console shows %d rows of suggestions", rows)
	}
}
