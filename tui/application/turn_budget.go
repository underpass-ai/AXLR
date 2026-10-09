package application

import (
	"context"
	"errors"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// maxStepLimitResumes is how many times the console restarts a ceremony
// step's budget at the call limit before the person decides. On 9 October
// 2026 /improve with claude-haiku-5.5 reached the 32-call limit three times
// while exploring its brief, and each time the console-driven step waited
// for someone to press Ctrl+R.
const maxStepLimitResumes = 2

// turnLimit is the turn's tool-call budget: settings' turn_tool_calls, or
// domain.MaxTurnToolCalls.
func (u ContinueTurnUseCase) turnLimit() int {
	if u.TurnToolCalls > 0 {
		return u.TurnToolCalls
	}
	return domain.MaxTurnToolCalls
}

// warnCallBudget adds one console note when the current budget is down to
// a fifth (6 of 32 calls, 3 of a compact step's 16), so the model finishes
// or hands the step back instead of being cut mid-work. The note is added
// once per budget and never rewritten: the prompt cache keeps it.
func warnCallBudget(ctx context.Context, s *domain.Session, u ContinueTurnUseCase, emit func(Event) error) error {
	left, limit := s.CallsLeft(u.turnLimit())
	if left <= 0 || left > limit/5 || budgetNoted(s.Messages(), limit-left) {
		return nil
	}
	next := *s
	if err := next.Note(budgetWarning(next, left)); err != nil {
		return err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*s = next
	return emitSession(s, emit)
}

// budgetNoted reports whether the current budget, whose used calls are the
// last ones of the transcript, already carries a budget note.
func budgetNoted(messages []root.Message, used int) bool {
	seen := 0
	for i := len(messages) - 1; i >= 0 && seen < used; i-- {
		switch messages[i].Role {
		case root.RoleUser:
			if domain.BudgetNote(messages[i].Content) {
				return true
			}
		case root.RoleAssistant:
			seen += len(messages[i].ToolCalls)
		}
	}
	return false
}

func budgetWarning(s domain.Session, left int) root.Text {
	calls := fmt.Sprintf("%d tool calls", left)
	if left == 1 {
		calls = "1 tool call"
	}
	if run, live := s.Ceremony(); live && !run.AwaitingPerson() {
		return root.Text(fmt.Sprintf("%s %s left for this step. Finish its work and hand it back with axlr_step_done, or hand back what you have.", domain.BudgetNotePrefix, calls))
	}
	return root.Text(fmt.Sprintf("%s %s left in this turn. Finish the task with what you have, or stop and tell the user what remains; they can continue with a new budget.", domain.BudgetNotePrefix, calls))
}

// resumeStepAtLimit continues a console-driven ceremony step that paused at
// the call limit with a new budget, at most maxStepLimitResumes times per
// step, and tells the model so; outside a ceremony the person decides.
func resumeStepAtLimit(ctx context.Context, s *domain.Session, u ContinueTurnUseCase, emit func(Event) error) (bool, error) {
	next := *s
	resumes, ok, err := next.ResumeStepAtLimit(maxStepLimitResumes)
	if err != nil || !ok {
		return false, err
	}
	_, limit := next.CallsLeft(u.turnLimit())
	note := root.Text(fmt.Sprintf("%s This step reached its %d tool-call limit; the console continued it with a new budget (%d of %d). Finish the step's work and hand it back with axlr_step_done.", domain.BudgetNotePrefix, limit, resumes, maxStepLimitResumes))
	if err := next.Note(note); err != nil {
		return false, err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return false, err
	}
	*s = next
	return true, emitSession(s, emit)
}

// atCallLimit reports the pause CompleteAssistantWithin makes at the limit.
func atCallLimit(err error) bool { return errors.Is(err, domain.ErrToolCallLimit) }
