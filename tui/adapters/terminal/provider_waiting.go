package terminal

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// providerWaitTick is scoped to one operation; an expired operation cannot
// update the current turn or restart its timer.
type providerWaitTick struct{ OperationID uint64 }
type providerAnimationTick struct {
	OperationID uint64
	Tick        tea.Msg
}

func (m *AppModel) waitingCommand(next tea.Cmd) tea.Cmd {
	if m.updatingBatch || !m.Busy {
		return next
	}
	var cmds []tea.Cmd
	if next != nil {
		cmds = append(cmds, next)
	}
	if (m.providerWaiting || m.toolExecuting) && !m.waitTickScheduled {
		m.waitTickScheduled = true
		id := m.operationID
		cmds = append(cmds, tea.Tick(time.Second, func(time.Time) tea.Msg { return providerWaitTick{OperationID: id} }))
	}
	if (m.providerWaiting || m.toolExecuting) && !m.UIPreferences.ReduceMotion && !m.spinnerScheduled {
		m.spinnerScheduled = true
		id := m.operationID
		frame := m.activitySpinner.Tick()
		cmds = append(cmds, tea.Tick(125*time.Millisecond, func(time.Time) tea.Msg { return providerAnimationTick{OperationID: id, Tick: frame} }))
	}
	if len(cmds) == 0 {
		return nil
	}
	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}
func (m AppModel) statusView() string {
	if m.Status.Error != "" {
		return m.errorRow()
	}
	status := m.Status
	status.Waiting = m.providerWaiting
	status.Executing = m.toolExecuting
	status.ToolName = m.toolName
	status.PreparingTool, status.PreparingBytes = m.toolCallName, m.toolCallBytes
	if m.providerWaiting || m.toolExecuting {
		if m.UIPreferences.ReduceMotion {
			status.Indicator = m.Theme.Icon("waiting")
		} else {
			status.Indicator = m.Theme.Accent(m.activitySpinner.View())
		}
	}
	if m.providerWaiting {
		status.WaitSeconds = int(time.Since(m.providerWaitStarted) / time.Second)
	}
	if m.toolExecuting {
		status.ToolSeconds = int(time.Since(m.toolStarted) / time.Second)
	}
	return m.Theme.overlayLine(status.View(max(1, m.Layout.Width-4)), m.Layout.Width, true)
}
