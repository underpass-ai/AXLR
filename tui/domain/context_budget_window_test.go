package domain

import "testing"

func TestContextBudgetForWindowScalesWithTheWindow(t *testing.T) {
	for _, window := range []ContextWindow{1 << 20, 10_000_000} {
		if ContextBudgetForWindow(window) != DefaultContextBudget() {
			t.Fatalf("window %d did not keep the default budget", window)
		}
	}
	// An unknown window says nothing about the model, so it gets the prompt
	// budget rather than the ceiling.
	for _, window := range []ContextWindow{0, 1, MinimumContextWindow - 1} {
		if ContextBudgetForWindow(window) != ContextBudgetForPrompt(DefaultPromptTokens) {
			t.Fatalf("window %d did not get the prompt budget", window)
		}
	}
	for _, tc := range []struct {
		window                                         ContextWindow
		maximum, lowWater, toolResult, checkpointBytes int
	}{
		{4096, 9216, 6912, 1152, 864},
		{32768, 73728, 55296, 9216, 6912},
		{65536, 147456, 110592, 18432, 13824},
		{262144, 589824, 442368, 64 << 10, 16 << 10},
	} {
		budget := ContextBudgetForWindow(tc.window)
		if budget.Validate() != nil || budget.MaximumBytes() != tc.maximum || budget.LowWaterBytes() != tc.lowWater || budget.ToolResultBytes() != tc.toolResult || budget.CheckpointBytes() != tc.checkpointBytes {
			t.Fatalf("window %d: budget = %d/%d/%d/%d", tc.window, budget.MaximumBytes(), budget.LowWaterBytes(), budget.ToolResultBytes(), budget.CheckpointBytes())
		}
		// The messages never claim more than three quarters of the window
		// at three bytes per token.
		if budget.MaximumBytes() > tc.window.Tokens()*3/4*3 {
			t.Fatalf("window %d: budget exceeds its share", tc.window)
		}
	}
}
