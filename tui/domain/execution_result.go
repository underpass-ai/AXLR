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
	// Answer is the last assistant message; empty when the turn gave none.
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
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Role == axlr.RoleAssistant {
			result.Answer = state.Messages[i].Content
			break
		}
	}
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
