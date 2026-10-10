package terminal

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026 the console's memory reminder was drawn as a prompt,
// "› [AXLR · memory] KMP is connected…", as if the person had typed it.
func TestTheMemoryReminderIsAConsoleLineNotAPrompt(t *testing.T) {
	s := domain.SessionState{Messages: []root.Message{
		{Role: root.RoleUser, Content: "fix the error message"},
		{Role: root.RoleAssistant, Content: "Done."},
		{Role: root.RoleUser, Content: "[AXLR · memory] KMP is connected and this turn changed files; record it now with axlr_remember."},
		{Role: root.RoleAssistant, Content: "Recorded."},
	}}
	for _, locale := range []Locale{English, Spanish} {
		theme := Theme{Locale: locale, Monochrome: true}
		tr := NewTranscript()
		tr.SetWidth(120)
		tr.SetSession(s, "", theme)
		text := ansi.Strip(tr.Text())
		if strings.Contains(text, "[AXLR · memory]") || strings.Contains(text, "KMP is connected") {
			t.Fatalf("%s: the reminder is drawn as a prompt:\n%s", locale, text)
		}
		if !strings.Contains(text, theme.Icon("memory")+" "+theme.T("transcript.memoryReminder")) {
			t.Fatalf("%s: no console line for the reminder:\n%s", locale, text)
		}
		if !strings.Contains(text, theme.Icon("user")+" fix the error message") {
			t.Fatalf("%s: the person's prompt lost its marker:\n%s", locale, text)
		}
	}
	m := sized()
	defer m.zones.Close()
	m.Header.State.Messages = s.Messages
	m.resetPromptHistory()
	if len(m.promptHistory) != 1 || m.promptHistory[0] != "fix the error message" {
		t.Fatalf("the reminder is in the prompt history: %q", m.promptHistory)
	}
}
