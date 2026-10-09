package domain

import (
	"math"
	"testing"
)

func TestBytesPerTokenAveragesMeasuredRequests(t *testing.T) {
	var b BytesPerToken
	if _, ok := b.Hundredths(); ok {
		t.Fatal("an unmeasured model has a ratio")
	}
	// A prompt too small to measure, or a request without a size, is not a sample.
	if got := b.Observe(4000, minimumCalibrationTokens-1); got != b {
		t.Fatalf("small prompt = %+v", got)
	}
	if got := b.Observe(0, 10000); got != b {
		t.Fatalf("unknown size = %+v", got)
	}
	// GLM on 8 October 2026: about 4.2 bytes per prompt token.
	for i := 1; i < CalibrationSamples; i++ {
		b = b.Observe(42000, 10000)
		if _, ok := b.Hundredths(); ok {
			t.Fatalf("calibrated after %d samples", i)
		}
	}
	b = b.Observe(42000, 10000)
	if got, ok := b.Hundredths(); !ok || got != 420 || b.Samples != CalibrationSamples {
		t.Fatalf("ratio = %d %v after %d samples", got, ok, b.Samples)
	}
	// Past the window one request weighs a fiftieth.
	b = BytesPerToken{Ratio: 4, Samples: 500}.Observe(50000, 10000)
	if math.Abs(b.Ratio-4.02) > 1e-9 || b.Samples != 501 {
		t.Fatalf("windowed average = %+v", b)
	}
}

func TestBytesPerTokenClampsOddRequests(t *testing.T) {
	low := BytesPerToken{}.Observe(2048, 2048)
	high := BytesPerToken{}.Observe(100*10000, 10000)
	if low.Ratio != 1.5 || high.Ratio != 6 {
		t.Fatalf("clamped = %v and %v", low.Ratio, high.Ratio)
	}
	// A stored ratio outside the range is replaced, never trusted.
	if got := (BytesPerToken{Ratio: 40, Samples: 9}).Observe(30000, 10000); got.Ratio != 3 || got.Samples != 1 {
		t.Fatalf("invalid stored ratio = %+v", got)
	}
	if _, ok := (BytesPerToken{Ratio: 40, Samples: 9}).Hundredths(); ok {
		t.Fatal("an invalid stored ratio was applied")
	}
}

func TestContextBudgetForPromptAtSizesTheBudgetToTheMeasuredRatio(t *testing.T) {
	if ContextBudgetForPromptAt(DefaultPromptTokens, promptBytesPerTokenHundredths) != ContextBudgetForPrompt(DefaultPromptTokens) {
		t.Fatal("the default ratio changed the budget")
	}
	// At 2.44 bytes per token GLM's requests stopped at about 37K tokens;
	// at its measured 4.2 they reach the 64K the budget aims at.
	at244 := ContextBudgetForPrompt(DefaultPromptTokens)
	if tokens := (at244.MaximumBytes() + promptPrefixReserve) * 10 / 42; tokens > 37200 {
		t.Fatalf("2.44 budget reaches %d GLM tokens", tokens)
	}
	glm := ContextBudgetForPromptAt(DefaultPromptTokens, 420)
	if glm.MaximumBytes() != 251392 || glm.LowWaterBytes() != 125696 || glm.ToolResultBytes() != 31424 || glm.CheckpointBytes() != 16384 {
		t.Fatalf("GLM budget = %d/%d/%d/%d", glm.MaximumBytes(), glm.LowWaterBytes(), glm.ToolResultBytes(), glm.CheckpointBytes())
	}
	if tokens := (glm.MaximumBytes() + promptPrefixReserve) * 100 / 420; tokens != DefaultPromptTokens {
		t.Fatalf("4.2 budget reaches %d tokens", tokens)
	}
	if ContextBudgetForPromptAt(DefaultPromptTokens, 10) != ContextBudgetForPromptAt(DefaultPromptTokens, MinimumBytesPerToken) || ContextBudgetForPromptAt(DefaultPromptTokens, 5000) != ContextBudgetForPromptAt(DefaultPromptTokens, MaximumBytesPerToken) {
		t.Fatal("a ratio outside the range was not clamped")
	}
}
