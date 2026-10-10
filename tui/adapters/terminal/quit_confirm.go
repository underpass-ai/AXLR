package terminal

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// quitConfirmWindow is how long a first Ctrl+C on an idle console waits
// for the second one that quits.
const quitConfirmWindow = 2 * time.Second

// quitDisarm ends the window a first Ctrl+C opened, unless a newer one
// opened since.
type quitDisarm struct{ arm uint64 }

// quitKey is Ctrl+C when no operation runs. On 10 October 2026 one Ctrl+C
// on an idle console quit at once, so a Ctrl+C meant to cancel a turn that
// had just ended, or a copy reflex, closed the console. The first Ctrl+C
// now clears a draft (↑ brings it back) or, with nothing to clear, asks for
// a second one within two seconds; that one quits.
func (m AppModel) quitKey() (tea.Model, tea.Cmd) {
	if !m.quitArmedAt.IsZero() && time.Since(m.quitArmedAt) <= quitConfirmWindow {
		return m.quit()
	}
	if draft := m.Composer.Input.Value(); m.overlay == "" && draft != "" {
		m.rememberPrompt(draft)
		m.Composer.Input.Reset()
		m.slashSelected, m.slashTop = 0, 0
		m.resizeComposer()
		m.quitArmedAt = time.Time{}
		return m, nil
	}
	m.quitArmedAt = time.Now()
	m.quitArm++
	m.Status.Notice = m.Theme.T("notice.quitAgain")
	arm := m.quitArm
	return m, tea.Tick(quitConfirmWindow, func(time.Time) tea.Msg { return quitDisarm{arm: arm} })
}
