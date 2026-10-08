package domain

import "testing"

func TestContextBudgetForPromptDerivesFromThePromptSize(t *testing.T) {
	for _, tc := range []struct {
		tokens                                         int
		maximum, lowWater, toolResult, checkpointBytes int
	}{
		{64000, 138752, 69376, 17344, 11562},
		{DefaultPromptTokens, 138752, 69376, 17344, 11562},
		{MinimumPromptTokens, 22568, 11284, 2821, 1880},
		{32000, 60672, 30336, 7584, 5056},
		{96000, 216832, 108416, 27104, 16384},
	} {
		budget := ContextBudgetForPrompt(tc.tokens)
		if budget.Validate() != nil || budget.MaximumBytes() != tc.maximum || budget.LowWaterBytes() != tc.lowWater || budget.ToolResultBytes() != tc.toolResult || budget.CheckpointBytes() != tc.checkpointBytes {
			t.Fatalf("prompt %d: budget = %d/%d/%d/%d", tc.tokens, budget.MaximumBytes(), budget.LowWaterBytes(), budget.ToolResultBytes(), budget.CheckpointBytes())
		}
		// Messages, guidance and schemas together never exceed the prompt
		// at the measured 2.44 bytes per token.
		if budget.MaximumBytes()+promptPrefixReserve > tc.tokens*244/100 {
			t.Fatalf("prompt %d: budget exceeds the prompt", tc.tokens)
		}
	}
	for _, tokens := range []int{0, -1, MinimumPromptTokens - 1} {
		if ContextBudgetForPrompt(tokens) != ContextBudgetForPrompt(DefaultPromptTokens) {
			t.Fatalf("prompt %d did not get the default prompt budget", tokens)
		}
	}
	for _, tokens := range []int{437_000, 1_000_000, 10_000_000} {
		if ContextBudgetForPrompt(tokens) != DefaultContextBudget() {
			t.Fatalf("prompt %d did not keep the ceiling", tokens)
		}
	}
	if ContextBudgetForWindow(0) != ContextBudgetForPrompt(DefaultPromptTokens) {
		t.Fatal("an unknown window did not get the prompt budget")
	}
}
