package terminal

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func numberedLines(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "line %03d of the pasted log\n", i)
	}
	return b.String()
}

// On 12 October 2026 a 200-line paste filled the composer to its maximum
// and the conversation with its bubble. It now reads as one token in both,
// and the model receives it whole.
func TestALongPasteIsFoldedButSentWhole(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	paste := numberedLines(200)
	m = typeDraft(m, "explain ")
	m = update(m, tea.PasteMsg{Content: paste})
	token := pasteToken(m.Theme, paste)
	if want := "explain " + token; m.Composer.Input.Value() != want || !strings.HasPrefix(token, "[pasted 200 lines · ") {
		t.Fatalf("composer = %q; want %q", m.Composer.Input.Value(), want)
	}
	if m.Composer.Input.Height() > 2 {
		t.Fatalf("the composer grew to %d rows", m.Composer.Input.Height())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	messages := m.Header.State.Messages
	if !m.Busy || len(messages) == 0 || string(messages[len(messages)-1].Content) != "explain "+paste {
		t.Fatalf("the prompt sent is not the whole paste")
	}
	m.cancel()
	view := ansi.Strip(m.Transcript.View())
	if !strings.Contains(view, "explain "+token) || strings.Contains(view, "line 150") {
		t.Fatalf("the conversation shows the paste:\n%s", view)
	}
	// A click on the folded prompt opens it.
	line := -1
	for i, l := range strings.Split(ansi.Strip(m.Transcript.Viewport.GetContent()), "\n") {
		if strings.Contains(l, token) {
			line = i
		}
	}
	key, ok := m.Transcript.NoteAt(line)
	if !ok {
		t.Fatal("the folded prompt cannot be opened")
	}
	m.Transcript.ToggleNote(key)
	m.refreshTranscript()
	if !strings.Contains(m.Transcript.Viewport.GetContent(), "line 150 of the pasted log") {
		t.Fatal("the opened prompt does not show the paste")
	}
}

// tmux paste-buffer and GNOME Terminal turn each pasted newline into a
// carriage return; on 12 October 2026 such a 200-line paste was not folded.
func TestAPasteWithCarriageReturnsIsFolded(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	paste := numberedLines(200)
	m = update(m, tea.PasteMsg{Content: strings.ReplaceAll(paste, "\n", "\r")})
	if want := pasteToken(m.Theme, paste); m.Composer.Input.Value() != want {
		t.Fatalf("composer = %q; want %q", m.Composer.Input.Value(), want)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	messages := m.Header.State.Messages
	if len(messages) == 0 || string(messages[len(messages)-1].Content) != paste {
		t.Fatal("the prompt sent is not the paste with its newlines")
	}
	m.cancel()
}

func TestShortPastesStayAsTypedAndTwoLongOnesKeepTheirOwnText(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = update(m, tea.PasteMsg{Content: "one\ntwo"})
	if m.Composer.Input.Value() != "one\ntwo" || len(m.pastes) != 0 {
		t.Fatalf("a short paste was folded: %q", m.Composer.Input.Value())
	}
	m.Composer.Input.Reset()
	first, second := strings.Repeat("a\n", 30), strings.Repeat("b\n", 30)
	m = update(m, tea.PasteMsg{Content: first})
	m = typeDraft(m, " and ")
	m = update(m, tea.PasteMsg{Content: second})
	if len(m.pastes) != 2 || m.pastes[0].token == m.pastes[1].token {
		t.Fatalf("pastes = %+v", m.pastes)
	}
	if got := m.expandPastes(m.Composer.Input.Value()); got != first+" and "+second {
		t.Fatalf("expanded = %q", got)
	}
	// A prompt saved earlier and longer than the fold is folded too.
	shown, folded := foldPastes(numberedLines(25), nil, m.Theme)
	if !folded || shown != m.Theme.Tn("paste.folded", 25, 25, formatBytes(len(numberedLines(25)))) {
		t.Fatalf("an unknown long prompt reads %q", shown)
	}
	if shown, folded := foldPastes("short", nil, m.Theme); folded || shown != "short" {
		t.Fatalf("a short prompt reads %q", shown)
	}
}

func TestAQueuedLongPasteIsQueuedWhole(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy, m.providerWaiting = true, true
	paste := numberedLines(30)
	m = update(m, tea.PasteMsg{Content: paste})
	m = update(m, ControlIntent("send"))
	if m.steer.Peek() != paste || len(m.pastes) != 0 {
		t.Fatalf("queued %d bytes of %d", len(m.steer.Peek()), len(paste))
	}
	if view := ansi.Strip(m.Transcript.View()); strings.Contains(view, "line 020") || !strings.Contains(view, pasteToken(m.Theme, paste)) {
		t.Fatalf("the queued paste is not folded:\n%s", view)
	}
}
