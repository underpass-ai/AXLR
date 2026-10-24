package domain

// ExecutionCallStatus is what became of one tool call of an execution.
type ExecutionCallStatus string

const (
	ExecutionCallPending ExecutionCallStatus = "pending"
	ExecutionCallDenied  ExecutionCallStatus = "denied"
	// ExecutionCallForbidden: the execution's policy refused the call
	// outside the model, before any decision.
	ExecutionCallForbidden ExecutionCallStatus = "forbidden"
	ExecutionCallCompleted ExecutionCallStatus = "completed"
	ExecutionCallFailed    ExecutionCallStatus = "failed"
	// ExecutionCallUncertain: the call was interrupted after it may have
	// run; its effect is unknown.
	ExecutionCallUncertain ExecutionCallStatus = "uncertain"
)
