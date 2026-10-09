package domain

import (
	"errors"
	"fmt"
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// BudgetNotePrefix opens the console's notes about the call budget: the
// warning before the limit and the note of a step continued at it.
const BudgetNotePrefix = "[AXLR · budget]"

// BudgetNote reports whether a message is one of those notes.
func BudgetNote(content axlr.Text) bool {
	return strings.HasPrefix(string(content), BudgetNotePrefix)
}

// OverBudgetOutcomePrefix opens the result of a call the console did not run
// because the turn's tool-call budget (or a ceremony step's) was spent: the
// person denied nothing, so the transcript labels these "over budget".
const OverBudgetOutcomePrefix = "not run: over the turn's tool-call budget"

// overBudget reports a result RefuseOverBudget or the whole-answer refusal
// recorded: the turn's count leaves such calls out.
func overBudget(decision ToolDecision, outcome ToolOutcome) bool {
	return decision == DecisionDeny && outcome.IsError && strings.HasPrefix(string(outcome.Content), OverBudgetOutcomePrefix)
}

// overBudgetOutcome tells the model how many calls of its answer ran and
// what to do with the rest; ran is 0 when the budget was spent before it.
func overBudgetOutcome(ran, total, limit int) axlr.Text {
	ranText := fmt.Sprintf("%d of this answer's %d calls ran, in order; this one did not", ran, total)
	if ran == 0 {
		ranText = fmt.Sprintf("none of this answer's %d calls ran", total)
	}
	return axlr.Text(fmt.Sprintf("%s of %d calls: %s. The turn pauses until a new budget starts (the person's next message or Ctrl+R; a console-driven step continues by itself); then repeat the calls that did not run, at most %d per answer.", OverBudgetOutcomePrefix, limit, ranText, limit))
}

// HeadOverBudget reports whether the first pending call lies past the
// current budget: the calls already answered (TurnCallCount counts the
// pending ones too) have used it up.
func (s Session) HeadOverBudget(turnLimit int) bool {
	pending := len(s.Pending())
	if s.Status() != StatusApproval || pending == 0 {
		return false
	}
	base, limit := s.callBudget(turnLimit)
	return s.state.TurnCallCount-pending-base >= limit
}

// RefuseOverBudget answers the first pending call as not run when it lies
// past the budget, and reports whether it did. Results keep call order, so
// the overflow of an answer is refused one call at a time, after the calls
// before it.
func (s *Session) RefuseOverBudget(turnLimit int) (bool, error) {
	if !s.HeadOverBudget(turnLimit) {
		return false, nil
	}
	head := s.Pending()[0].Call.ID
	total, ran := 0, 0
	for i := len(s.state.Messages) - 1; i >= 0; i-- {
		if message := s.state.Messages[i]; message.Role == axlr.RoleAssistant && len(message.ToolCalls) > 0 {
			total = len(message.ToolCalls)
			for _, call := range message.ToolCalls {
				if call.ID == head {
					break
				}
				for _, record := range s.state.Activity {
					if record.Call.ID == call.ID && record.Outcome != nil && !overBudget(record.Decision, *record.Outcome) {
						ran++
					}
				}
			}
			break
		}
	}
	_, limit := s.callBudget(turnLimit)
	if err := s.RecordToolOutcome(head, DecisionDeny, ToolOutcome{Content: overBudgetOutcome(ran, total, limit), IsError: true}); err != nil {
		return false, err
	}
	return true, nil
}

// CompleteAssistant records the model's answer under the default per-turn
// budget, MaxTurnToolCalls.
func (s *Session) CompleteAssistant(result axlr.CompletionResult) error {
	return s.CompleteAssistantWithin(result, MaxTurnToolCalls)
}

// callBudget is where the current tool-call budget starts in the turn's
// count and how many calls it allows: the turn's, or a ceremony step's,
// which starts when the step was accepted.
func (s Session) callBudget(turnLimit int) (base, limit int) {
	base, limit = s.state.FinishedBudgetBase, turnLimit
	if s.state.Ceremony != nil {
		base = max(base, s.state.Ceremony.BudgetBase)
		limit = s.state.Ceremony.StepCallLimit(turnLimit)
	}
	return base, limit
}

// CallsLeft is how many tool calls the current budget still allows, and
// how many it allows in all, under the turn's limit.
func (s Session) CallsLeft(turnLimit int) (left, limit int) {
	base, limit := s.callBudget(turnLimit)
	return limit - (s.state.TurnCallCount - base), limit
}

// Note adds a console message to the running turn after its tool results.
// Unlike Steer it is not the person's word: the call budget goes on. Only
// budget notes are accepted, so a replayed transcript can tell them from a
// turn's start.
func (s *Session) Note(content axlr.Text) error {
	if s.Status() != StatusStreaming || len(s.Pending()) != 0 {
		return errors.New("a note requires a streaming turn without pending calls")
	}
	if !BudgetNote(content) {
		return errors.New("only budget notes join a running turn")
	}
	message := axlr.Message{Role: axlr.RoleUser, Content: content}
	if err := message.Validate(); err != nil {
		return err
	}
	next := s.Export()
	next.Messages = append(next.Messages, message)
	stampLast(&next)
	s.state = next
	return nil
}

// ResumeStepAtLimit restarts the budget of a console-driven ceremony step
// whose turn paused at the call limit, as Ctrl+R would, at most maxResumes
// times per step and attempt; the person decides after that. It reports the
// restarts this step has had, and false when it did not restart.
func (s *Session) ResumeStepAtLimit(maxResumes int) (int, bool, error) {
	run, live := s.Ceremony()
	if !live || run.AwaitingPerson() || s.Status() != StatusInterrupted || len(s.Pending()) != 0 {
		return 0, false, nil
	}
	step := fmt.Sprintf("%s#%d", run.Step, run.Iteration)
	if run.LimitStep != step {
		run.LimitStep, run.LimitResumes = step, 0
	}
	if run.LimitResumes >= maxResumes {
		return run.LimitResumes, false, nil
	}
	run.LimitResumes++
	next := Session{state: s.Export()}
	if err := next.SetCeremony(run); err != nil {
		return 0, false, err
	}
	if err := next.ResumeTurn(); err != nil {
		return 0, false, err
	}
	s.state = next.state
	return run.LimitResumes, true, nil
}
