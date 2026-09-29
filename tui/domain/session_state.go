package domain

import (
	"bytes"
	"errors"
	"slices"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// SessionState is the domain persistence boundary, independent of storage DTOs.
// Activity retains all requested calls in transcript order, including resolved calls.
// TurnCallCount counts calls since the last user message; Draft is never model history.
type SessionState struct {
	ID            SessionID
	Workspace     Workspace
	Model         axlr.ModelID
	Status        SessionStatus
	Messages      []axlr.Message
	ToolSnapshot  []AvailableTool
	Activity      []PendingTool
	Draft         axlr.Text
	TurnCallCount int
}

// RestoreSession validates transcript and activity together without executing work.
// Active states reopen interrupted and require an explicit user action.
func RestoreSession(state SessionState) (Session, error) {
	s, err := NewSession(state.ID, state.Workspace, state.Model)
	if err != nil {
		return Session{}, err
	}
	if err = validateTools(state.Model, state.ToolSnapshot); err != nil {
		return Session{}, err
	}
	if _, err = axlr.NewText(string(state.Draft)); err != nil {
		return Session{}, err
	}
	switch state.Status {
	case StatusIdle, StatusStreaming, StatusApproval, StatusInterrupted, StatusComplete:
	default:
		return Session{}, errors.New("invalid session status")
	}
	activityIndex := 0
	for _, message := range state.Messages {
		if err = message.Validate(); err != nil {
			return Session{}, err
		}
		switch message.Role {
		case axlr.RoleUser:
			// A previous stream can have ended without a complete assistant response.
			if s.Status() == StatusStreaming {
				s.state.Status = StatusInterrupted
			}
			err = s.BeginTurn(message.Content, state.ToolSnapshot)
		case axlr.RoleAssistant:
			err = s.CompleteAssistant(axlr.CompletionResult{Message: message})
		case axlr.RoleTool:
			if activityIndex >= len(state.Activity) {
				return Session{}, errors.New("missing tool activity")
			}
			record := state.Activity[activityIndex]
			activityIndex++
			if record.Outcome == nil || record.Outcome.Content != message.Content {
				return Session{}, errors.New("tool outcome does not match transcript")
			}
			err = s.RecordToolOutcome(message.ToolCallID, record.Decision, *record.Outcome)
		default:
			err = errors.New("session history must start with a user turn")
		}
		if err != nil {
			return Session{}, err
		}
	}
	if s.state.TurnCallCount != state.TurnCallCount || !slices.EqualFunc(s.state.Activity, state.Activity, sameActivity) {
		return Session{}, errors.New("inconsistent tool activity or turn count")
	}
	switch state.Status {
	case StatusIdle:
		if len(state.Messages) != 0 || len(state.ToolSnapshot) != 0 {
			return Session{}, errors.New("idle session contains turn state")
		}
	case StatusComplete:
		if s.Status() != StatusComplete {
			return Session{}, errors.New("incomplete transcript marked complete")
		}
	case StatusStreaming:
		if s.Status() != StatusStreaming {
			return Session{}, errors.New("invalid streaming transcript")
		}
	case StatusApproval:
		if s.Status() != StatusApproval {
			return Session{}, errors.New("approval requires pending calls")
		}
	case StatusInterrupted:
		if s.Status() != StatusStreaming && s.Status() != StatusApproval {
			return Session{}, errors.New("invalid interrupted transcript")
		}
	}
	if state.Draft != "" && (state.Status != StatusInterrupted || len(s.Pending()) != 0) {
		return Session{}, errors.New("draft requires interrupted stream")
	}
	s.state = cloneState(state)
	if state.Status == StatusStreaming || state.Status == StatusApproval {
		s.state.Status = StatusInterrupted
	}
	return s, nil
}
func cloneState(state SessionState) SessionState {
	state.Messages = append([]axlr.Message(nil), state.Messages...)
	for i := range state.Messages {
		state.Messages[i].ToolCalls = append([]axlr.ToolCall(nil), state.Messages[i].ToolCalls...)
	}
	state.ToolSnapshot = append([]AvailableTool(nil), state.ToolSnapshot...)
	state.Activity = append([]PendingTool(nil), state.Activity...)
	for i := range state.Activity {
		if state.Activity[i].Outcome != nil {
			outcome := *state.Activity[i].Outcome
			state.Activity[i].Outcome = &outcome
		}
	}
	return state
}

func sameActivity(a, b PendingTool) bool {
	if a.Call.ID != b.Call.ID || a.Call.Name != b.Call.Name || !bytes.Equal(a.Call.Arguments.Bytes(), b.Call.Arguments.Bytes()) || a.Decision != b.Decision {
		return false
	}
	if a.Outcome == nil || b.Outcome == nil {
		return a.Outcome == nil && b.Outcome == nil
	}
	return *a.Outcome == *b.Outcome
}
