package domain

import (
	"context"
	"errors"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ExecutionResult is what a control plane reads when an execution ends,
// without reading its events (the runtime contract, docs/runtime-contract.md):
// one execution is one session, plus the successors its context cap opened.
type ExecutionResult struct {
	Status ExecutionStatus
	// Session is the session the execution ended in; Sessions lists every
	// session it spanned, first to last.
	Session  SessionID
	Sessions []SessionID
	// Answer is the last assistant message, a console follow-up's reply
	// left out (executionAnswer); empty when the turn gave none.
	Answer axlr.Text
	// Pending is the call that waits for a decision when Status is
	// ExecutionNeedsDecision.
	Pending *axlr.ToolCall
	// Error says why the execution failed, was interrupted or exceeded its
	// budget.
	Error string
	Calls []ExecutionCall
	Usage ExecutionUsage
	// Artifacts are what the execution delivered with axlr_deliver, in the
	// sessions it spanned, in delivery order.
	Artifacts []ExecutionArtifact
	// Correlation repeats the execution's ids, nil without any.
	Correlation *ExecutionCorrelation
}

// NewExecutionResult reads how an execution ended from the session the loop
// left and the error it returned. The caller adds the usage of each
// session the execution spanned with AddUsage.
func NewExecutionResult(s Session, err error) ExecutionResult {
	state := s.Export()
	result := ExecutionResult{Session: state.ID, Sessions: []SessionID{state.ID}}
	for _, record := range state.Activity {
		result.Calls = append(result.Calls, executionCallOf(record))
	}
	result.Artifacts = SessionArtifacts(s)
	result.Answer = executionAnswer(state.Messages)
	if correlation, has := s.Correlation(); has {
		result.Correlation = &correlation
	}
	pending := s.Pending()
	switch {
	case s.BudgetStop() != nil && state.Status != StatusComplete:
		// The limit that ended the execution is read from the session, so
		// the service's result says the same as the command's.
		result.Status, result.Error = ExecutionBudgetExceeded, s.BudgetStop().Error()
		if len(pending) > 0 {
			call := pending[0].Call
			result.Pending = &call
		}
	case errors.As(err, new(*ToolFailureStreakError)):
		// The console stopped a model that kept repeating a failure: the
		// turn is interrupted, kept for a person, and the run failed.
		result.Status, result.Error = ExecutionFailed, err.Error()
	case err != nil && (errors.Is(err, context.Canceled) || state.Status == StatusInterrupted):
		result.Status, result.Error = ExecutionInterrupted, err.Error()
	case err != nil:
		result.Status, result.Error = ExecutionFailed, err.Error()
	case len(pending) > 0:
		call := pending[0].Call
		result.Status, result.Pending = ExecutionNeedsDecision, &call
	case state.Status == StatusInterrupted:
		result.Status = ExecutionInterrupted
	default:
		result.Status = ExecutionCompleted
	}
	return result
}

// AddUsage sums one session's ledger into the result's usage.
func (r *ExecutionResult) AddUsage(ledger UsageLedger) { r.Usage.Add(ledger) }

// Spanned records that the execution went through session id before the
// one it ended in: a successor opened at the context cap. The last entry
// stays the session of the result.
func (r *ExecutionResult) Spanned(ids ...SessionID) {
	if len(ids) == 0 {
		return
	}
	r.Sessions = append(append([]SessionID(nil), ids...), r.Session)
}

// Delivered adds the artifacts of the sessions the execution went through
// before the one it ended in, first to last, ahead of its own: a later
// delivery of the same reference still replaces the earlier one.
func (r *ExecutionResult) Delivered(predecessors ...Session) {
	var artifacts []ExecutionArtifact
	for _, predecessor := range predecessors {
		for _, artifact := range SessionArtifacts(predecessor) {
			artifacts = withArtifact(artifacts, artifact)
		}
	}
	for _, artifact := range r.Artifacts {
		artifacts = withArtifact(artifacts, artifact)
	}
	r.Artifacts = artifacts
}

// executionAnswer is the execution's answer: the last assistant message,
// unless it replies to a console follow-up that did no work. A reply to
// the memory reminder is never the answer, and a reply to a Jev
// follow-up is only when the follow-up called a tool; otherwise the
// answer is the one before the console's note. On 1 November 2026 an SRE
// bot's report was replaced three times by its one-sentence reply to
// those notes ("The answer is complete"), and the control plane recorded
// that sentence as the incident's diagnosis.
func executionAnswer(messages []axlr.Message) axlr.Text {
	end := len(messages)
	for end > 0 {
		at := end - 1
		for at >= 0 && messages[at].Role != axlr.RoleUser {
			at--
		}
		var answer axlr.Text
		found, worked := false, false
		for i := end - 1; i > at; i-- {
			if messages[i].Role != axlr.RoleAssistant {
				continue
			}
			if !found {
				answer, found = messages[i].Content, true
			}
			if len(messages[i].ToolCalls) > 0 {
				worked = true
			}
		}
		if at < 0 || !(MemoryNote(messages[at].Content) || JevNote(messages[at].Content) && !worked) {
			if found {
				return answer
			}
			if at < 0 {
				return ""
			}
		}
		end = at
	}
	return ""
}
