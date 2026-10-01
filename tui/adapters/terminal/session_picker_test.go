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
