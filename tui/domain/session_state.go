package domain

import (
	"bytes"
	"errors"
	"slices"
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// SessionState is the domain persistence boundary, independent of storage DTOs.
// Activity retains all requested calls in transcript order, including resolved calls.
// TurnCallCount counts calls since the last user message; Draft is never model history.
type SessionState struct {
	ID             SessionID
	Owner          string
	Revision       uint64
	OperationID    string
	Workspace      Workspace
	Model          axlr.ModelID
	Status         SessionStatus
	Messages       []axlr.Message
	ToolSnapshot   []AvailableTool
	Activity       []PendingTool
	ArchivedDrafts []ArchivedDraft
	Draft          axlr.Text
	TurnCallCount  int
	// MessageTimes[i] is when Messages[i] was added, in UTC to the second.
	// Sessions saved before times were recorded have fewer entries; a zero
	// time means unknown.
	MessageTimes []time.Time
	// Mode is the session's work mode; empty reads as ModeNormal. It is kept
	// outside the snapshot so the snapshot format does not change.
	Mode WorkMode
	// Ceremony is the live console-driven ceremony, if any. Like Mode, it is
	// kept outside the snapshot.
	Ceremony *CeremonyRun
}

// RestoreSession validates transcript and activity together without executing work.
// Active states reopen interrupted and require an explicit user action.
func RestoreSession(state SessionState) (Session, error) {
	return restoreSession(state, true)
}

// RestoreSessionActive validates a live service snapshot without treating the
// in-process writer as a crashed turn. Startup recovery calls PauseTurn itself.
func RestoreSessionActive(state SessionState) (Session, error) {
	return restoreSession(state, false)
}

func restoreSession(state SessionState, interruptActive bool) (Session, error) {
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
	if state.Mode != "" {
		if err = state.Mode.Validate(); err != nil {
			return Session{}, err
		}
	}
	if state.Ceremony != nil {
		if err = state.Ceremony.Validate(); err != nil {
			return Session{}, err
		}
	}
	previous := 0
	for _, archived := range state.ArchivedDrafts {
		if archived.AfterMessage <= previous || archived.AfterMessage >= len(state.Messages) || archived.Content == "" || state.Messages[archived.AfterMessage].Role != axlr.RoleUser {
			return Session{}, errors.New("archived draft has no following user turn")
		}
		if _, err = axlr.NewText(string(archived.Content)); err != nil {
			return Session{}, err
		}
		previous = archived.AfterMessage
	}
	switch state.Status {
	case StatusIdle, StatusStreaming, StatusApproval, StatusInterrupted, StatusComplete:
	default:
		return Session{}, errors.New("invalid session status")
	}
	activityIndex := 0
	s.replaying = true
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
	s.replaying = false
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
	if interruptActive && (state.Status == StatusStreaming || state.Status == StatusApproval) {
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
	state.ArchivedDrafts = append([]ArchivedDraft(nil), state.ArchivedDrafts...)
	state.MessageTimes = append([]time.Time(nil), state.MessageTimes...)
	if state.Ceremony != nil {
		run := state.Ceremony.clone()
		state.Ceremony = &run
	}
	if len(state.MessageTimes) > len(state.Messages) {
		state.MessageTimes = state.MessageTimes[:len(state.Messages)]
	}
	for i := range state.Activity {
		if state.Activity[i].Outcome != nil {
			outcome := cloneOutcome(*state.Activity[i].Outcome)
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
	if a.Outcome.Content != b.Outcome.Content || a.Outcome.IsError != b.Outcome.IsError || a.Outcome.Uncertain != b.Outcome.Uncertain {
		return false
	}
	if a.Outcome.Change == nil || b.Outcome.Change == nil {
		return a.Outcome.Change == nil && b.Outcome.Change == nil
	}
	return *a.Outcome.Change == *b.Outcome.Change
}
