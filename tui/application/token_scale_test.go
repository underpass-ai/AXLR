package application

import (
	"context"
	"strings"
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

// A learned factor that leaves no room for the request at all is not
// trusted: the request is fitted as counted and the provider's report
// corrects the factor. Saved across launches since 20 November 2026, one
// odd high ratio otherwise refused every request of a local model before
// any was sent, and nothing could lower it (21 November 2026).
func TestAFactorThatLeavesNoRoomIsNotTrusted(t *testing.T) {
	session := turnSession(t)
	if err := session.BeginTurn(root.Text(strings.Repeat("x", 3000)), turnTools()); err != nil {
		t.Fatal(err)
	}
	model := root.ModelID("test/model")
	memory := &memoryFactors{factors: map[root.ModelID]float64{model: 4}}
	sent := 0
	useCase := ContinueTurnUseCase{
		Store: &memoryStore{}, Successor: &successorStub{}, Tokens: &byteCounter{},
		LocalModel: func(root.ModelID) bool { return true },
		ContextCap: func(root.ModelID) int { return 6000 },
		Scale:      &TokenScale{Memory: memory},
		Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
			sent++
			result := assistant("done")
			result.Usage = &root.TokenUsage{PromptTokens: 3600}
			return result, nil
		}),
	}
	if err := useCase.Execute(context.Background(), &session, ignoreEvent); err != nil || sent != 1 {
		t.Fatalf("err=%v sent=%d", err, sent)
	}
	if got := memory.factors[model]; got >= 4 {
		t.Fatalf("the report did not lower the saved factor: %v", got)
	}
}
