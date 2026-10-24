package domain

// ExecutionStatus is how an execution ended, in the runtime contract's
// words (docs/runtime-contract.md): the states a control plane acts on
// without reading the session's events.
type ExecutionStatus string

const (
	// ExecutionCompleted: the turn ended in an answer.
	ExecutionCompleted ExecutionStatus = "completed"
	// ExecutionNeedsDecision: the turn waits for a decision on a tool call
	// the run was not allowed to take; the session keeps the call.
	ExecutionNeedsDecision ExecutionStatus = "needs_decision"
	// ExecutionInterrupted: the turn stopped before its end (cancelled, or
	// the process died) and the session can be resumed.
	ExecutionInterrupted ExecutionStatus = "interrupted"
	// ExecutionFailed: the loop returned an error.
	ExecutionFailed ExecutionStatus = "failed"
	// ExecutionBudgetExceeded: a limit of the execution's budget ended it
	// before the next request or call; the error names the limit and the
	// session can be resumed, its pending state intact.
	ExecutionBudgetExceeded ExecutionStatus = "budget_exceeded"
)
