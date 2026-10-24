package domain

import "errors"

// BudgetLimit names one limit of an execution's budget, as the contract
// spells it.
type BudgetLimit string

const (
	BudgetSeconds   BudgetLimit = "seconds"
	BudgetRequests  BudgetLimit = "requests"
	BudgetToolCalls BudgetLimit = "tool_calls"
	BudgetUSD       BudgetLimit = "usd"
)

// Validate accepts the four limits of the contract.
func (l BudgetLimit) Validate() error {
	switch l {
	case BudgetSeconds, BudgetRequests, BudgetToolCalls, BudgetUSD:
		return nil
	}
	return errors.New("unknown budget limit")
}

var errBudgetSpendNegative = errors.New("a budget's spend cannot be negative")
