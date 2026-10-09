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
