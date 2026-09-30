package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/application"
)

func TestWaitingIndicatorUpdatesWithoutProviderTextAndKeepsEscape(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.cancel = cancel
	next, cmd := m.Update(application.Event{Kind: application.EventStreamStart})
	m = next.(AppModel)
	if cmd == nil || !m.waitTickScheduled || !strings.Contains(m.View().Content, "Waiting for model · 0s") {
		t.Fatal("waiting has no initial indication or timer")
	}
	m.providerWaitStarted = time.Now().Add(-3 * time.Second)
	next, cmd = m.Update(providerWaitTick{OperationID: 1})
	m = next.(AppModel)
	if cmd == nil || !strings.Contains(m.View().Content, "Waiting for model · 3s") {
		t.Fatal("waiting display cannot progress without provider events")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if ctx.Err() != context.Canceled {
		t.Fatal("waiting prevented cancellation")
	}
}
func TestWaitingTickDoesNotRestartWhenIdleOrStale(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	next, cmd := m.Update(providerWaitTick{OperationID: 0})
	m = next.(AppModel)
	if cmd != nil || m.waitTickScheduled {
		t.Fatal("idle UI is ticking")
	}
	m.Busy = true
	m.operationID = 2
	m.providerWaiting = true
	m.waitTickScheduled = true
	next, cmd = m.Update(providerWaitTick{OperationID: 1})
	m = next.(AppModel)
	if cmd != nil || !m.waitTickScheduled {
		t.Fatal("old turn changed current timer")
	}
}
func TestWaitingStopsOnTextSnapshotToolAndCompletion(t *testing.T) {
	for _, kind := range []application.EventKind{application.EventTextDelta, application.EventSession, application.EventToolExecutionStarted} {
		t.Run(string(kind), func(t *testing.T) {
			m := sized()
			defer m.zones.Close()
			m.Busy = true
			m.operationID = 1
			m.providerWaiting = true
			m.waitTickScheduled = true
			event := application.Event{Kind: kind, Text: "answer"}
			if kind == application.EventSession {
				state := m.Header.State
				event.Snapshot = &state
			}
			m = update(m, event)
			next, cmd := m.Update(providerWaitTick{OperationID: 1})
			m = next.(AppModel)
			if m.providerWaiting || strings.Contains(m.View().Content, "Waiting for model") {
				t.Fatal("waiting continued after provider finished waiting")
			}
			if kind == application.EventToolExecutionStarted && (cmd == nil || !m.toolExecuting) {
				t.Fatal("tool execution has no activity timer")
			}
			if kind != application.EventToolExecutionStarted && cmd != nil {
				t.Fatal("idle state kept an activity timer")
			}
		})
	}
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	m.providerWaiting = true
	m.waitTickScheduled = true
	m = update(m, operationComplete{ID: 1, Session: *m.deps.Session})
	if m.providerWaiting || m.waitTickScheduled {
		t.Fatal("completion kept timer active")
	}
}
func TestWaitingStartsFromCoalescedEventsAndAvoidsUnneededTimer(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	next, cmd := m.Update(operationBatch{Messages: []tea.Msg{application.Event{Kind: application.EventStreamStart}, application.Event{Kind: application.EventState}}})
	m = next.(AppModel)
	if cmd == nil || !m.waitTickScheduled {
		t.Fatal("coalescing discarded waiting timer")
	}
	m.waitTickScheduled = false
	m.providerWaiting = false
	next, cmd = m.Update(operationBatch{Messages: []tea.Msg{application.Event{Kind: application.EventStreamStart}, application.Event{Kind: application.EventTextDelta, Text: "immediate"}}})
	m = next.(AppModel)
	if cmd != nil || m.waitTickScheduled {
		t.Fatal("immediate provider scheduled an unnecessary timer")
	}
}

func TestSpinnerRestartsAfterIdleTickInSameOperation(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	m = update(m, application.Event{Kind: application.EventStreamStart})
	if !m.spinnerScheduled {
		t.Fatal("waiting did not schedule animation")
	}
	m = update(m, application.Event{Kind: application.EventTextDelta, Text: "answer"})
	m = update(m, providerAnimationTick{OperationID: 1, Tick: m.activitySpinner.Tick()})
	if m.spinnerScheduled {
		t.Fatal("idle animation tick left the spinner scheduled")
	}
	m = update(m, application.Event{Kind: application.EventStreamStart})
	if !m.spinnerScheduled {
		t.Fatal("new waiting phase did not restart animation")
	}
	before := m.activitySpinner.View()
	m = update(m, providerAnimationTick{OperationID: 1, Tick: m.activitySpinner.Tick()})
	if m.activitySpinner.View() == before {
		t.Fatal("restarted spinner did not advance")
	}
}

func TestSpinnerTickRunsWhileProviderReadIsBlocked(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	events := make(chan tea.Msg)
	m.events = events
	next, cmd := m.Update(application.Event{Kind: application.EventStreamStart})
	m = next.(AppModel)
	if cmd == nil {
		t.Fatal("waiting has no commands")
	}
	batched := cmd()
	batch, ok := batched.(tea.BatchMsg)
	if !ok || len(batch) < 3 {
		t.Fatalf("want provider read and independent timers, got %T", batched)
	}
	readResult := make(chan tea.Msg, 1)
	go func() { readResult <- batch[0]() }()
	defer func() {
		close(events)
		<-readResult
	}()
	animation := batch[len(batch)-1]
	result := make(chan tea.Msg, 1)
	go func() { result <- animation() }()
	select {
	case msg := <-result:
		if _, ok := msg.(providerAnimationTick); !ok {
			t.Fatalf("animation command returned %T", msg)
		}
		before := m.activitySpinner.View()
		m = update(m, msg)
		if m.activitySpinner.View() == before {
			t.Fatal("spinner did not advance while provider read was blocked")
		}
	case <-time.After(time.Second):
		t.Fatal("spinner waited for the provider read")
	}
}

func TestStatusSpinnerHasSpaceBeforeText(t *testing.T) {
	for _, status := range []StatusBar{
		{Indicator: "◐", Waiting: true},
		{Indicator: "◐", Executing: true, ToolName: "kmp_wake"},
	} {
		view := ansi.Strip(status.View(100))
		if strings.Contains(view, "◐Waiting") || strings.Contains(view, "◐Executing") {
			t.Fatalf("spinner touches status text: %q", view)
		}
		if !strings.Contains(view, "◐  ") {
			t.Fatalf("spinner needs two cells of separation: %q", view)
		}
	}
}
