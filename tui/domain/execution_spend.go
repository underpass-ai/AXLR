package domain

import "time"

// ExecutionSpend is what an execution has used of its budget: the time
// since it began, the model requests it made, the tool calls it ran and
// the dollars its requests reported.
type ExecutionSpend struct {
	Elapsed   time.Duration
	Requests  int
	ToolCalls int
	USD       float64
}

// Validate refuses a spend that counts backwards.
func (s ExecutionSpend) Validate() error {
	if s.Elapsed < 0 || s.Requests < 0 || s.ToolCalls < 0 || s.USD < 0 {
		return errBudgetSpendNegative
	}
	return nil
}
