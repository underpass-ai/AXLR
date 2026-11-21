package application

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/adapters/provider"
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

// A stale factor that the server then refuses as too long is corrected by
// the server's own count, and the retry is sized by it: the retry was
// compacted under the stale factor again, could not fit, and stopped the
// turn, every turn, since a refusal never corrected the factor (review,
// 21 November 2026).
func TestAServerRefusalSizesTheRetryAndCorrectsAStaleFactor(t *testing.T) {
	session := turnSession(t)
	if err := session.BeginTurn("Keep the original tests", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteAssistant(assistant(root.Text("Decided to keep parser behavior. " + strings.Repeat("detail ", 1700)))); err != nil {
		t.Fatal(err)
	}
	if err := session.BeginTurn(root.Text(strings.Repeat("y", 2000)), turnTools()); err != nil {
		t.Fatal(err)
	}
	model := session.Export().Model
	memory := &memoryFactors{factors: map[root.ModelID]float64{model: 4}}
	counter := &byteCounter{}
	attempts := 0
	useCase := ContinueTurnUseCase{
		Store: &memoryStore{}, Successor: &successorStub{}, Tokens: counter,
		LocalModel: func(root.ModelID) bool { return true },
		ContextCap: func(root.ModelID) int { return 6000 },
		Scale:      &TokenScale{Memory: memory},
		Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
			attempts++
			served := RequestTokens(counter, req.Messages[0], req.Tools, req.Messages[1:]) * 3 / 2
			if served > 7000 {
				return root.CompletionResult{}, &provider.ProviderError{StatusCode: 400, Message: fmt.Sprintf("request (%d tokens) exceeds the available context size (7000 tokens), try increasing it", served)}
			}
			result := assistant("done")
			result.Usage = &root.TokenUsage{PromptTokens: served}
			return result, nil
		}),
	}
	if err := useCase.Execute(context.Background(), &session, ignoreEvent); err != nil || attempts != 2 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
	if got := memory.factors[model]; got >= 3 {
		t.Fatalf("the refusal and the report left the factor at %v", got)
	}
}

// blockingFactors is a memory whose save waits until released.
type blockingFactors struct {
	saving, release chan struct{}
}

func (b *blockingFactors) TokenFactor(root.ModelID) (float64, bool) { return 0, false }

func (b *blockingFactors) SaveTokenFactor(root.ModelID, float64) error {
	b.saving <- struct{}{}
	<-b.release
	return nil
}

// Saving a factor (a cross-process file lock, a write and an fsync) does
// not hold the scale: another model's estimate, another session's in
// axlr-serve, does not wait for it (21 November 2026).
func TestSavingAFactorDoesNotBlockAnotherModelsEstimate(t *testing.T) {
	memory := &blockingFactors{saving: make(chan struct{}), release: make(chan struct{})}
	scale := &TokenScale{Memory: memory}
	scale.Factor("local/other") // loaded before the save starts
	go scale.Observe("local/qwen", 1000, 1500)
	<-memory.saving
	done := make(chan int)
	go func() { done <- scale.Estimate("local/other", 1000) }()
	select {
	case got := <-done:
		if got != 1000 {
			t.Fatalf("estimate = %d", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("an estimate waited for another model's save")
	}
	if got := scale.Factor("local/qwen"); got != 1.5 {
		t.Fatalf("the learned factor is not in force while it saves: %v", got)
	}
	close(memory.release)
}
