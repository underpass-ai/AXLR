package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type windowsFunc func(root.ModelID) domain.ContextWindow

func (f windowsFunc) ContextWindow(id root.ModelID) domain.ContextWindow { return f(id) }
func (f windowsFunc) ContextBudget(id root.ModelID) domain.ContextBudget {
	return domain.ContextBudgetForWindow(f(id))
}

func TestContinueTurnSizesTheProjectionToTheModelWindow(t *testing.T) {
	prompt := root.Text(strings.Repeat("palabra ", 4000)) // 32 KB
	run := func(windows ModelContextWindowPort) (bool, error) {
		s := turnSession(t)
		if err := s.BeginTurn(prompt, turnTools()); err != nil {
			t.Fatal(err)
		}
		called := false
		u := ContinueTurnUseCase{Store: &memoryStore{}, Windows: windows, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
			called = true
			return assistant("ok"), nil
		})}
		return called, u.Execute(context.Background(), &s, ignoreEvent)
	}
	if called, err := run(nil); err != nil || !called {
		t.Fatalf("default budget: called=%v err=%v", called, err)
	}
	var asked root.ModelID
	called, err := run(windowsFunc(func(id root.ModelID) domain.ContextWindow { asked = id; return 4096 }))
	if called || !errors.Is(err, ErrContextBudgetExceeded) || asked != "test/model" {
		t.Fatalf("4096-token window: called=%v err=%v asked=%q", called, err, asked)
	}
	if called, err := run(windowsFunc(func(root.ModelID) domain.ContextWindow { return 65536 })); err != nil || !called {
		t.Fatalf("65536-token window: called=%v err=%v", called, err)
	}
}
