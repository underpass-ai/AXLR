package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func pickerFixture(now time.Time) SessionPicker {
	here, there := domain.Workspace("/w/here"), domain.Workspace("/w/there")
	return NewSessionPicker([]domain.SessionSummary{
		{ID: "old", Workspace: here, Model: "m", Title: "revisa el contrato", MessageCount: 22, UpdatedAt: now.Add(-72 * time.Hour)},
		{ID: "empty", Workspace: here, Model: "m", MessageCount: 0, UpdatedAt: now},
		{ID: "new", Workspace: here, Model: "m", Title: "¿qué ceremonias\ntienes?", MessageCount: 14, Status: domain.StatusInterrupted, UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "away", Workspace: there, Model: "m", Title: "otra cosa", MessageCount: 3, UpdatedAt: now.Add(-time.Minute * 5)},
	}, here)
}

func ids(list []domain.SessionSummary) string {
	var out []string
	for _, s := range list {
		out = append(out, string(s.ID))
	}
	return strings.Join(out, ",")
}

func TestSessionPickerShowsThisWorkspaceNewestFirstWithoutEmptySessions(t *testing.T) {
	p := pickerFixture(time.Now())
	if got := ids(p.Visible()); got != "new,old" {
		t.Fatalf("visible = %s", got)
	}
	p.Key("tab", "")
	if got := ids(p.Visible()); got != "away,new,old" {
		t.Fatalf("all workspaces = %s", got)
	}
	for _, r := range "CEREM" {
		p.Key(string(r), string(r))
	}
	if got := ids(p.Visible()); got != "new" {
		t.Fatalf("filtered = %s", got)
	}
	p.Key("backspace", "")
	if p.Query != "CERE" {
		t.Fatalf("backspace left %q", p.Query)
	}
}

// On 10 October 2026 resuming needed --session <32-hex id>, which neither
// the exit nor this panel showed.
func TestSessionPickerShowsTheSelectedSessionsID(t *testing.T) {
	now := time.Date(2026, 10, 1, 18, 0, 0, 0, time.Local)
	z := zone.New()
	defer z.Close()
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	p := NewSessionPicker([]domain.SessionSummary{
		{ID: id, Workspace: "/w", Model: "m", Title: "first", MessageCount: 2, UpdatedAt: now},
		{ID: "fedcba9876543210fedcba9876543210", Workspace: "/w", Model: "m", Title: "second", MessageCount: 2, UpdatedAt: now.Add(-time.Hour)},
	}, "/w")
	view := ansi.Strip(p.view(Theme{Monochrome: true}, z, "p", 30, 90, now))
	if !strings.Contains(view, string(id)) || strings.Contains(view, "fedcba98") {
		t.Fatalf("the selected session's id is not on its line alone:\n%s", view)
	}
}

func TestSessionPickerViewShowsTitleAgeGroupAndState(t *testing.T) {
	now := time.Date(2026, 10, 1, 18, 0, 0, 0, time.Local)
	z := zone.New()
	defer z.Close()
	view := ansi.Strip(pickerFixture(now).view(Theme{Monochrome: true}, z, "p", 30, 90, now))
	for _, want := range []string{"TODAY", "¿qué ceremonias tienes?", "2 h ago", "14 messages", "interrupted", "EARLIER", "revisa el contrato", "3 days ago", "2 sessions · this workspace"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "0123") || strings.Contains(view, "/w/here") {
		t.Fatalf("view shows ids or the workspace in this-workspace scope:\n%s", view)
	}
}

func TestSessionPickerRenamesArchivesAndCyclesScopes(t *testing.T) {
	p := pickerFixture(time.Now())
	p.Labels = map[domain.SessionID]domain.SessionLabel{"old": {Title: "Contrato revisado"}}
	if title := p.Title(p.Visible()[1]); title != "Contrato revisado" {
		t.Fatalf("label title not used: %q", title)
	}
	if p.Key("f2", "") != "" || !p.Renaming || p.RenameText != "¿qué ceremonias tienes?" {
		t.Fatalf("F2 did not start renaming: %+v", p)
	}
	// An arrow keeps the selected title for editing at its end.
	p.Key("right", "")
	p.Key("backspace", "")
	p.Key("!", "!")
	if p.RenameText != "¿qué ceremonias tienes!" {
		t.Fatalf("rename text = %q", p.RenameText)
	}
	if p.Key("enter", "") != SessionRenameIntent || p.Renaming {
		t.Fatal("Enter did not ask to save the title")
	}
	if p.Key("ctrl+x", "") != SessionArchiveIntent {
		t.Fatal("Ctrl+X did not ask to archive")
	}
	p.Labels["new"] = domain.SessionLabel{Archived: true}
	if got := ids(p.Visible()); got != "old" {
		t.Fatalf("archived session still listed: %s", got)
	}
	p.Key("tab", "")
	p.Key("tab", "")
	if !p.TasksView || p.ArchivedView || ids(p.Visible()) != "" {
		t.Fatalf("plan tasks view = %v %s", p.TasksView, ids(p.Visible()))
	}
	p.Key("tab", "")
	if !p.ArchivedView || ids(p.Visible()) != "new" {
		t.Fatalf("archived view = %v %s", p.ArchivedView, ids(p.Visible()))
	}
	p.Key("tab", "")
	if p.ArchivedView || p.AllWorkspaces || p.TasksView {
		t.Fatal("Tab did not return to this workspace")
	}
}

// On 10 October 2026 Ctrl+O listed a plan's worker session next to the
// person's, titled by its context pack; a plan of 12 tasks adds 12 rows.
func TestPlanTaskSessionsHaveTheirOwnScope(t *testing.T) {
	now := time.Now()
	here := domain.Workspace("/w/here")
	p := NewSessionPicker([]domain.SessionSummary{
		{ID: "mine", Workspace: here, Model: "m", Title: "add buildinfo.Short()", MessageCount: 9, UpdatedAt: now.Add(-time.Hour)},
		{ID: "worker", Workspace: here, Model: "m", Title: "Task buildinfo-short of plan add-a-func-dce953, wave 1: Add func Short() string and cover it with a table test", MessageCount: 12, UpdatedAt: now},
		{ID: "pruned", Workspace: here, Model: "m", Title: "Task docs of plan older-plan, wave 2: Document it", MessageCount: 4, UpdatedAt: now.Add(-2 * time.Hour)},
	}, here)
	p.Tasks = planTaskSessions([]domain.PlanRecord{{ID: "add-a-func-dce953", Tasks: []domain.PlanTask{{ID: "buildinfo-short", Session: "worker", Status: domain.TaskDone}}}})
	if got := ids(p.Visible()); got != "mine" {
		t.Fatalf("this workspace lists plan task sessions: %s", got)
	}
	p.Key("tab", "")
	if got := ids(p.Visible()); got != "mine" {
		t.Fatalf("every workspace lists plan task sessions: %s", got)
	}
	p.Key("tab", "")
	if got := ids(p.Visible()); !p.TasksView || got != "worker,pruned" {
		t.Fatalf("plan tasks scope = %s", got)
	}
	theme := Theme{Monochrome: true}
	z := zone.New()
	defer z.Close()
	view := ansi.Strip(p.view(theme, z, "", 30, 100, now))
	for _, want := range []string{"task buildinfo-short · plan add-a-func-dce953", "task docs · plan older-plan", "done", "plan tasks"} {
		if !strings.Contains(view, want) {
			t.Fatalf("plan tasks view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Add func Short()") {
		t.Fatalf("a task session is still titled by its context pack:\n%s", view)
	}
}

// On 12 October 2026 F2 prefilled the old title with the cursor at its end,
// and typing appended ("…responde solo okQA buildinfo").
func TestRenamingStartsWithTheOldTitleSelected(t *testing.T) {
	p := pickerFixture(time.Now())
	p.Key("f2", "")
	view := ansi.Strip(p.View(Theme{Monochrome: true}, zone.New(), "", 30, 100))
	if !p.RenameSelected || !strings.Contains(view, "[¿qué ceremonias tienes?]") {
		t.Fatalf("the old title is not shown selected:\n%s", view)
	}
	for _, r := range "QA buildinfo" {
		p.Key(string(r), string(r))
	}
	if p.RenameText != "QA buildinfo" || p.RenameSelected {
		t.Fatalf("typing did not replace the title: %q", p.RenameText)
	}
	p.Key("ctrl+u", "")
	if p.RenameText != "" || !p.Renaming {
		t.Fatalf("Ctrl+U did not clear the title: %q", p.RenameText)
	}
	p.Key("esc", "")
	p.Key("f2", "")
	p.Key("backspace", "")
	if p.RenameText != "" {
		t.Fatalf("Backspace on the selected title left %q", p.RenameText)
	}
}

// manySessions is a picker over n sessions of this workspace, newest first.
func manySessions(n int, now time.Time) SessionPicker {
	var items []domain.SessionSummary
	for i := range n {
		items = append(items, domain.SessionSummary{ID: domain.SessionID(fmt.Sprintf("%032x", i)), Workspace: "/w", Title: root.Text(fmt.Sprintf("prompt %d", i)), MessageCount: 2, UpdatedAt: now.Add(-time.Duration(i) * time.Minute)})
	}
	return NewSessionPicker(items, "/w")
}

// On 12 October 2026 the picker had no PgUp, PgDn, Home or End, and a paste
// into its filter was swallowed.
func TestSessionPickerPagesAndTakesPastes(t *testing.T) {
	p := manySessions(30, time.Now())
	p.NewRow, p.Page = true, 5
	p.Key("pgdown", "")
	p.Key("pgdown", "")
	if p.Selected != 10 {
		t.Fatalf("PgDn twice selected %d; want 10", p.Selected)
	}
	p.Key("pgup", "")
	if p.Selected != 5 {
		t.Fatalf("PgUp selected %d; want 5", p.Selected)
	}
	p.Key("end", "")
	if p.Selected != 29 || p.OnNew {
		t.Fatalf("End selected %d", p.Selected)
	}
	p.Key("home", "")
	if p.Selected != 0 || !p.NewSelected() {
		t.Fatalf("Home selected %d new=%v", p.Selected, p.OnNew)
	}
	p.Key("pgdown", "")
	if p.Selected != 5 || p.OnNew {
		t.Fatalf("PgDn from the new-session row selected %d", p.Selected)
	}

	s := navSession(t)
	m := navModel(t, &s)
	m.overlay, m.Picker = "sessions", manySessions(30, time.Now())
	m = update(m, tea.PasteMsg{Content: "prompt 29\n"})
	if m.Picker.Query != "prompt 29" || len(m.Picker.Visible()) != 1 {
		t.Fatalf("the paste did not filter: query=%q visible=%d", m.Picker.Query, len(m.Picker.Visible()))
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyF2})
	m = update(m, tea.PasteMsg{Content: "pasted title"})
	if m.Picker.RenameText != "pasted title" {
		t.Fatalf("the paste did not replace the selected title: %q", m.Picker.RenameText)
	}
	// The root gives the picker its page from the screen.
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.Picker.Page != m.Picker.PageSize(m.Layout.Height-1, m.Layout.Width) {
		t.Fatalf("page = %d", m.Picker.Page)
	}
}

// On 12 October 2026 the filter matched titles only: a session was found
// by its first prompt or not at all.
func TestSessionPickerFiltersByConversation(t *testing.T) {
	p := NewSessionPicker([]domain.SessionSummary{
		{ID: "a", Workspace: "/w", Title: "first", Text: "first\nWe chose the Zebra plan", MessageCount: 2},
		{ID: "b", Workspace: "/w", Title: "second", Text: "second\nnothing here", MessageCount: 2},
	}, "/w")
	p.Paste("zebra")
	if got := ids(p.Visible()); got != "a" {
		t.Fatalf("filtering by content listed %q", got)
	}
	view := ansi.Strip(p.View(Theme{Monochrome: true}, zone.New(), "", 30, 100))
	if !strings.Contains(view, Theme{}.T("sessions.contentMatch")) {
		t.Fatalf("a content match is not marked:\n%s", view)
	}
}

// On 14 October 2026 Ctrl+O listed a session open in another console like
// any other, and Enter on it failed with "Error:…" cut short behind the
// notice before it.
func TestSessionPickerMarksSessionsOpenHereAndElsewhere(t *testing.T) {
	s := navSession(t)
	if err := s.BeginTurn("mine", nil); err != nil {
		t.Fatal(err)
	}
	m := navModel(t, &s)
	m.Header.State = s.Export()
	if err := m.deps.Store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	other, _ := domain.NewSession("1123456789abcdef0123456789abcdef", testWorkspace(), "model")
	if err := other.BeginTurn("theirs", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.deps.Store.Save(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	m.deps.SessionHeld = func(id domain.SessionID) bool { return id == other.ID() }
	n, c := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), c)
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "open in another console") || !strings.Contains(view, "open here") {
		t.Fatalf("no marks:\n%s", view)
	}
	for m.Picker.Visible()[m.Picker.Selected].ID != other.ID() {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	m.Status.Notice = "Exported to /tmp/a.md"
	n, c = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = n.(AppModel)
	if c != nil || m.Busy || m.Status.Error != m.Theme.T("sessions.heldOpen") || m.Status.Notice != "" {
		t.Fatalf("Enter on a held session: busy=%v error=%q notice=%q", m.Busy, m.Status.Error, m.Status.Notice)
	}
	view = ansi.Strip(m.View().Content)
	if lines := strings.Split(view, "\n"); len(lines) != m.Layout.Height || !strings.Contains(strings.Join(strings.Fields(strings.Join(lines[len(lines)-2:], " ")), " "), "close it there to open it here") {
		t.Fatalf("the reason is not readable:\n%s", view)
	}
}

// A new error displaces a notice an earlier update left.
func TestANewErrorDisplacesAnOldNotice(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Status.Notice = "Exported to /tmp/a.md"
	m.Busy, m.operationID = true, 1
	m = update(m, operationComplete{ID: 1, Session: *m.deps.Session, Err: errors.New("could not open it")})
	if m.Status.Notice != "" || !strings.Contains(m.Status.Error, "could not open it") {
		t.Fatalf("notice %q error %q", m.Status.Notice, m.Status.Error)
	}
}
