package domain

// ExecutionCallStatus is what became of one tool call of an execution.
type ExecutionCallStatus string

const (
	ExecutionCallPending   ExecutionCallStatus = "pending"
	ExecutionCallDenied    ExecutionCallStatus = "denied"
	ExecutionCallCompleted ExecutionCallStatus = "completed"
	ExecutionCallFailed    ExecutionCallStatus = "failed"
	// ExecutionCallUncertain: the call was interrupted after it may have
	// run; its effect is unknown.
	ExecutionCallUncertain ExecutionCallStatus = "uncertain"
)
