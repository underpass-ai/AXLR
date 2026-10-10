package application

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// RecordPlanOutcome adds a finished plan's outcome to the session that
// planned it, as a console line for the person: the plan, its status, the
// tasks done, blocked and skipped, each wave's sync verdict, the files the
// tasks changed and what the task sessions cost. The line is never part of
// the model's history, so it starts no model request and later requests keep
// their cache prefix. On 10 October 2026 a plan synced green and its
// planning session still ended with "It's waiting for your approval on the
// plan card".
//
// The console calls it with the session it holds when the plan runner
// reports a plan of that session done, partial or blocked; the runner does
// not write the session itself, which the console owns while it is open. It
// reports false when there is nothing to add: the plan belongs to another
// session, has not finished, or the session already shows it.
func (u StartTurnUseCase) RecordPlanOutcome(ctx context.Context, session *domain.Session, plan domain.PlanRecord, emit func(Event) error) (bool, error) {
	if session == nil || u.Store == nil {
		return false, errors.New("recording a plan's outcome needs a session and a store")
	}
	if plan.Session != session.Export().ID || !domain.PlanOutcomeReported(plan.Status) {
		return false, nil
	}
	outcome := domain.NewPlanOutcome(plan)
	if u.Continue.Usage != nil {
		for _, task := range plan.Tasks {
			if task.Session == "" {
				continue
			}
			// A ledger that cannot be read leaves the cost partial, not the
			// line out.
			if ledger, err := u.Continue.Usage.LoadUsage(ctx, task.Session); err == nil && ledger.CostRequests > 0 {
				outcome.CostUSD += ledger.Cost
				outcome.CostKnown = true
			}
		}
	}
	next := *session
	added, err := next.AddPlanOutcome(outcome)
	if err != nil || !added {
		return false, err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return false, err
	}
	*session = next
	return true, emitSession(session, emit)
}

// UnreportedPlans are the plans of a session whose outcome it does not show
// yet and that ended after the person's last prompt, newest last: the plans
// that finished while the session was closed or a turn ran, when a console
// line cannot be added. A plan that ended before the person's last prompt
// was already behind them.
func UnreportedPlans(session domain.Session, plans []domain.PlanRecord) []domain.PlanRecord {
	state := session.Export()
	shown := map[string]bool{}
	for _, line := range state.ConsoleLines {
		if line.Plan != nil {
			shown[line.Plan.PlanID] = true
		}
	}
	var last time.Time
	if prompt := personRequest(state.Messages); prompt >= 0 && prompt < len(state.MessageTimes) {
		last = state.MessageTimes[prompt]
	}
	var unreported []domain.PlanRecord
	for _, plan := range plans {
		if plan.Session == state.ID && domain.PlanOutcomeReported(plan.Status) && !shown[plan.ID] && plan.Updated.After(last) {
			unreported = append(unreported, plan)
		}
	}
	sort.SliceStable(unreported, func(i, j int) bool { return unreported[i].Updated.Before(unreported[j].Updated) })
	return unreported
}
