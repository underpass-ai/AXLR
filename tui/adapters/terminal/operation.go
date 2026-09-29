package terminal

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Operation runs against a private session snapshot. Only completion publishes it.
type Operation func(context.Context, *domain.Session, func(application.Event) error) error

type operationBatch struct{ Messages []tea.Msg }

func readOperation(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-ch
		if !ok {
			return nil
		}
		if _, done := first.(operationComplete); done {
			return first
		}
		queued := []tea.Msg{first}
		timer := time.NewTimer(16 * time.Millisecond)
		defer timer.Stop()
		for len(queued) < 256 {
			select {
			case msg, ok := <-ch:
				if !ok {
					return coalesceOperation(queued)
				}
				queued = append(queued, msg)
				if _, done := msg.(operationComplete); done {
					return coalesceOperation(queued)
				}
			case <-timer.C:
				return coalesceOperation(queued)
			}
		}
		return coalesceOperation(queued)
	}
}

func coalesceOperation(queued []tea.Msg) tea.Msg {
	result := make([]tea.Msg, 0, len(queued))
	var text strings.Builder
	textPending := false
	flush := func() {
		if textPending {
			result = append(result, application.Event{Kind: application.EventTextDelta, Text: root.Text(text.String())})
			text.Reset()
			textPending = false
		}
	}
	for _, msg := range queued {
		if event, ok := msg.(application.Event); ok && event.Kind == application.EventTextDelta {
			textPending = true
			text.WriteString(string(event.Text))
			continue
		}
		flush()
		result = append(result, msg)
	}
	flush()
	if len(result) == 1 {
		return result[0]
	}
	return operationBatch{Messages: result}
}
func (m *AppModel) BeginOperation(run Operation) tea.Cmd {
	if m.Busy {
		return nil
	}
	m.operationID++
	id := m.operationID
	var snapshot domain.Session
	if m.deps.Session != nil {
		snapshot = *m.deps.Session
	}
	m.operationMessages = len(snapshot.Messages())
	m.streamMessages = m.operationMessages
	m.streamPending = false
	lifetime := m.lifetime
	ctx, cancel := context.WithCancel(lifetime.ctx)
	m.cancel = cancel
	m.Busy = true
	m.record(application.DiagnosticEvent{Stage: application.DiagnosticOperationStarted, OperationID: id})
	trace := m.deps.Diagnostics
	ch := make(chan tea.Msg, 32)
	m.events = ch
	return func() tea.Msg {
		lifetime.mu.Lock()
		if lifetime.closed {
			lifetime.mu.Unlock()
			return operationComplete{ID: id, Session: snapshot, Err: context.Canceled}
		}
		lifetime.workers.Add(1)
		lifetime.mu.Unlock()
		go func() {
			defer lifetime.workers.Done()
			started := time.Now()
			chunks, bytes := 0, 0
			err := run(ctx, &snapshot, func(e application.Event) error {
				if e.Usage != nil {
					usage := *e.Usage
					e.Usage = &usage
				}
				select {
				case ch <- e:
					chunks++
					bytes += len(e.Text)
					if trace != nil {
						_ = trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticEventEmitted, OperationID: id, Chunks: chunks, Bytes: bytes, ElapsedMilliseconds: time.Since(started).Milliseconds()})
					}
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			stage := application.DiagnosticOperationDone
			if err != nil {
				stage = application.DiagnosticOperationFailed
			}
			if trace != nil {
				_ = trace.Record(application.DiagnosticEvent{Stage: stage, OperationID: id, Chunks: chunks, Bytes: bytes, ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: diagnosticErrorClass(err)})
			}
			select {
			case ch <- operationComplete{ID: id, Session: snapshot, Err: err}:
			case <-lifetime.ctx.Done():
			}
			close(ch)
		}()
		return <-ch
	}
}
