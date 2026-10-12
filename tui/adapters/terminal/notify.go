package terminal

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
)

// Notifications. On 12 October 2026 a long plan or an approval card in
// another tmux window went unnoticed: the console rang no bell, sent no
// desktop notification and left the window's title alone. It now rings
// when a turn that took notifyAfter or more ends and whenever an approval
// card or a plan, postmortem or repair card appears, as the notify setting
// says, and keeps the terminal's title at "AXLR · <state> · <session>",
// restoring the one it found on exit.

// Notify settings.
const (
	NotifyBell = "bell"
	NotifyOSC  = "osc"
	NotifyOff  = "off"
)

// notifyAfter is how long a turn runs before its end rings.
const notifyAfter = 10 * time.Second

// Terminal title stack (xterm's, honoured by most terminals and tmux): the
// console saves the title it found and puts it back when it quits, after
// clearing its own for terminals without the stack.
const (
	titlePush    = "\x1b[22;0t"
	titleRestore = "\x1b]2;\x07\x1b[23;0t"
)

// titleReadyMsg follows the title's save: from then on the console sets it.
type titleReadyMsg struct{}

// startTitle saves the terminal's title before the console sets its own.
func startTitle() tea.Cmd {
	return tea.Sequence(tea.Raw(titlePush), func() tea.Msg { return titleReadyMsg{} })
}

// ring is the notification the setting asks for: a bell, an OSC 9 desktop
// notification with the bell, or nothing.
func (m AppModel) ring(text string) tea.Cmd {
	switch m.deps.Notify {
	case NotifyOff:
		return nil
	case NotifyOSC:
		return tea.Raw(ansi.Notify("AXLR: "+singleLine(text)) + "\a")
	}
	return tea.Raw("\a")
}

// attentionKey names what waits for the person, "" for nothing: the call
// an approval card is about, or the card of a ceremony awaiting them.
func (m AppModel) attentionKey() string {
	if m.approvalFocus() {
		if p, ok := m.pending(); ok {
			return "call:" + string(p.Call.ID)
		}
	}
	if m.overlay != "incident" {
		return ""
	}
	if run, awaiting := m.incidentRun(); awaiting {
		return fmt.Sprintf("card:%s:%s:%d", run.Instance, run.Step, run.Iteration)
	}
	return ""
}

// windowTitle is "AXLR · <state> · <session>".
func (m AppModel) windowTitle() string {
	state := m.Theme.T("title.idle")
	switch {
	case m.attentionKey() != "":
		state = m.Theme.T("title.waiting")
	case m.Busy && m.turnRunning():
		state = m.Theme.T("title.working")
	case m.Status.Error != "":
		state = m.Theme.T("title.failed")
	}
	title := "AXLR · " + state
	if session := m.sessionName(); session != "" {
		title += " · " + session
	}
	return ansi.Truncate(singleLine(title), 80, "…")
}

// sessionName is the session's title, or its first prompt's first line.
func (m AppModel) sessionName() string {
	if m.titleLabel != "" {
		return m.titleLabel
	}
	for _, message := range m.Header.State.Messages {
		if message.Role == root.RoleUser && !consoleNote(message.Content) {
			first, _, _ := strings.Cut(strings.TrimSpace(string(message.Content)), "\n")
			return ansi.Truncate(singleLine(first), 48, "…")
		}
	}
	return ""
}

// loadTitleLabel reads the session's title from its labels, which a turn
// may set.
func (m *AppModel) loadTitleLabel() {
	m.titleLabel = ""
	if m.deps.SessionLabels == nil || m.Header.State.ID == "" {
		return
	}
	if labels, err := m.deps.SessionLabels.Load(m.lifetime.ctx); err == nil {
		m.titleLabel = singleLine(string(labels[m.Header.State.ID].Title))
	}
}

// signal adds what the step that just ran asks of the terminal: a ring for
// a card that appeared or a long turn that ended, and the title when it
// changed.
func (m AppModel) signal(cmd tea.Cmd) (AppModel, tea.Cmd) {
	if m.quitting {
		return m, cmd
	}
	cmds := []tea.Cmd{cmd}
	if key := m.attentionKey(); key != m.lastAttention {
		m.lastAttention = key
		if key != "" {
			cmds = append(cmds, m.ring(m.Theme.T("title.waiting")))
		}
	}
	if m.turnDone {
		m.turnDone = false
		cmds = append(cmds, m.ring(m.Theme.T("notify.turnDone")))
	}
	if m.titleReady {
		if title := m.windowTitle(); title != m.lastTitle {
			m.lastTitle = title
			cmds = append(cmds, tea.Raw(ansi.SetWindowTitle(title)))
		}
	}
	return m, tea.Batch(cmds...)
}
