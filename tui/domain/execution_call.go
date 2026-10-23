package domain

import axlr "github.com/underpass-ai/AXLR/domain"

// ExecutionCall summarises one tool call of an execution for its result:
// enough for a control plane to audit what ran, without the arguments or
// the output, which the session's events keep.
type ExecutionCall struct {
	ID       axlr.ToolCallID
	Tool     axlr.ToolName
	Decision ToolDecision
	Status   ExecutionCallStatus
}

// executionCallOf reads a call's status from its record in the session.
func executionCallOf(record PendingTool) ExecutionCall {
	call := ExecutionCall{ID: record.Call.ID, Tool: record.Call.Name, Decision: record.Decision, Status: ExecutionCallCompleted}
	switch {
	case record.Outcome == nil:
		call.Status = ExecutionCallPending
	case forbidden(record.Decision, *record.Outcome):
		call.Status = ExecutionCallForbidden
	case record.Decision == DecisionDeny:
		call.Status = ExecutionCallDenied
	case record.Outcome.Uncertain:
		call.Status = ExecutionCallUncertain
	case record.Outcome.IsError:
		call.Status = ExecutionCallFailed
	}
	return call
}
