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
	if err := next.BeginTurn(decisionNote(approve, reason, next), tools); err != nil {
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
	if approve {
		published := ""
		if run.Incident != nil {
			published = run.Incident.Published
		}
		return root.Text(fmt.Sprintf("[AXLR] The person approved the postmortem; the console wrote it to %s. Current step: %s. %s", published, run.Step, stepInstructions[run.Step]))
	}
	return root.Text(fmt.Sprintf("[AXLR] The person sent the draft back: %s. Current step: %s. %s", reason, run.Step, stepInstructions[run.Step]+incidentInstruction(run)))
}
