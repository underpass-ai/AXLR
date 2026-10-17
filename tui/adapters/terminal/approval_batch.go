package terminal

import (
	"context"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// pendingCalls counts the calls waiting for a decision: the card's and the
// ones queued behind it.
func (m AppModel) pendingCalls() int {
	count := 0
	for _, p := range m.Header.State.Activity {
		if p.Outcome == nil {
			count++
		}
	}
	return count
}

// approveAll approves the card's call and every call queued behind it
// when Shift+A was pressed, in order, as A on each card would. On 12
// October 2026 one /review question raised four cards in a row. A call the
// model adds after the key press is not in the batch and gets its own card;
// a call the console refuses (an unknown tool, a mode's denial) is refused
// as it would be one by one.
func (m AppModel) approveAll() (tea.Model, tea.Cmd) {
	if m.Busy || m.Layout.TooSmall || !m.approvalFocus() || !m.knownPending() || m.pendingCalls() < 2 {
		return m, nil
	}
	if !m.installCardReady() {
		m.Status.Notice = m.Theme.T("approval.installNotReady")
		return m, nil
	}
	batch := map[root.ToolCallID]bool{}
	for _, p := range m.Header.State.Activity {
		if p.Outcome == nil {
			batch[p.Call.ID] = true
		}
	}
	resolve := m.deps.Resolve
	m.resumeTurnClock()
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
		for s.Status() == domain.StatusApproval {
			pending := s.Pending()
			if len(pending) == 0 || !batch[pending[0].Call.ID] {
				return nil
			}
			if err := resolve.Execute(ctx, s, pending[0].Call.ID, domain.DecisionApprove, emit); err != nil {
				return err
			}
		}
		return nil
	})
	return m, cmd
}
