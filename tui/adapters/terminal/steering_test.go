package terminal

import (
	"context"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSteeringInterruptsActiveStreamAndStartsNewUserTurn(t *testing.T) {
	s := navSession(t)
	if err := s.BeginTurn("original", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft("partial response"); err != nil {
		t.Fatal(err)
	}
	m := navModel(t, &s)
	continuation := application.ContinueTurnUseCase{Store: m.deps.Store, Models: navStream{}}
	m.deps.Start = application.StartTurnUseCase{Catalog: submissionCatalog{}, Store: m.deps.Store, Continue: continuation}
	m.Busy = true
	m.providerWaiting = true
	m.operationID = 1
	cancelled := false
	m.cancel = func() { cancelled = true }
	m.Composer.Input.SetValue("new direction")
	m = update(m, ControlIntent("send"))
	if !cancelled || m.steerPrompt != "new direction" || m.Composer.Input.Value() != "" {
		t.Fatal("steer was not queued and old stream interrupted")
	}
	next, cmd := m.Update(operationComplete{ID: 1, Session: s, Err: context.Canceled})
	m = drain(t, next.(AppModel), cmd)
	messages := m.deps.Session.Messages()
	if len(messages) != 3 || messages[0].Content != "original" || messages[1].Role != root.RoleUser || messages[1].Content != "new direction" || messages[2].Role != root.RoleAssistant {
		t.Fatalf("steer was not delivered as user input: %+v", messages)
	}
	if m.deps.Session.Status() != domain.StatusComplete {
		t.Fatalf("steered turn status = %s", m.deps.Session.Status())
	}
}

func TestSendDuringUnrelatedBusyOperationKeepsDraft(t *testing.T) {
	m := sized()
	m.Busy = true
	m.Composer.Input.SetValue("keep this message")
	m = update(m, ControlIntent("send"))
	if m.Composer.Input.Value() != "keep this message" || m.steerPrompt != "" {
		t.Fatal("unrelated operation consumed user input as a steer")
	}
}
