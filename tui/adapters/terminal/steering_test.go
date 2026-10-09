package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

// A message queued during a turn that fails starts the next turn at once:
// the failure stays in the conversation instead of being cleared before it
// is ever drawn, and the queued message is still delivered.
func TestAFailureBeforeAQueuedMessageStaysInTheConversation(t *testing.T) {
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
	m.cancel = func() {}
	m.Composer.Input.SetValue("new direction")
	m = update(m, ControlIntent("send"))
	next, cmd := m.Update(operationComplete{ID: 1, Session: s, Err: errors.New("provider returned 503")})
	m = next.(AppModel)
	if !strings.Contains(m.Transcript.Text(), "provider returned 503") {
		t.Fatalf("the failure was cleared before it was drawn: error %q", m.Status.Error)
	}
	m = drain(t, m, cmd)
	if messages := m.deps.Session.Messages(); len(messages) != 3 || messages[1].Content != "new direction" {
		t.Fatalf("the queued message was not delivered: %+v", messages)
	}
	if !strings.Contains(m.Transcript.Text(), "provider returned 503") {
		t.Fatal("the failure note did not outlast the queued turn")
	}
	m.Composer.Input.SetValue("next question")
	if m = update(m, ControlIntent("send")); strings.Contains(m.Transcript.Text(), "provider returned 503") {
		t.Fatal("the failure note stayed after the person's next message")
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

// An operation that is not a turn (/update with its panel closed, MADE
// preparation, a catalog or session list) shows that it runs, and Enter
// says why nothing was sent instead of doing nothing.
func TestABackgroundOperationShowsItRunsAndAnswersEnter(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	if footer := ansi.Strip(m.footerView()); strings.Contains(footer, m.Theme.T("status.idle")) || !strings.Contains(footer, m.Theme.T("status.working")) {
		t.Fatalf("the footer does not show the running operation: %q", footer)
	}
	m.Composer.Input.SetValue("hello")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Composer.Input.Value() != "hello" || m.steer.Peek() != "" || m.Status.Notice != m.Theme.T("notice.operationRunning") {
		t.Fatalf("Enter during the operation: draft %q, notice %q", m.Composer.Input.Value(), m.Status.Notice)
	}
	if footer := ansi.Strip(m.footerView()); !strings.Contains(footer, m.Theme.T("notice.operationRunning")) {
		t.Fatalf("the notice is not shown: %q", footer)
	}
}

// Ctrl+R is refused while an operation runs, so the footer offers it only
// once the console is free.
func TestTheFooterOffersContinueOnlyWhenNothingRuns(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Header.State.Status = domain.StatusStreaming
	m.Busy = true
	if footer := ansi.Strip(m.footerView()); strings.Contains(footer, "ctrl+r") {
		t.Fatalf("continue offered while busy: %q", footer)
	}
	m.Busy = false
	if footer := ansi.Strip(m.footerView()); !strings.Contains(footer, "ctrl+r continue") {
		t.Fatalf("continue not offered for a stopped stream: %q", footer)
	}
}
