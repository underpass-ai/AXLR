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
	// Error says why the execution failed or was interrupted.
	Error string
	Calls []ExecutionCall
	Usage ExecutionUsage
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
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Role == axlr.RoleAssistant {
			result.Answer = state.Messages[i].Content
			break
		}
	}
	pending := s.Pending()
	switch {
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
