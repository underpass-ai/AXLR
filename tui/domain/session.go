package domain

import (
	"errors"

	axlr "github.com/underpass-ai/AXLR/domain"
)

type Session struct{ state SessionState }

const MaxTurnToolCalls = 32

var ErrToolCallLimit = errors.New("turn tool-call limit reached")

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
func (s Session) Status() SessionStatus         { return s.state.Status }
func (s Session) Export() SessionState          { return cloneState(s.state) }
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
func (s *Session) ChangeModel(model axlr.ModelID) error {
	if _, err := axlr.NewModelID(string(model)); err != nil {
		return err
	}
	if s.Status() != StatusIdle && s.Status() != StatusComplete && s.Status() != StatusInterrupted {
		return errors.New("cannot change model while turn is active")
	}
	if len(s.Pending()) != 0 {
		return errors.New("pending calls must be resolved before changing model")
	}
	s.state.Model = model
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
	next.ToolSnapshot = append([]AvailableTool(nil), tools...)
	next.Status = StatusStreaming
	next.Draft = ""
	next.TurnCallCount = 0
	s.state = next
	return nil
}
func (s *Session) CompleteAssistant(result axlr.CompletionResult) error {
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
	if len(message.ToolCalls)+s.state.TurnCallCount > MaxTurnToolCalls {
		next := s.Export()
		next.Status = StatusInterrupted
		s.state = next
		return ErrToolCallLimit
	}
	next := s.Export()
	message.ToolCalls = append([]axlr.ToolCall(nil), message.ToolCalls...)
	next.Messages = append(next.Messages, message)
	next.TurnCallCount += len(message.ToolCalls)
	next.Status = StatusComplete
	for _, call := range message.ToolCalls {
		next.Activity = append(next.Activity, PendingTool{Call: call})
		next.Status = StatusApproval
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
// completed tool effects are never replayed.
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
	return nil
}
