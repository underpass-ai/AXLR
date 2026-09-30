package terminal

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// providerWaitTick is scoped to one operation; an expired operation cannot
// update the current turn or restart its timer.
type providerWaitTick struct{ OperationID uint64 }

func (m *AppModel) waitingCommand(next tea.Cmd) tea.Cmd {
	if m.updatingBatch || !m.Busy || !m.providerWaiting || m.waitTickScheduled {
		return next
	}
	m.waitTickScheduled = true
	id := m.operationID
	tick := tea.Tick(time.Second, func(time.Time) tea.Msg { return providerWaitTick{OperationID: id} })
	if next == nil {
		return tick
	}
	return tea.Batch(next, tick)
}
func (m AppModel) statusView() string {
	status := m.Status
	status.Waiting = m.providerWaiting
	if m.providerWaiting {
		status.WaitSeconds = int(time.Since(m.providerWaitStarted) / time.Second)
	}
	return status.View(m.Layout.Width)
}
