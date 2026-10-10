package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var ctrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// On 10 October 2026 one Ctrl+C on an idle console quit at once: a Ctrl+C
// meant to cancel a turn that had just ended closed the console instead.
func TestAnIdleConsoleQuitsOnASecondCtrlC(t *testing.T) {
	m := sized()
	next, cmd := m.Update(ctrlC)
	m = next.(AppModel)
	if cmd == nil || m.quitArmedAt.IsZero() || m.Status.Notice != m.Theme.T("notice.quitAgain") {
		t.Fatalf("the first Ctrl+C did not ask for a second one: notice %q", m.Status.Notice)
	}
	if footer := ansi.Strip(m.footerView()); !strings.Contains(footer, "press Ctrl+C again to quit") {
		t.Fatalf("the footer does not say how to quit: %q", footer)
	}
	if _, cmd = m.Update(ctrlC); !quits(cmd) {
		t.Fatal("a second Ctrl+C within the window did not quit")
	}
}

func TestTheFirstCtrlCClearsADraftAndKeepsItInHistory(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Composer.Input.SetValue("half a thought")
	m = update(m, ctrlC)
	if m.Composer.Input.Value() != "" || !m.quitArmedAt.IsZero() || m.Status.Notice != "" {
		t.Fatalf("the draft was not just cleared: %q, armed %v", m.Composer.Input.Value(), !m.quitArmedAt.IsZero())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Composer.Input.Value() != "half a thought" {
		t.Fatalf("↑ did not bring the cleared draft back: %q", m.Composer.Input.Value())
	}
}

func TestTheQuitWindowCloses(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = update(m, ctrlC)
	// Two seconds later the window is closed: a Ctrl+C asks again.
	m.quitArmedAt = time.Now().Add(-3 * time.Second)
	next, cmd := m.Update(ctrlC)
	m = next.(AppModel)
	if cmd == nil || m.quitArmedAt.IsZero() || time.Since(m.quitArmedAt) > time.Second {
		t.Fatal("a Ctrl+C after the window did not ask again")
	}
	// The tick that closes the window clears the notice; an older one does not.
	m = update(m, quitDisarm{arm: m.quitArm - 1})
	if m.quitArmedAt.IsZero() {
		t.Fatal("an older window's tick disarmed the newer one")
	}
	m = update(m, quitDisarm{arm: m.quitArm})
	if !m.quitArmedAt.IsZero() || m.Status.Notice != "" {
		t.Fatalf("the window's tick did not disarm it: notice %q", m.Status.Notice)
	}
	m = update(m, ctrlC)
	m = update(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !m.quitArmedAt.IsZero() {
		t.Fatal("another key did not disarm the first Ctrl+C")
	}
}

func TestCtrlCStillCancelsABusyConsole(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	ctx, cancel := context.WithCancel(context.Background())
	m.Busy, m.cancel = true, cancel
	next, cmd := m.Update(ctrlC)
	if ctx.Err() == nil || quits(cmd) || !next.(AppModel).quitArmedAt.IsZero() {
		t.Fatal("Ctrl+C on a busy console did not just cancel")
	}
}
