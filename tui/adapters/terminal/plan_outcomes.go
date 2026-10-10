package terminal

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// planOutcomesMsg asks the console to record the plan outcomes it holds,
// once the program runs.
type planOutcomesMsg struct{}

// queuePlanOutcomes keeps the plans of the open session that ended done,
// partial or blocked, so their outcome becomes a console line of the session
// that planned them. On 10 October 2026 a plan synced green and its planning
// session still ended with the planner's "It's waiting for your approval on
// the plan card".
func (m AppModel) queuePlanOutcomes(plans ...domain.PlanRecord) AppModel {
	for _, plan := range plans {
		if plan.Session == "" || plan.Session != m.Header.State.ID || !domain.PlanOutcomeReported(plan.Status) {
			continue
		}
		queued := false
		for i := range m.pendingPlanOutcomes {
			if m.pendingPlanOutcomes[i].ID == plan.ID {
				m.pendingPlanOutcomes[i], queued = plan, true
			}
		}
		if !queued {
			m.pendingPlanOutcomes = append(m.pendingPlanOutcomes, plan)
		}
	}
	return m
}

// queueUnreportedPlans queues the plans of the session just opened that
// ended while it was closed or a turn ran.
func (m AppModel) queueUnreportedPlans() AppModel {
	if m.deps.Session == nil || m.Header.State.ID == "" {
		return m
	}
	return m.queuePlanOutcomes(application.UnreportedPlans(*m.deps.Session, m.PlanPanel.Records)...)
}

// recordPlanOutcomes adds the queued outcomes to the session between turns,
// as console lines for the person: they are never sent to the model and
// start no model request. During a turn they wait for it to end.
func (m *AppModel) recordPlanOutcomes() tea.Cmd {
	if len(m.pendingPlanOutcomes) == 0 || m.Busy || m.deps.Session == nil || m.deps.Start.Store == nil {
		return nil
	}
	switch m.deps.Session.Status() {
	case domain.StatusIdle, domain.StatusComplete, domain.StatusInterrupted:
	default:
		return nil
	}
	if len(m.deps.Session.Pending()) != 0 {
		return nil
	}
	plans := m.pendingPlanOutcomes
	m.pendingPlanOutcomes = nil
	start := m.deps.Start
	return m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
		var errs []error
		for _, plan := range plans {
			if _, err := start.RecordPlanOutcome(ctx, s, plan, emit); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	})
}
