package terminal

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func changeState() domain.SessionState {
	return domain.SessionState{ID: "0123456789abcdef0123456789abcdef", Activity: []domain.PendingTool{
		{Call: root.ToolCall{ID: "first"}, Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{Change: &domain.FileChange{Path: "new.go", Created: true, After: "package main\n"}}},
		{Call: root.ToolCall{ID: "second"}, Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{Change: &domain.FileChange{Path: "src/main.go", Before: "package main\n\nfunc main() {\n    old()\n}\n", After: "package main\n\nfunc main() {\n    new()\n}\n"}}},
	}}
}

func TestChangePreviewLineNumbersCountsAndMissingNewline(t *testing.T) {
	p := previewChange(domain.FileChange{Path: "file", Before: "same\nold\ntail\n", After: "same\nnew\nextra\ntail"})
	if p.Added != 3 || p.Removed != 2 {
		t.Fatalf("wrong line counts +%d -%d", p.Added, p.Removed)
	}
	text := p.render(Theme{Monochrome: true})
	for _, want := range []string{"   1    1   same", "   2      - old", "        2 + new", "        3 + extra", "No newline at end of file"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	p = previewChange(domain.FileChange{Path: "file", Created: true, After: "line\n"})
	if p.Added != 1 || p.Removed != 0 {
		t.Fatalf("new file counts: %+v", p)
	}
	p = previewChange(domain.FileChange{Path: "file", Before: "same", After: "same"})
	if len(p.Lines) != 0 {
		t.Fatal("identical snapshots produced a diff")
	}
}

func TestChangesCommandsPreserveDraftAndDoNotStartAnAgentTurn(t *testing.T) {
	for _, command := range []string{"/changes", "/diff"} {
		m := sized()
		m.Composer.Input.SetValue(command)
		next, cmd := m.Update(ControlIntent("send"))
		m = next.(AppModel)
		if cmd != nil || m.Busy || m.overlay != "changes" || m.Composer.Input.Value() != "" {
			t.Fatalf("%s sent to agent", command)
		}
		if !strings.Contains(m.View().Content, "No file changes yet") {
			t.Fatal("empty state missing")
		}
		m.zones.Close()
	}
	m := sized()
	defer m.zones.Close()
	m.Composer.Input.SetValue("draft to keep")
	m.Busy = true
	m = update(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.overlay != "changes" {
		t.Fatal("cannot inspect changes during streaming")
	}
	m = update(m, tea.PasteMsg{Content: "ignored"})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.Composer.Input.Value() != "draft to keep" || !m.Busy {
		t.Fatal("review changed draft or cancelled turn")
	}
	m.Busy = false
	m = update(m, ControlIntent("palette"))
	m = update(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.overlay != "changes" {
		t.Fatal("palette shortcut missing")
	}
}

func TestChangesNavigateResizeAndKeepSnapshotInsteadOfLiveFiles(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Header.State = changeState()
	m.refreshTranscript()
	m = update(m, ControlIntent("changes"))
	if !strings.Contains(m.View().Content, "old()") || !strings.Contains(m.View().Content, "new()") {
		t.Fatal("recorded before/after missing")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.Changes.records[m.Changes.selected].ID != "first" {
		t.Fatal("file selection did not move")
	}
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if !strings.Contains(m.View().Content, "package main") {
		t.Fatal("narrow terminal did not switch to diff")
	}
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.Changes.records[m.Changes.selected].ID != "first" {
		t.Fatal("resize lost selected change")
	}
	next := changeState()
	next.Activity = append(next.Activity, domain.PendingTool{Call: root.ToolCall{ID: "third"}, Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{Change: &domain.FileChange{Path: "other", Created: true, After: "other\n"}}})
	m = update(m, application.Event{Kind: application.EventSession, Snapshot: &next})
	if m.Changes.records[m.Changes.selected].ID != "first" || len(m.Changes.records) != 3 {
		t.Fatal("new published changes stole review selection")
	}
	next.ID = "fedcba9876543210fedcba9876543210"
	next.Activity = nil
	m = update(m, application.Event{Kind: application.EventSession, Snapshot: &next})
	if len(m.Changes.records) != 0 || !strings.Contains(m.View().Content, "No file changes yet") {
		t.Fatal("changes leaked across sessions")
	}
}

func TestChangesFitAllThemesAndSanitizeToolContent(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	for _, theme := range []domain.ThemeID{domain.ThemeInk, domain.ThemeAurora, domain.ThemePaper, domain.ThemePhosphor} {
		for _, size := range [][2]int{{50, 15}, {80, 24}, {100, 32}} {
			m := New(Dependencies{Locale: Spanish, UIPreferences: domain.UIPreferences{Theme: theme, Icons: domain.IconsSafe}})
			m = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			state := changeState()
			state.Activity[1].Outcome.Change.After += "safe\x1b]52;c;secret\a\x1b[2Jtext\n"
			m.Header.State = state
			m.refreshTranscript()
			m = update(m, ControlIntent("changes"))
			for _, focus := range []bool{false, true} {
				m.Changes.detailFocus = focus
				view := m.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("overflow at %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
				}
				if strings.Contains(view, "secret") || strings.Contains(view, "\x1b[2J") || !strings.Contains(ansi.Strip(view), "Cambios en archivos") {
					t.Fatal("unsafe or untranslated diff")
				}
				m.Theme.Monochrome = true
				m.Changes.Theme = m.Theme
				if strings.Contains(m.View().Content, "\x1b") {
					t.Fatal("NO_COLOR review contains escapes")
				}
				m.Theme.Monochrome = false
				m.Changes.Theme = m.Theme
			}
			m.zones.Close()
		}
	}
}

func TestChangeReviewScrollsAndMouseSelectsFiles(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	state := changeState()
	state.Activity[1].Outcome.Change.After = root.Text(strings.Repeat("new line\n", 80) + strings.Repeat("wide", 50))
	m.Header.State = state
	m.refreshTranscript()
	m = update(m, ControlIntent("changes"))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.Changes.Details.YOffset() == 0 {
		t.Fatal("page down did not scroll diff")
	}
	offset := m.Changes.Details.YOffset()
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.Changes.Details.YOffset() != offset {
		t.Fatal("resize lost scroll")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.Changes.Details.XOffset() == 0 {
		t.Fatal("long diff lines cannot be inspected")
	}
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.View()
	z := m.zones.Get(m.prefix + "changes-change-1")
	for deadline := time.Now().Add(time.Second); z == nil && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		z = m.zones.Get(m.prefix + "changes-change-1")
	}
	if z == nil {
		t.Fatal("file row is not clickable")
	}
	m = update(m, tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft})
	if m.Changes.selected != 1 || m.Changes.Details.YOffset() != 0 {
		t.Fatal("mouse selected wrong file or retained old scroll")
	}
}

func TestChangeReviewExplainsUnavailableAndEmptyPreviews(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	m.Theme.Locale, m.Changes.Theme.Locale = Spanish, Spanish
	for _, change := range []domain.FileChange{
		{Path: "large", Unavailable: "too_large"},
		{Path: "unknown", Unavailable: "unavailable"},
		{Path: "empty", Created: true},
	} {
		state := changeState()
		state.Activity = []domain.PendingTool{{Call: root.ToolCall{ID: "notice"}, Decision: domain.DecisionApprove, Outcome: &domain.ToolOutcome{Change: &change}}}
		m.Header.State = state
		m.refreshTranscript()
		m = update(m, ControlIntent("changes"))
		m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
		content := m.Changes.Details.GetContent()
		if change.Unavailable == "" {
			if !strings.Contains(content, "Archivo vacío creado") {
				t.Fatal("empty creation not explained")
			}
		} else {
			if strings.Contains(m.View().Content, "+0 -0") || !strings.Contains(content, "Archivo modificado") {
				t.Fatal("unavailable preview invented counts or hid status")
			}
			for _, line := range strings.Split(content, "\n") {
				if ansi.StringWidth(line) > 46 {
					t.Fatal("notice cannot be read on narrow terminal")
				}
			}
		}
	}
}
