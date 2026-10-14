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

// RestoreNotePrefix opens the console note that tells the model which files
// the person restored since its last answer (application.restoreNote).
const RestoreNotePrefix = "[AXLR · restore]"

// JoinsRunningTurn reports a console note that joins a running turn
// instead of beginning one: a budget note, or a restore note delivered when
// an interrupted turn is resumed. Neither is the person's word, so the call
// budget goes on.
func JoinsRunningTurn(content axlr.Text) bool {
	return BudgetNote(content) || strings.HasPrefix(string(content), RestoreNotePrefix)
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
// which starts when the step was accepted. A Jev follow-up allows only
// what the person's request left (see JevNotePrefix).
func (s Session) callBudget(turnLimit int) (base, limit int) {
	base, limit = s.state.FinishedBudgetBase, turnLimit
	if s.state.Ceremony != nil {
		base = max(base, s.state.Ceremony.BudgetBase)
		limit = s.state.Ceremony.StepCallLimit(turnLimit)
		return base, limit
	}
	if at := lastTurnStart(s.state.Messages); at >= 0 && JevNote(s.state.Messages[at].Content) {
		limit = max(0, turnLimit-requestCalls(s.state.Messages, at))
	}
	return base, limit
}

// JevNotePrefix opens the console's follow-up turn when Jev doubts a final
// answer. The follow-up is not the person's word, so it does not start a
// new tool-call allowance: it gets what the person's request left
// (turn_tool_calls less the calls already made). The turn's count still
// restarts at the note, as at every user message, and the allowance is
// read from the transcript, so a saved session replays to the same
// count. On 14 October 2026, with turn_tool_calls 8, a request used 8
// calls and its Jev follow-up ran 8 more.
const JevNotePrefix = "[AXLR · Jev]"

// JevNote reports whether a message is a Jev follow-up.
func JevNote(content axlr.Text) bool {
	return strings.HasPrefix(string(content), JevNotePrefix)
}

// RequestCallsUsed is how many tool calls the person's last request has
// made so far, console turns included, the calls refused over budget left
// out.
func (s Session) RequestCallsUsed() int {
	return requestCalls(s.state.Messages, len(s.state.Messages))
}

// lastTurnStart is the index of the user message that started the current
// turn: the last one that is not a budget note, which joins a running
// turn. It is -1 without one.
func lastTurnStart(messages []axlr.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == axlr.RoleUser && !JoinsRunningTurn(messages[i].Content) {
			return i
		}
	}
	return -1
}

// consoleNotePrefix opens every message the console writes as a user
// message; the person's own messages never start with it.
const consoleNotePrefix = "[AXLR"

// requestCalls counts the tool calls made before end since the person's
// last message before it, console turns in between included, the calls
// refused over budget left out, as TurnCallCount does.
func requestCalls(messages []axlr.Message, end int) int {
	start := -1
	for i := end - 1; i >= 0; i-- {
		if messages[i].Role == axlr.RoleUser && !strings.HasPrefix(string(messages[i].Content), consoleNotePrefix) {
			start = i
			break
		}
	}
	if start < 0 {
		return 0
	}
	calls := 0
	for _, message := range messages[start+1 : end] {
		switch message.Role {
		case axlr.RoleAssistant:
			calls += len(message.ToolCalls)
		case axlr.RoleTool:
			if strings.HasPrefix(string(message.Content), OverBudgetOutcomePrefix) {
				calls--
			}
		}
	}
	return max(0, calls)
}

// CallsLeft is how many tool calls the current budget still allows, and
// how many it allows in all, under the turn's limit.
func (s Session) CallsLeft(turnLimit int) (left, limit int) {
	base, limit := s.callBudget(turnLimit)
	return limit - (s.state.TurnCallCount - base), limit
}

// Note adds a console message to the running turn after its tool results.
// Unlike Steer it is not the person's word: the call budget goes on. Only
// budget and restore notes are accepted, so a replayed transcript can tell
// them from a turn's start.
func (s *Session) Note(content axlr.Text) error {
	if s.Status() != StatusStreaming || len(s.Pending()) != 0 {
		return errors.New("a note requires a streaming turn without pending calls")
	}
	if !JoinsRunningTurn(content) {
		return errors.New("only budget and restore notes join a running turn")
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
