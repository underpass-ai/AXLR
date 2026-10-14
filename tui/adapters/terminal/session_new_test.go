package terminal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
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

// On 12 October 2026 Ctrl+O could only archive. Ctrl+Del deletes the
// selected session after a y, and never the open one.
func TestCtrlDeleteDeletesASessionAfterConfirmation(t *testing.T) {
	open := usedSession(t)
	m, store := selectionModel(t, &open)
	m.deps.DeleteSession = store
	other, err := domain.NewSession("fedcba9876543210fedcba9876543210", testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.BeginTurn("the other one", nil); err != nil {
		t.Fatal(err)
	}
	for _, s := range []domain.Session{open, other} {
		if err := store.Save(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	n, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = drain(t, n.(AppModel), cmd)
	for m.Picker.Title(must1(m.Picker.Current())) != "the other one" {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	ctrlDel := tea.KeyPressMsg{Code: tea.KeyDelete, Mod: tea.ModCtrl}
	m = update(m, ctrlDel)
	if !m.Picker.Deleting || !strings.Contains(ansi.Strip(m.View().Content), "the other one") {
		t.Fatal("Ctrl+Del did not ask to confirm")
	}
	m = update(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.Picker.Deleting || len(m.Picker.Visible()) != 2 {
		t.Fatal("n did not keep the session")
	}
	// Enter is "any other key": it keeps the session and opens nothing.
	m = update(m, ctrlDel)
	n, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = n.(AppModel)
	if cmd != nil || m.Busy || m.Picker.Deleting || m.overlay != "sessions" || len(m.Picker.Visible()) != 2 {
		t.Fatalf("Enter while confirming: busy=%v deleting=%v overlay=%q", m.Busy, m.Picker.Deleting, m.overlay)
	}
	m = update(m, ctrlDel)
	m = update(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if len(m.Picker.Visible()) != 1 || m.Status.Notice != m.Theme.Tf("sessions.deleted", "the other one") {
		t.Fatalf("y did not delete: visible=%d notice=%q error=%q", len(m.Picker.Visible()), m.Status.Notice, m.Status.Error)
	}
	if list, _ := store.List(context.Background()); len(list) != 1 {
		t.Fatalf("%d sessions saved after the delete", len(list))
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyHome})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, ctrlDel)
	m = update(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.Status.Error != m.Theme.T("sessions.deleteOpen") || len(m.Picker.Visible()) != 1 {
		t.Fatalf("the open session: error=%q", m.Status.Error)
	}
}

func must1[T any](v T, _ bool) T { return v }

// On 12 October 2026 /copy, the last reply, was the only way to take a
// conversation out of the console.
func TestExportWritesTheSessionAsMarkdown(t *testing.T) {
	s := usedSession(t)
	m, _ := selectionModel(t, &s)
	files := map[string]string{}
	m.deps.ExportFiles = exportRecorder(files)
	m.Composer.Input.SetValue("/export")
	m = update(m, ControlIntent("send"))
	if len(files) != 1 || m.Composer.Input.Value() != "" || m.Status.Error != "" {
		t.Fatalf("/export: files=%d draft=%q error=%q", len(files), m.Composer.Input.Value(), m.Status.Error)
	}
	for path, text := range files {
		if !strings.Contains(path, ".axlr") || !strings.Contains(text, "earlier") || m.Status.Notice != m.Theme.Tf("export.done", m.workspacePath(path)) {
			t.Fatalf("default export %s: notice=%q\n%s", path, m.Status.Notice, text)
		}
	}
	m.Composer.Input.SetValue("/export notes/out.md")
	m = update(m, ControlIntent("send"))
	want := filepath.Join(string(testWorkspace()), "notes", "out.md")
	if _, ok := files[want]; !ok || m.Busy || len(m.Header.State.Messages) != 2 {
		t.Fatalf("/export notes/out.md wrote %v", files)
	}
	empty := navSession(t)
	m, _ = selectionModel(t, &empty)
	m.deps.ExportFiles = exportRecorder(files)
	m.Composer.Input.SetValue("/export")
	m = update(m, ControlIntent("send"))
	if !strings.Contains(m.Status.Error, "nothing to export") || m.Composer.Input.Value() != "/export" {
		t.Fatalf("empty session: error=%q", m.Status.Error)
	}
}

// On 14 October 2026 a failed /export read "…/s.md already exists; export
// to another path" and "mkdir …/notas/s.md: not a directory" in English in
// a Spanish console: it is said in the person's language, with the path
// they typed or the file in the way.
func TestExportFailuresAreSaidInThePersonsLanguage(t *testing.T) {
	s := usedSession(t)
	m, _ := selectionModel(t, &s)
	m.deps.ExportFiles = storage.ExportFiles{}
	m.Theme.Locale = Spanish
	existing := filepath.Join(t.TempDir(), "s.md")
	if err := os.WriteFile(existing, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Composer.Input.SetValue("/export " + existing)
	m = update(m, ControlIntent("send"))
	if m.Status.Error != "No se ha exportado: "+existing+" ya existe; indica otra ruta" {
		t.Fatalf("collision: %q", m.Status.Error)
	}
	m.Composer.Input.SetValue("/export " + filepath.Join(existing, "x"))
	m = update(m, ControlIntent("send"))
	if m.Status.Error != "No se ha exportado: "+shortenHome(existing)+" es un archivo, no una carpeta; indica otra ruta" {
		t.Fatalf("file in the way: %q", m.Status.Error)
	}
}

type exportRecorder map[string]string

func (e exportRecorder) Create(_ context.Context, path string, data []byte) error {
	e[path] = string(data)
	return nil
}

// On 14 October 2026 a console that switched sessions with Ctrl+O still
// held the old session's lock (both .lock files in /proc/<pid>/fd), and
// --continue in a second console called it "open in another console".
// Opening another session and /new give the left session's lock back.
func TestSwitchingSessionsReleasesTheLeftSessionsLock(t *testing.T) {
	dir := t.TempDir()
	open := func() *storage.SessionStore {
		store, err := storage.New(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close() })
		return store
	}
	console, other := open(), open()
	a, b := usedSession(t), usedSession(t)
	bState := b.Export()
	bState.ID = "fedcba9876543210fedcba9876543210"
	b, err := domain.RestoreSession(bState)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []domain.Session{a, b} {
		if err := console.Save(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	// The console writes the session it opens into its Session.
	aID, bID := a.Export().ID, b.Export().ID
	m := update(New(Dependencies{Session: &a, Workspace: testWorkspace(), NewSessionID: "0123456789abcdef0123456789abcde0", Store: console, Sessions: console, Create: application.CreateSessionUseCase{Store: console}, Monochrome: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Cleanup(m.Close)
	if _, err := other.Load(context.Background(), aID); err == nil {
		t.Fatal("the open session's lock is not held")
	}
	n, cmd := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), cmd)
	for i, row := range m.Picker.Visible() {
		if row.ID == bID {
			m.Picker.Selected = i
		}
	}
	n, cmd = m.Update(ControlIntent("open-session"))
	m = drain(t, n.(AppModel), cmd)
	if m.Header.State.ID != bID {
		t.Fatalf("the other session did not open: %s", m.Status.Error)
	}
	if _, err := other.Load(context.Background(), aID); err != nil {
		t.Fatalf("the left session is still held: %v", err)
	}
	if _, err := other.Load(context.Background(), bID); err == nil {
		t.Fatal("the opened session's lock is not held")
	}
	if err := other.Release(aID); err != nil {
		t.Fatal(err)
	}
	// /new leaves b: its lock goes back too, and the new session's is held.
	n, cmd = m.Update(NewSessionIntent)
	m = drain(t, n.(AppModel), cmd)
	if m.Header.State.ID == bID {
		t.Fatalf("no new session: %s %s", m.Status.Notice, m.Status.Error)
	}
	if _, err := other.Load(context.Background(), bID); err != nil {
		t.Fatalf("the session left by /new is still held: %v", err)
	}
	if _, err := other.Load(context.Background(), m.Header.State.ID); err == nil {
		t.Fatal("the new session's lock is not held")
	}
}
