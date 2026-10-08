package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSteeringQueuesWithoutCancellingAndStartsNewUserTurn(t *testing.T) {
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
	m.Status.Phase = domain.ProviderReasoning
	m.operationID = 1
	cancelled := false
	m.cancel = func() { cancelled = true }
	m.Composer.Input.SetValue("new direction")
	m = update(m, ControlIntent("send"))
	if cancelled || m.steer.Peek() != "new direction" || m.Composer.Input.Value() != "" {
		t.Fatal("steer was not queued or the reasoning stream was cancelled")
	}
	if view := m.Transcript.View(); !strings.Contains(view, "new direction") || !strings.Contains(view, m.Theme.T("transcript.queued")) {
		t.Fatalf("queued message is not visible: %q", view)
	}
	if footer := m.statusActivity(StatusBar{Waiting: true, Phase: domain.ProviderReasoning}); !strings.Contains(footer, m.Theme.T("status.queued")) {
		t.Fatalf("footer does not show the queue: %q", footer)
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
	if strings.Contains(m.Transcript.View(), m.Theme.T("transcript.queued")) {
		t.Fatal("delivered message still shown as queued")
	}
}

func TestQueuedSteerLeavesTheTurnToTakeIt(t *testing.T) {
	m := sized()
	m.Busy = true
	m.toolExecuting = true
	cancelled := false
	m.cancel = func() { cancelled = true }
	m.steer.Add("after the tool")
	m = update(m, application.Event{Kind: application.EventStreamStart})
	if cancelled {
		t.Fatal("queued steer cancelled a turn that takes it after its tool step")
	}
}

func TestQueuedSteerStopsACeremonyStepWithoutReportingAnError(t *testing.T) {
	m := sized()
	m.Busy = true
	m.operationID = 4
	m.toolExecuting = true
	m.Header.State.Ceremony = &domain.CeremonyRun{Definition: "axlr_incident", Step: "present"}
	cancelled := false
	m.cancel = func() { cancelled = true }
	m.steer.Add("after the tool")
	m = update(m, application.Event{Kind: application.EventStreamStart})
	if !cancelled {
		t.Fatal("queued steer did not stop the ceremony step")
	}
	m.steer.Take()
	m = update(m, operationComplete{ID: 4, Session: *m.deps.Session, Err: errors.Join(context.Canceled, context.Canceled)})
	if m.Status.Error != "" {
		t.Fatalf("stopping for a queued message was reported as an error: %q", m.Status.Error)
	}
}

func TestSendDuringUnrelatedBusyOperationKeepsDraft(t *testing.T) {
	m := sized()
	m.Busy = true
	m.Composer.Input.SetValue("keep this message")
	m = update(m, ControlIntent("send"))
	if m.Composer.Input.Value() != "keep this message" || m.steer.Peek() != "" {
		t.Fatal("unrelated operation consumed user input as a steer")
	}
}
