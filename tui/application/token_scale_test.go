package application

import (
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

type memoryFactors struct {
	factors map[root.ModelID]float64
	loads   int
}

func (m *memoryFactors) TokenFactor(model root.ModelID) (float64, bool) {
	m.loads++
	factor, ok := m.factors[model]
	return factor, ok
}

func (m *memoryFactors) SaveTokenFactor(model root.ModelID, factor float64) error {
	m.factors[model] = factor
	return nil
}

// A launch starts from the factor earlier launches measured, and what it
// measures is saved for the next (20 November 2026: the first request of a
// resumed local session went out at 1.5 times its estimate).
func TestTokenScaleStartsFromTheFactorEarlierLaunchesMeasured(t *testing.T) {
	model, other := root.ModelID("local/qwen"), root.ModelID("local/other")
	memory := &memoryFactors{factors: map[root.ModelID]float64{model: 1.5, other: 9}}
	scale := &TokenScale{Memory: memory}
	if got := scale.Estimate(model, 60000); got != 90000 {
		t.Fatalf("estimate before any report = %d, want 90000", got)
	}
	if got := scale.Factor(other); got != tokenScaleMaximum {
		t.Fatalf("a saved factor out of range = %v", got)
	}
	scale.Factor(model)
	if memory.loads != 2 {
		t.Fatalf("the saved factor was read %d times, want once per model", memory.loads)
	}
	scale.Observe(model, 60000, 60000)
	if got := memory.factors[model]; got != 1.25 {
		t.Fatalf("saved factor = %v, want the blend 1.25", got)
	}
	if got := (&TokenScale{}).Factor(model); got != 1 {
		t.Fatalf("a scale without memory = %v", got)
	}
}
