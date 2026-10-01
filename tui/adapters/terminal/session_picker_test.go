package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
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
	if !p.ArchivedView || ids(p.Visible()) != "new" {
		t.Fatalf("archived view = %v %s", p.ArchivedView, ids(p.Visible()))
	}
	p.Key("tab", "")
	if p.ArchivedView || p.AllWorkspaces {
		t.Fatal("Tab did not return to this workspace")
	}
}
