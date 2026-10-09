package terminal

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
	"testing"
)

func TestAppModelRestoresOnlyUnacceptedSubmission(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		catalogErr, saveErr        error
		cancel, rejected, accepted bool
		newer                      string
	}{
		{name: "catalog", catalogErr: errors.New("catalog unavailable")},
		{name: "save", saveErr: errors.New("disk full")},
		{name: "cancel", cancel: true},
		{name: "rejected", rejected: true},
		{name: "newer draft", catalogErr: errors.New("catalog unavailable"), newer: "new draft"},
		{name: "accepted then continuation fails", accepted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
			if err != nil {
				t.Fatal(err)
			}
			if tc.rejected {
				if err = s.BeginTurn("old prompt", nil); err != nil {
					t.Fatal(err)
				}
			}
			store := submissionStore{err: tc.saveErr}
			start := application.StartTurnUseCase{Catalog: submissionCatalog{err: tc.catalogErr}, Store: store, Continue: application.ContinueTurnUseCase{Store: store}}
			m := update(New(Dependencies{Session: &s, Start: start, Monochrome: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
			defer m.zones.Close()
			m.Composer.Input.SetValue("recover this\noriginal prompt")
			n, cmd := m.Update(ControlIntent("send"))
			m = n.(AppModel)
			if tc.cancel {
				m = update(m, ControlIntent("cancel"))
			}
			if tc.newer != "" {
				m.Composer.Input.SetValue(tc.newer)
			}
			for cmd != nil {
				n, cmd = m.Update(cmd())
				m = n.(AppModel)
			}
			want := "recover this\noriginal prompt"
			if tc.newer != "" {
				want = tc.newer
			}
			if tc.accepted {
				want = ""
			}
			if got := m.Composer.Input.Value(); got != want {
				t.Fatalf("editor = %q; want %q", got, want)
			}
			if tc.newer != "" && !strings.Contains(m.Transcript.Viewport.GetContent(), "recover this") {
				t.Fatal("unaccepted prompt lost while newer draft preserved")
			}
			if tc.accepted && len(s.Messages()) != 1 {
				t.Fatal("accepted prompt absent from session")
			}
		})
	}
}

func TestAppModelRestoresActivityPresentation(t *testing.T) {
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BeginTurn("question", nil); err != nil {
		t.Fatal(err)
	}
	args, _ := root.NewJSONObject([]byte(`{}`))
	if err = s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call-1", Name: "read", Arguments: args}}}}); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordToolOutcome("call-1", domain.DecisionDeny, domain.ToolOutcome{Content: "denied by user"}); err != nil {
		t.Fatal(err)
	}
	restored, err := domain.RestoreSession(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: &restored, Monochrome: true}), tea.WindowSizeMsg{Width: 70, Height: 20})
	defer m.zones.Close()
	got := m.Transcript.Text()
	if !strings.Contains(got, "x read") || !strings.Contains(got, "denied") {
		t.Fatalf("restored denied call missing: %q", got)
	}
}

type submissionNotices []string

func (n submissionNotices) Drain(context.Context, domain.SessionID) ([]string, error) { return n, nil }

// StartTurn appends console notes to the stored prompt (a repair notice
// here, a ceremony step note in a ceremony): the prompt is still accepted,
// so a later failure neither refills the composer nor shows "Not sent".
func TestAppModelAcceptsAPromptStoredWithConsoleNotes(t *testing.T) {
	for _, typed := range []string{"", "typed while it ran"} {
		s := navSession(t)
		store := submissionStore{}
		start := application.StartTurnUseCase{Catalog: submissionCatalog{}, Store: store, Notices: submissionNotices{"[AXLR] Repair r-1 was merged."}, Continue: application.ContinueTurnUseCase{Store: store}}
		m := update(New(Dependencies{Session: &s, Start: start, Monochrome: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
		m.Composer.Input.SetValue("fix the login bug")
		n, cmd := m.Update(ControlIntent("send"))
		m = n.(AppModel)
		m.Composer.Input.SetValue(typed)
		m = runUIOperation(m, cmd)
		if m.Status.Error == "" {
			t.Fatal("the continuation did not fail")
		}
		if messages := s.Messages(); len(messages) != 1 || !strings.HasPrefix(string(messages[0].Content), "fix the login bug\n\n[AXLR] Repair r-1") {
			t.Fatalf("prompt not stored with its notice: %+v", messages)
		}
		if got := m.Composer.Input.Value(); got != typed {
			t.Fatalf("composer = %q; want %q (the sent prompt came back)", got, typed)
		}
		if len(m.unsentPrompts) != 0 || strings.Contains(m.Transcript.Text(), Translate(English, "transcript.notSent")) {
			t.Fatalf("an accepted prompt is shown as not sent: %v", m.unsentPrompts)
		}
		m.zones.Close()
	}
}

// A prompt the session did not take goes back to the composer, or to a "Not
// sent" row when the composer has new text; the error says which, in the
// person's language, instead of the application guessing.
func TestAFailedPromptSaysWhereItWent(t *testing.T) {
	failure := application.NotPreparedError(application.MissingDefinition{Name: "axlr_delivery", Version: "2.0"})
	for typed, where := range map[string]string{"": "error.promptReturned", "typed while it ran": "error.promptUnsent"} {
		s := navSession(t)
		store := submissionStore{}
		start := application.StartTurnUseCase{Catalog: submissionCatalog{err: failure}, Store: store, Continue: application.ContinueTurnUseCase{Store: store}}
		m := update(New(Dependencies{Session: &s, Start: start, Monochrome: true, Locale: Spanish}), tea.WindowSizeMsg{Width: 100, Height: 30})
		m.Composer.Input.SetValue("ship it")
		n, cmd := m.Update(ControlIntent("send"))
		m = n.(AppModel)
		m.Composer.Input.SetValue(typed)
		m = runUIOperation(m, cmd)
		if !strings.HasPrefix(m.Status.Error, failure.Error()) || !strings.HasSuffix(m.Status.Error, Translate(Spanish, where)) {
			t.Fatalf("composer %q: error %q does not say %s", typed, m.Status.Error, where)
		}
		if strings.Contains(m.Status.Error, "kept in the composer") {
			t.Fatalf("the error still claims the composer: %q", m.Status.Error)
		}
		m.zones.Close()
	}
}
