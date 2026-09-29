package terminal

import (
	tea "charm.land/bubbletea/v2"
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
			s, err := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "model")
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
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "model")
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
	m := New(Dependencies{Session: &restored})
	defer m.zones.Close()
	got := m.Activity.View(70, 10)
	if !strings.Contains(got, "read: deny denied by user") {
		t.Fatalf("restored activity missing: %q", got)
	}
}
