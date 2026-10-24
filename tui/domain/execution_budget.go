package domain

import (
	"errors"
	"math"
	"time"
)

// ExecutionBudget bounds one execution (docs/runtime-contract.md, "Budget
// per execution"): the seconds it may run, the model requests it may make,
// the tool calls it may run and the dollars it may spend, each zero for no
// limit. It is enforced outside the model: the loop refuses the next
// request or call once a limit is reached, and the deadline cancels the
// turn. The seconds count from when the execution began, across the
// successor sessions a context cap opens; the rest sum over them.
type ExecutionBudget struct {
	seconds   int
	requests  int
	toolCalls int
	usd       float64
}

// Bounds of a budget: a limit past them is a mistake, not a budget.
const (
	MaxBudgetSeconds = 30 * 24 * 60 * 60
	MaxBudgetCount   = 1_000_000
	MaxBudgetUSD     = 1_000_000
)

func NewExecutionBudget(seconds, requests, toolCalls int, usd float64) (ExecutionBudget, error) {
	budget := ExecutionBudget{seconds: seconds, requests: requests, toolCalls: toolCalls, usd: usd}
	return budget, budget.Validate()
}

func (b ExecutionBudget) Validate() error {
	switch {
	case b.seconds < 0 || b.requests < 0 || b.toolCalls < 0:
		return errors.New("a budget's seconds, requests and tool_calls cannot be negative")
	case math.IsNaN(b.usd) || math.IsInf(b.usd, 0) || b.usd < 0:
		return errors.New("a budget's usd must be a non-negative number")
	case b.seconds > MaxBudgetSeconds:
		return errors.New("a budget's seconds exceed thirty days")
	case b.requests > MaxBudgetCount || b.toolCalls > MaxBudgetCount:
		return errors.New("a budget's requests and tool_calls exceed one million")
	case b.usd > MaxBudgetUSD:
		return errors.New("a budget's usd exceeds one million")
	}
	return nil
}

func (b ExecutionBudget) Seconds() int   { return b.seconds }
func (b ExecutionBudget) Requests() int  { return b.requests }
func (b ExecutionBudget) ToolCalls() int { return b.toolCalls }
func (b ExecutionBudget) USD() float64   { return b.usd }

// IsZero reports a budget that limits nothing.
func (b ExecutionBudget) IsZero() bool {
	return b.seconds == 0 && b.requests == 0 && b.toolCalls == 0 && b.usd == 0
}

// Deadline is when the seconds run out for an execution that began at
// startedAt; false without a limit on seconds or a start.
func (b ExecutionBudget) Deadline(startedAt time.Time) (time.Time, bool) {
	if b.seconds <= 0 || startedAt.IsZero() {
		return time.Time{}, false
	}
	return startedAt.Add(time.Duration(b.seconds) * time.Second), true
}

// Exceeded is the first of the named limits the spend has reached, nil
// while none is; with no limit named, every limit is checked in the
// order seconds, requests, tool_calls, usd.
func (b ExecutionBudget) Exceeded(spend ExecutionSpend, limits ...BudgetLimit) error {
	if len(limits) == 0 {
		limits = []BudgetLimit{BudgetSeconds, BudgetRequests, BudgetToolCalls, BudgetUSD}
	}
	for _, limit := range limits {
		switch limit {
		case BudgetSeconds:
			if b.seconds > 0 && spend.Elapsed >= time.Duration(b.seconds)*time.Second {
				return &BudgetExceededError{Limit: limit, Allowed: float64(b.seconds)}
			}
		case BudgetRequests:
			if b.requests > 0 && spend.Requests >= b.requests {
				return &BudgetExceededError{Limit: limit, Allowed: float64(b.requests)}
			}
		case BudgetToolCalls:
			if b.toolCalls > 0 && spend.ToolCalls >= b.toolCalls {
				return &BudgetExceededError{Limit: limit, Allowed: float64(b.toolCalls)}
			}
		case BudgetUSD:
			if b.usd > 0 && spend.USD >= b.usd {
				return &BudgetExceededError{Limit: limit, Allowed: b.usd}
			}
		}
	}
	return nil
}
