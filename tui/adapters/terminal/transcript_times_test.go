package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestTurnTimesShowOnPromptsAndSpeakers(t *testing.T) {
	today := time.Now()
	at := time.Date(today.Year(), today.Month(), today.Day(), 9, 5, 0, 0, time.Local).UTC()
	state := domain.SessionState{
		Messages:     []root.Message{{Role: root.RoleUser, Content: "hola"}, {Role: root.RoleAssistant, Content: "buenas"}},
		MessageTimes: []time.Time{at, at},
	}
	margin := NewTranscript()
	margin.SetWidth(40)
	margin.SetSession(state, "", Theme{Monochrome: true})
	first := strings.Split(ansi.Strip(margin.Text()), "\n")[0]
	if !strings.HasPrefix(first, "> hola") || !strings.HasSuffix(first, "09:05") || ansi.StringWidth(first) != 40 {
		t.Fatalf("Margen prompt row = %q", first)
	}
	editorial := NewTranscript()
	editorial.SetWidth(40)
	editorial.SetSession(state, "", Theme{Monochrome: true, ID: domain.ThemeEditorial})
	lines := strings.Split(ansi.Strip(editorial.Text()), "\n")
	if !strings.HasPrefix(lines[0], "You") || !strings.HasSuffix(lines[0], "09:05") || lines[1] != "hola" {
		t.Fatalf("Editorial rows = %q", lines)
	}
	if got := formatClock(at.AddDate(0, 0, -3), today); !strings.Contains(got, "-") {
		t.Fatalf("older days need a date: %q", got)
	}
	if formatClock(time.Time{}, today) != "" {
		t.Fatal("unknown time produced a label")
	}
}
