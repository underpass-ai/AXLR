package terminal

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// NewSessionIntent starts a new session in the console's workspace, from
// /new, the palette or the first row of Ctrl+O.
const NewSessionIntent ControlIntent = "new-session"

// newSession starts an empty session in the same workspace with the
// session's model, in normal mode, and shows it; the previous one stays in
// Ctrl+O. The composer's draft is kept. On 12 October 2026 the only way to
// a new session was to quit and launch again. It is refused while a turn
// or operation runs or a tool call waits for a decision, and says why.
func (m AppModel) newSession() (AppModel, tea.Cmd) {
	m.overlay = ""
	switch {
	case m.Busy:
		m.Status.Notice = m.Theme.T("notice.newBusy")
		return m, nil
	case m.hasPendingCalls():
		m.Status.Notice = m.Theme.T("notice.newPending")
		return m, nil
	case m.Header.State.Model == "":
		m.Status.Notice = m.Theme.T("notice.newNoModel")
		return m, nil
	case m.Header.State.ID != "" && len(m.Header.State.Messages) == 0 && len(m.Header.State.ConsoleLines) == 0:
		m.Status.Notice = m.Theme.T("notice.newAlready")
		return m, nil
	}
	create, leave, sessions := m.deps.Create, m.deps.Leave, m.deps.Sessions
	workspace, model := m.Header.State.Workspace, m.Header.State.Model
	if workspace == "" {
		workspace = m.deps.Workspace
	}
	m.Status.Error = ""
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
		created, err := create.ExecuteNew(ctx, workspace, model)
		if err != nil {
			return err
		}
		if leave != nil && s != nil {
			// The session left closes its MADE instance for this console.
			_ = leave(ctx, s)
		}
		releaseLeft(sessions, s, created.Export().ID)
		*s = created
		return nil
	})
	return m, func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok {
			done.Fresh = true
			return done
		}
		return msg
	}
}

// hasPendingCalls reports tool calls of the open session that wait for a
// decision.
func (m AppModel) hasPendingCalls() bool {
	if _, pending := m.pending(); pending {
		return true
	}
	return m.deps.Session != nil && len(m.deps.Session.Pending()) != 0
}

// releaseLeft gives back the writer lock of the session the console
// switched away from, once whatever leaving it saved is written: on 14
// October 2026 a console that had opened another session with Ctrl+O still
// held both locks, and --continue in a second console called the old one
// "open in another console". The store's lock serializes the release with
// any save, and a later save of the left session takes the lock again.
// nil sessions (no store that locks) release nothing.
func releaseLeft(sessions application.SessionLister, left *domain.Session, opened domain.SessionID) {
	if sessions == nil || left == nil {
		return
	}
	if id := left.Export().ID; id != "" && id != opened {
		_ = sessions.Release(id)
	}
}
