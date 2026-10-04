package domain

import "testing"

func TestContextBudgetValidatesAllLimits(t *testing.T) {
	for _, values := range [][4]int{{0, 1, 1, 1}, {100, 100, 10, 10}, {100, 101, 10, 10}, {100, 60, 0, 10}, {100, 60, 10, 0}, {100, 60, 10, 60}, {100, 60, 100, 10}} {
		if _, err := NewContextBudget(values[0], values[1], values[2], values[3]); err == nil {
			t.Fatalf("accepted invalid limits: %v", values)
		}
	}
	budget := DefaultContextBudget()
	if budget.MaximumBytes() != 256*1024 || budget.LowWaterBytes() != 192*1024 || budget.ToolResultBytes() != 32*1024 || budget.CheckpointBytes() != 16*1024 || budget.Validate() != nil {
		t.Fatalf("unexpected defaults: %+v", budget)
	}
	if (ContextBudget{}).Validate() == nil {
		t.Fatal("zero budget accepted")
	}
}
