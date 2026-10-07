package application

import (
	"context"
	"errors"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Decide applies the person's decision on the approval card and starts the
// turn that carries the ceremony on. Whatever MADE already recorded is saved
// even when a later part fails, so pressing the key again resumes after it.
func (u StartTurnUseCase) Decide(ctx context.Context, session *domain.Session, approve bool, reason string, emit func(Event) error) error {
	if session == nil || u.Catalog == nil || u.Store == nil || u.Continue.Ceremonies == nil {
		return errors.New("the decision needs a session and a connected ceremony driver")
	}
	var result StepResult
	var err error
	if approve {
		result, err = u.Continue.Ceremonies.Approve(ctx, *session)
	} else {
		result, err = u.Continue.Ceremonies.Return(ctx, *session, reason)
	}
	return u.applyDecision(ctx, session, result, err, func(next domain.Session) root.Text { return decisionNote(approve, reason, next) }, emit)
}

// Decline applies the person's x on a plan card: the plan ends BLOCKED.
func (u StartTurnUseCase) Decline(ctx context.Context, session *domain.Session, reason string, emit func(Event) error) error {
	if session == nil || u.Catalog == nil || u.Store == nil || u.Continue.Ceremonies == nil {
		return errors.New("the decision needs a session and a connected ceremony driver")
	}
	result, err := u.Continue.Ceremonies.DeclinePlan(ctx, *session, reason)
	return u.applyDecision(ctx, session, result, err, func(domain.Session) root.Text {
		return root.Text("[AXLR] The person declined the plan: " + reason + ". The ceremony is over. Tell the user in their language.")
	}, emit)
}

func (u StartTurnUseCase) applyDecision(ctx context.Context, session *domain.Session, result StepResult, err error, note func(domain.Session) root.Text, emit func(Event) error) error {
	next := *session
	switch {
	case err == nil && result.Accepted && result.Run == nil:
		next.FinishCeremony()
	case result.Run != nil:
		if setErr := next.SetCeremony(*result.Run); setErr != nil {
			return errors.Join(err, setErr)
		}
	}
	if err != nil {
		if result.Run != nil {
			if saveErr := u.Store.Save(context.WithoutCancel(ctx), next); saveErr != nil {
				return errors.Join(err, saveErr)
			}
			*session = next
			err = errors.Join(err, emitSession(session, emit))
		}
		return err
	}
	tools, err := u.Catalog.Snapshot(ctx)
	if err != nil {
		return err
	}
	if err := next.BeginTurn(note(next), tools); err != nil {
		return err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*session = next
	return (AgentTurnUseCase{Continue: u.Continue, Tools: u.Tools, Approval: u.Approval}).Execute(ctx, session, emit)
}

// decisionNote is the visible message that hands the ceremony back to the
// model after the person decided.
func decisionNote(approve bool, reason string, s domain.Session) root.Text {
	run, live := s.Ceremony()
	if !live {
		return "[AXLR] The person decided; the ceremony is over. Tell the user the outcome."
	}
	if run.Plan != nil {
		if approve {
			return "[AXLR] The person approved the plan. Tell the user in their language; the console runs its tasks."
		}
		return root.Text(fmt.Sprintf("[AXLR] The person returned the plan: %s. Current step: %s. %s%s", reason, run.Step, stepInstructions[run.Step], planInstruction(run)))
	}
	if approve {
		published := ""
		if run.Incident != nil {
			published = run.Incident.Published
		}
		return root.Text(fmt.Sprintf("[AXLR] The person approved the postmortem; the console wrote it to %s. Current step: %s. %s", published, run.Step, stepInstructions[run.Step]))
	}
	return root.Text(fmt.Sprintf("[AXLR] The person sent the draft back: %s. Current step: %s. %s", reason, run.Step, stepInstructions[run.Step]+incidentInstruction(run)))
}
