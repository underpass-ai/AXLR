package terminal

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
)

// Scrolled up, a row above the composer says how many lines arrived since;
// End with an empty composer, Ctrl+End or a click on the row go back down
// and the row goes; Ctrl+Home goes to the top. With a draft End is the
// composer's.
func TestTheScrollPillCountsNewLinesAndJumpsBack(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	for i := 0; i < 40; i++ {
		m.Header.State.Messages = append(m.Header.State.Messages, root.Message{Role: root.RoleUser, Content: root.Text(fmt.Sprintf("prompt %d", i))})
	}
	m.refreshTranscript()
	if strings.Contains(ansi.Strip(m.View().Content), "End") {
		t.Fatal("a pill at the bottom")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "↓ more below · End") {
		t.Fatalf("no pill after PgUp:\n%s", view)
	}
	m.draft = "streamed\nthree\nlines"
	m.refreshTranscript()
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "↓ 4 new lines · End") {
		t.Fatalf("new lines not counted:\n%s", view)
	}
	m.Composer.Input.SetValue("draft")
	if m = update(m, tea.KeyPressMsg{Code: tea.KeyEnd}); m.Transcript.Viewport.AtBottom() {
		t.Fatal("End with a draft left the composer")
	}
	m.Composer.Input.SetValue("")
	if m = update(m, tea.KeyPressMsg{Code: tea.KeyEnd}); !m.Transcript.Viewport.AtBottom() || strings.Contains(ansi.Strip(m.View().Content), "new lines") {
		t.Fatal("End did not go back down")
	}
	if m = update(m, tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}); m.Transcript.Viewport.YOffset() != 0 {
		t.Fatal("Ctrl+Home did not go to the top")
	}
	if m = update(m, tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl}); !m.Transcript.Viewport.AtBottom() {
		t.Fatal("Ctrl+End did not go to the bottom")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	n, _ := click(t, m, "jump-bottom")
	if m = n.(AppModel); !m.Transcript.Viewport.AtBottom() {
		t.Fatal("a click on the pill did not go back down")
	}
}
