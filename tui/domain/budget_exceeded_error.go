package domain

import (
	"fmt"
	"strconv"
)

// BudgetExceededError ends an execution at the limit it reached. The loop
// returns it instead of the next model request or tool call, and the
// result reads budget_exceeded with its text as the error: "budget:
// tool_calls 60 reached".
type BudgetExceededError struct {
	Limit BudgetLimit
	// Allowed is the limit's value: whole for seconds, requests and tool
	// calls, dollars for usd.
	Allowed float64
}

func (e *BudgetExceededError) Error() string {
	return "budget: " + string(e.Limit) + " " + e.amount() + " reached"
}

func (e *BudgetExceededError) amount() string {
	if e.Limit == BudgetUSD {
		return strconv.FormatFloat(e.Allowed, 'f', 2, 64)
	}
	return fmt.Sprintf("%d", int64(e.Allowed))
}

// BudgetStopOf is the error of a stop recorded on a session, for the limit
// and the budget it was taken under.
func budgetStopOf(limit BudgetLimit, budget ExecutionBudget) *BudgetExceededError {
	switch limit {
	case BudgetSeconds:
		return &BudgetExceededError{Limit: limit, Allowed: float64(budget.seconds)}
	case BudgetRequests:
		return &BudgetExceededError{Limit: limit, Allowed: float64(budget.requests)}
	case BudgetToolCalls:
		return &BudgetExceededError{Limit: limit, Allowed: float64(budget.toolCalls)}
	case BudgetUSD:
		return &BudgetExceededError{Limit: limit, Allowed: budget.usd}
	}
	return nil
}
