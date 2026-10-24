package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

// A budget is four limits, each zero for none, within sane bounds.
func TestExecutionBudgetValidation(t *testing.T) {
	if _, err := NewExecutionBudget(900, 20, 60, 0.5); err != nil {
		t.Fatal(err)
	}
	none, err := NewExecutionBudget(0, 0, 0, 0)
	if err != nil || !none.IsZero() {
		t.Fatalf("%v %+v", err, none)
	}
	for name, tc := range map[string]struct {
		seconds, requests, calls int
		usd                      float64
	}{
		"negative seconds":    {-1, 0, 0, 0},
		"negative requests":   {0, -1, 0, 0},
		"negative tool calls": {0, 0, -1, 0},
		"negative usd":        {0, 0, 0, -0.01},
		"nan usd":             {0, 0, 0, math.NaN()},
		"infinite usd":        {0, 0, 0, math.Inf(1)},
		"a month and more":    {MaxBudgetSeconds + 1, 0, 0, 0},
		"too many requests":   {0, MaxBudgetCount + 1, 0, 0},
		"too many calls":      {0, 0, MaxBudgetCount + 1, 0},
		"too many dollars":    {0, 0, 0, MaxBudgetUSD + 1},
	} {
		if _, err := NewExecutionBudget(tc.seconds, tc.requests, tc.calls, tc.usd); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Exceeded names the first limit the spend reached, in the contract's
// words; a zero limit is never reached.
func TestExecutionBudgetExceeded(t *testing.T) {
	budget, _ := NewExecutionBudget(900, 20, 60, 0.5)
	if err := budget.Exceeded(ExecutionSpend{Elapsed: 899 * time.Second, Requests: 19, ToolCalls: 59, USD: 0.49}); err != nil {
		t.Fatalf("under every limit: %v", err)
	}
	for _, tc := range []struct {
		spend ExecutionSpend
		want  string
		limit BudgetLimit
	}{
		{ExecutionSpend{Elapsed: 900 * time.Second}, "budget: seconds 900 reached", BudgetSeconds},
		{ExecutionSpend{Requests: 20}, "budget: requests 20 reached", BudgetRequests},
		{ExecutionSpend{ToolCalls: 60}, "budget: tool_calls 60 reached", BudgetToolCalls},
		{ExecutionSpend{USD: 0.5}, "budget: usd 0.50 reached", BudgetUSD},
	} {
		err := budget.Exceeded(tc.spend)
		var exceeded *BudgetExceededError
		if !errors.As(err, &exceeded) || exceeded.Limit != tc.limit || err.Error() != tc.want {
			t.Fatalf("%+v: %v", tc.spend, err)
		}
	}
	// Only the named limits are checked.
	if err := budget.Exceeded(ExecutionSpend{ToolCalls: 60}, BudgetRequests, BudgetUSD); err != nil {
		t.Fatalf("tool calls checked when not named: %v", err)
	}
	none, _ := NewExecutionBudget(0, 0, 0, 0)
	if err := none.Exceeded(ExecutionSpend{Elapsed: time.Hour, Requests: 1000, ToolCalls: 1000, USD: 100}); err != nil {
		t.Fatalf("no limit reached: %v", err)
	}
	started := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	if deadline, ok := budget.Deadline(started); !ok || !deadline.Equal(started.Add(15*time.Minute)) {
		t.Fatalf("deadline %v %v", deadline, ok)
	}
	if _, ok := none.Deadline(started); ok {
		t.Fatal("a budget without seconds has a deadline")
	}
	if _, ok := budget.Deadline(time.Time{}); ok {
		t.Fatal("a deadline without a start")
	}
}

// Correlation ids are safe ASCII of bounded length; any may be empty.
func TestExecutionCorrelationValidation(t *testing.T) {
	ok := ExecutionCorrelation{BotID: "server-incident-investigator", TaskID: "task-789", ExecutionID: "exec-1", TraceID: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}
	if err := ok.Validate(); err != nil || ok.IsZero() {
		t.Fatal(err)
	}
	if err := (ExecutionCorrelation{}).Validate(); err != nil || !(ExecutionCorrelation{}).IsZero() {
		t.Fatal(err)
	}
	long := make([]byte, MaxCorrelationIDBytes+1)
	for i := range long {
		long[i] = 'a'
	}
	for name, id := range map[string]string{"space": "bot 1", "quote": `bot"1`, "newline": "bot\n1", "unicode": "bót", "control": "bot\x01", "long": string(long)} {
		if err := (ExecutionCorrelation{BotID: id}).Validate(); err == nil {
			t.Errorf("%s: accepted %q", name, id)
		}
	}
}
