package terminal

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// usedSession is a session with one finished turn.
func usedSession(t *testing.T) domain.Session {
	t.Helper()
	s := navSession(t)
	if err := s.BeginTurn("earlier", nil); err != nil {
		t.Fatal(err)
	}
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "answer"}})
	return s
}

// On 12 October 2026 /new and /clear were unknown commands: a new session
// meant quitting and launching again.
func TestNewStartsAnEmptySessionWithTheSameModel(t *testing.T) {
	for _, draft := range []string{"/new", "/clear", "/Nueva"} {
		t.Run(draft, func(t *testing.T) {
			s := usedSession(t)
			old := s.Export()
			m, store := selectionModel(t, &s)
			m.Composer.Input.SetValue(draft)
			n, cmd := m.Update(ControlIntent("send"))
			if cmd == nil {
				t.Fatalf("%s started nothing: notice=%q error=%q", draft, n.(AppModel).Status.Notice, n.(AppModel).Status.Error)
			}
			m = drain(t, n.(AppModel), cmd)
			state := m.Header.State
			if state.ID == old.ID || state.Model != "model" || state.Workspace != old.Workspace || len(state.Messages) != 0 || state.Mode != "" {
				t.Fatalf("new session = %+v", state)
			}
			if m.Composer.Input.Value() != "" || m.Status.Notice != m.Theme.T("notice.newStarted") {
				t.Fatalf("draft=%q notice=%q", m.Composer.Input.Value(), m.Status.Notice)
			}
			if _, err := store.Load(context.Background(), state.ID); err != nil {
				t.Fatalf("the new session was not saved: %v", err)
			}
		})
	}
}

func TestNewSessionKeepsTheDraftAndIsRefusedWhileBusyOrPending(t *testing.T) {
	s := usedSession(t)
	oldID := s.Export().ID
	m, _ := selectionModel(t, &s)
	m.Composer.Input.SetValue("half a thought")
	m = update(m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	n, cmd := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = drain(t, n.(AppModel), cmd)
	if m.Header.State.ID == oldID || m.Composer.Input.Value() != "half a thought" || m.overlay != "" {
		t.Fatalf("palette N: id=%s draft=%q overlay=%q", m.Header.State.ID, m.Composer.Input.Value(), m.overlay)
	}
	// An empty session says so instead of starting another.
	m.Composer.Input.SetValue("/new")
	m = update(m, ControlIntent("send"))
	if m.Status.Notice != m.Theme.T("notice.newAlready") || m.Composer.Input.Value() != "/new" {
		t.Fatalf("empty session: notice=%q draft=%q", m.Status.Notice, m.Composer.Input.Value())
	}

	busy := usedSession(t)
	m, _ = selectionModel(t, &busy)
	m.Busy = true
	m = update(m, NewSessionIntent)
	if m.Status.Notice != m.Theme.T("notice.newBusy") || m.Header.State.ID != busy.Export().ID {
		t.Fatalf("busy: notice=%q", m.Status.Notice)
	}
	m.Busy = false
	m.Header.State.Activity = []domain.PendingTool{pendingCall(t, "local_exec", `{"argv":["ls"]}`)}
	m = update(m, NewSessionIntent)
	if m.Status.Notice != m.Theme.T("notice.newPending") || m.Busy {
		t.Fatalf("pending: notice=%q busy=%v", m.Status.Notice, m.Busy)
	}
}

func TestTheSessionPickerOffersANewSessionFirst(t *testing.T) {
	s := usedSession(t)
	oldID := s.Export().ID
	m, _ := selectionModel(t, &s)
	if err := m.deps.Store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	n, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = drain(t, n.(AppModel), cmd)
	view := ansi.Strip(m.View().Content)
	if m.overlay != "sessions" || !strings.Contains(view, "+ "+m.Theme.T("sessions.new")) {
		t.Fatalf("no new-session row:\n%s", view)
	}
	if strings.Index(view, m.Theme.T("sessions.new")) > strings.Index(view, "earlier") {
		t.Fatalf("the new-session row is not first:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if !m.Picker.NewSelected() {
		t.Fatal("Up from the first session did not reach the new-session row")
	}
	if _, ok := m.Picker.Current(); ok {
		t.Fatal("the new-session row reads as a session")
	}
	n, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drain(t, n.(AppModel), cmd)
	if m.Header.State.ID == oldID || m.overlay != "" {
		t.Fatalf("Enter on the new-session row: id=%s overlay=%q", m.Header.State.ID, m.overlay)
	}
	// A filter hides the row.
	p := SessionPicker{NewRow: true, OnNew: true, Query: "x"}
	if p.ShowsNew() || p.NewSelected() {
		t.Fatal("the new-session row stays while filtering")
	}
}
