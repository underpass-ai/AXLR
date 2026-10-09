package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	if err := next.Note(budgetWarning(next, left, limit)); err != nil {
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

// budgetWarning says how many calls are left and that the count is this
// budget's: on 9 October 2026 claude-haiku-5.5, asked to go on after "1
// tool call left in this turn", stopped after one call of the new turn,
// taking the old note for its budget.
func budgetWarning(s domain.Session, left, limit int) root.Text {
	calls := fmt.Sprintf("%d tool calls", left)
	if left == 1 {
		calls = "1 tool call"
	}
	if run, live := s.Ceremony(); live && !run.AwaitingPerson() {
		return root.Text(fmt.Sprintf("%s %s left of this step's %d. Finish its work and hand it back with axlr_step_done, or hand back what you have; the next step starts a new budget.", domain.BudgetNotePrefix, calls, limit))
	}
	return root.Text(fmt.Sprintf("%s %s left of this turn's %d. Finish the task with what you have, or stop and tell the user what remains; the user's next message starts a new budget of %d.", domain.BudgetNotePrefix, calls, limit, limit))
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

// callBudgetGuidance tells the model the turn's budget before it plans. On
// 10 October 2026 claude-haiku-5.5, told of the budget only when a fifth was
// left, sent 93 calls in one answer under a budget of 32. The number is the
// console's setting, fixed for its lifetime, so the system prompt stays the
// same from one request to the next.
func callBudgetGuidance(limit int) string {
	return fmt.Sprintf("Each turn allows at most %d tool calls, counted from the user's message (a ceremony step may allow fewer). Calls of an answer past the budget do not run and the turn pauses until a new budget starts, so plan large jobs within it.\n", limit)
}

// pausesOverBudget reports a turn whose last answer ran out of budget part
// way: its last result is an over-budget refusal and no call is left.
func pausesOverBudget(s domain.Session, turnLimit int) bool {
	messages := s.Messages()
	if len(messages) == 0 {
		return false
	}
	last := messages[len(messages)-1]
	left, _ := s.CallsLeft(turnLimit)
	return left <= 0 && last.Role == root.RoleTool && strings.HasPrefix(string(last.Content), domain.OverBudgetOutcomePrefix)
}

// atCallLimit reports the pause CompleteAssistantWithin makes at the limit.
func atCallLimit(err error) bool { return errors.Is(err, domain.ErrToolCallLimit) }
