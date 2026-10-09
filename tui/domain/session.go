package domain

import (
	"errors"
	"fmt"
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

type Session struct {
	state SessionState
	// replaying is true while restore rebuilds a saved transcript: the
	// per-turn call limit guards live turns, not history that already ran.
	replaying bool
}

const MaxTurnToolCalls = 32

var ErrToolCallLimit = errors.New("turn tool-call limit reached")

// now is the session clock; tests replace it.
var now = time.Now

// stampLast records when the last message was added, padding unknown times
// for messages from sessions saved before times were kept.
func stampLast(state *SessionState) {
	for len(state.MessageTimes) < len(state.Messages)-1 {
		state.MessageTimes = append(state.MessageTimes, time.Time{})
	}
	state.MessageTimes = append(state.MessageTimes[:len(state.Messages)-1], now().UTC().Truncate(time.Second))
}

func NewSession(id SessionID, workspace Workspace, model axlr.ModelID) (Session, error) {
	if _, err := NewSessionID(string(id)); err != nil {
		return Session{}, err
	}
	if _, err := NewWorkspace(string(workspace)); err != nil {
		return Session{}, err
	}
	if _, err := axlr.NewModelID(string(model)); err != nil {
		return Session{}, err
	}
	return Session{state: SessionState{ID: id, Workspace: workspace, Model: model, Status: StatusIdle}}, nil
}
func (s Session) Status() SessionStatus { return s.state.Status }
func (s Session) Export() SessionState  { return cloneState(s.state) }
func (s *Session) SetServiceMetadata(owner string, revision uint64, operationID string) {
	s.state.Owner, s.state.Revision, s.state.OperationID = owner, revision, operationID
}
func (s Session) Messages() []axlr.Message      { return s.Export().Messages }
func (s Session) ToolSnapshot() []AvailableTool { return s.Export().ToolSnapshot }
func (s Session) Pending() []PendingTool {
	var pending []PendingTool
	for _, call := range s.state.Activity {
		if call.Outcome == nil {
			pending = append(pending, call)
		}
	}
	return pending
}

// requireBetweenTurns verifies that a session change can only happen between turns.
// The change parameter is used in error messages (e.g., "model" or "mode").
func (s Session) requireBetweenTurns(change string) error {
	if s.Status() != StatusIdle && s.Status() != StatusComplete && s.Status() != StatusInterrupted {
		return errors.New("cannot change " + change + " while turn is active")
	}
	if len(s.Pending()) != 0 {
		return errors.New("pending calls must be resolved before changing " + change)
	}
	return nil
}
func (s *Session) ChangeModel(model axlr.ModelID) error {
	if _, err := axlr.NewModelID(string(model)); err != nil {
		return err
	}
	if err := s.requireBetweenTurns("model"); err != nil {
		return err
	}
	s.state.Model = model
	return nil
}

func (s Session) Mode() WorkMode {
	if s.state.Mode == "" {
		return ModeNormal
	}
	return s.state.Mode
}

// SetMode changes how the next turn works. A turn in progress keeps the mode
// it started with.
func (s *Session) SetMode(mode WorkMode) error {
	if err := mode.Validate(); err != nil {
		return err
	}
	if err := s.requireBetweenTurns("mode"); err != nil {
		return err
	}
	s.state.Mode = mode
	return nil
}

func (s *Session) BeginTurn(prompt axlr.Text, tools []AvailableTool) error {
	if s.Status() != StatusIdle && s.Status() != StatusComplete && s.Status() != StatusInterrupted {
		return errors.New("cannot begin turn in current state")
	}
	if len(s.Pending()) != 0 {
		return errors.New("pending calls must be cancelled before a new turn")
	}
	message := axlr.Message{Role: axlr.RoleUser, Content: prompt}
	if err := message.Validate(); err != nil {
		return err
	}
	if err := validateTools(s.state.Model, tools); err != nil {
		return err
	}
	next := s.Export()
	if s.Status() == StatusInterrupted && next.Draft != "" {
		next.ArchivedDrafts = append(next.ArchivedDrafts, ArchivedDraft{AfterMessage: len(next.Messages), Content: next.Draft})
	}
	next.Messages = append(next.Messages, message)
	stampLast(&next)
	next.ToolSnapshot = append([]AvailableTool(nil), tools...)
	next.Status = StatusStreaming
	next.Draft = ""
	next.TurnCallCount = 0
	if next.Ceremony != nil {
		next.Ceremony.BudgetBase = 0
	}
	next.FinishedBudgetBase = 0
	s.state = next
	return nil
}

// Steer adds a message the person sent while the turn was running. It joins
// the running turn after its tool results; like a new turn, it restarts the
// call budget, which counts calls since the last user message.
func (s *Session) Steer(prompt axlr.Text) error {
	if s.Status() != StatusStreaming || len(s.Pending()) != 0 {
		return errors.New("steering requires a streaming turn without pending calls")
	}
	if s.state.Ceremony != nil {
		return errors.New("a ceremony step cannot be steered")
	}
	message := axlr.Message{Role: axlr.RoleUser, Content: prompt}
	if err := message.Validate(); err != nil {
		return err
	}
	next := s.Export()
	next.Messages = append(next.Messages, message)
	stampLast(&next)
	next.TurnCallCount = 0
	next.FinishedBudgetBase = 0
	s.state = next
	return nil
}

// CompleteAssistantWithin records the model's answer; turnLimit is the
// turn's tool-call budget (settings' turn_tool_calls), which a compact
// ceremony step lowers to its own.
func (s *Session) CompleteAssistantWithin(result axlr.CompletionResult, turnLimit int) error {
	if s.Status() != StatusStreaming {
		return errors.New("assistant completion requires streaming state")
	}
	message := result.Message
	if message.Role != axlr.RoleAssistant {
		return errors.New("completion must be an assistant message")
	}
	if err := message.Validate(); err != nil {
		return err
	}
	ids := make(map[axlr.ToolCallID]bool)
	for _, record := range s.state.Activity {
		ids[record.Call.ID] = true
	}
	for _, call := range message.ToolCalls {
		if ids[call.ID] {
			return errors.New("duplicate tool call ID")
		}
		ids[call.ID] = true
	}
	base, limit := s.callBudget(turnLimit)
	over := !s.replaying && len(message.ToolCalls) > 0 && len(message.ToolCalls)+s.state.TurnCallCount-base > limit
	next := s.Export()
	message.ToolCalls = append([]axlr.ToolCall(nil), message.ToolCalls...)
	// Arguments are held in the compact form the session store writes, so a
	// resumed session sends its earlier calls with the bytes the live one did.
	// A replay checks the saved bytes as they are.
	if !s.replaying {
		for i := range message.ToolCalls {
			message.ToolCalls[i].Arguments = message.ToolCalls[i].Arguments.Compact()
		}
	}
	next.Messages = append(next.Messages, message)
	stampLast(&next)
	next.TurnCallCount += len(message.ToolCalls)
	next.Status = StatusComplete
	for _, call := range message.ToolCalls {
		next.Activity = append(next.Activity, PendingTool{Call: call})
		next.Status = StatusApproval
	}
	if over {
		// The answer was streamed to the person, so it stays; none of its
		// calls runs and the turn pauses. Results follow call order, so every
		// call of this answer is answered, not only those past the limit.
		capped := Session{state: next}
		outcome := ToolOutcome{Content: axlr.Text(fmt.Sprintf("not run: the turn reached its %d tool-call limit; send a message to continue", limit)), IsError: true}
		for _, call := range message.ToolCalls {
			if err := capped.RecordToolOutcome(call.ID, DecisionDeny, outcome); err != nil {
				return err
			}
		}
		capped.state.Status = StatusInterrupted
		s.state = capped.state
		return ErrToolCallLimit
	}
	s.state = next
	return nil
}
func (s *Session) RecordToolOutcome(id axlr.ToolCallID, decision ToolDecision, outcome ToolOutcome) error {
	if outcome.Change != nil {
		if outcome.IsError || outcome.Uncertain || decision == DecisionDeny {
			return errors.New("file change requires a completed execution")
		}
		if err := outcome.Change.Validate(); err != nil {
			return err
		}
	}
	outcome = cloneOutcome(outcome)
	if s.Status() != StatusApproval {
		return errors.New("tool result requires approval state")
	}
	if decision != DecisionApprove && decision != DecisionAutoApprove && decision != DecisionDeny {
		return errors.New("invalid tool decision")
	}
	if _, err := axlr.NewText(string(outcome.Content)); err != nil {
		return err
	}
	pending := s.Pending()
	if len(pending) == 0 || pending[0].Call.ID != id {
		return errors.New("tool results must follow call order")
	}
	next := s.Export()
	for i := range next.Activity {
		if next.Activity[i].Call.ID == id {
			next.Activity[i].Decision = decision
			next.Activity[i].Outcome = &outcome
			break
		}
	}
	next.Messages = append(next.Messages, axlr.Message{Role: axlr.RoleTool, ToolCallID: id, Content: outcome.Content})
	stampLast(&next)
	if len(pending) == 1 {
		next.Status = StatusStreaming
	}
	s.state = next
	return nil
}
func (s *Session) InterruptDraft(draft axlr.Text) error {
	if s.Status() != StatusStreaming {
		return errors.New("draft interruption requires streaming state")
	}
	if _, err := axlr.NewText(string(draft)); err != nil {
		return err
	}
	next := s.Export()
	next.Draft = draft
	next.Status = StatusInterrupted
	s.state = next
	return nil
}
func (s *Session) CancelPending() error {
	if s.Status() != StatusApproval && s.Status() != StatusStreaming && s.Status() != StatusInterrupted {
		return errors.New("cannot cancel inactive turn")
	}
	next := Session{state: s.Export()}
	for _, pending := range next.Pending() {
		next.state.Status = StatusApproval
		if err := next.RecordToolOutcome(pending.Call.ID, DecisionDeny, ToolOutcome{Content: "tool call cancelled", IsError: true}); err != nil {
			return err
		}
	}
	next.state.Status = StatusInterrupted
	s.state = next.state
	return nil
}

// ResumePending is an explicit user transition; it restores approval, never execution.
func (s *Session) ResumePending() error {
	if s.Status() != StatusInterrupted || len(s.Pending()) == 0 {
		return errors.New("resume requires interrupted pending calls")
	}
	next := s.Export()
	next.Status = StatusApproval
	s.state = next
	return nil
}

// PauseTurn blocks further work until an explicit user action.
func (s *Session) PauseTurn() error {
	if s.Status() != StatusApproval && s.Status() != StatusStreaming {
		return errors.New("pause requires an active turn")
	}
	next := s.Export()
	next.Status = StatusInterrupted
	s.state = next
	return nil
}

// FinishToolExecution replaces the durable uncertain checkpoint with the observed
// result. The checkpoint itself remains a valid result if execution or saving fails.
func (s *Session) FinishToolExecution(id axlr.ToolCallID, outcome ToolOutcome) error {
	if outcome.Change != nil {
		if outcome.IsError || outcome.Uncertain {
			return errors.New("file change requires a completed execution")
		}
		if err := outcome.Change.Validate(); err != nil {
			return err
		}
	}
	outcome = cloneOutcome(outcome)
	if s.Status() != StatusInterrupted {
		return errors.New("execution result requires interrupted checkpoint")
	}
	if _, err := axlr.NewText(string(outcome.Content)); err != nil {
		return err
	}
	next := s.Export()
	found := false
	for i, p := range next.Activity {
		if p.Call.ID == id && (p.Decision == DecisionApprove || p.Decision == DecisionAutoApprove) && p.Outcome != nil && p.Outcome.Uncertain {
			next.Activity[i].Outcome = &outcome
			found = true
			break
		}
	}
	if !found {
		return errors.New("execution checkpoint not found")
	}
	for i, m := range next.Messages {
		if m.Role == axlr.RoleTool && m.ToolCallID == id {
			next.Messages[i].Content = outcome.Content
			break
		}
	}
	next.Status = StatusStreaming
	for _, p := range next.Activity {
		if p.Outcome == nil {
			next.Status = StatusApproval
			break
		}
	}
	s.state = next
	return nil
}

// ResumeTurn is an explicit recovery action. Pending calls return to approval;
// completed tool effects are never replayed. Like a steered message, it is
// the person's word to go on, so it restarts the turn's call budget: a turn
// paused at the limit, or retried by the console, gets a new one instead of
// tripping the limit again.
func (s *Session) ResumeTurn() error {
	if s.Status() != StatusInterrupted {
		return errors.New("resume requires interrupted turn")
	}
	next := s.Export()
	next.Status = StatusStreaming
	next.Draft = ""
	if len(s.Pending()) > 0 {
		next.Status = StatusApproval
	}
	s.state = next
	s.RestartTurnBudget()
	return nil
}
