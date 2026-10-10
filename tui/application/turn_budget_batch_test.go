package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026 claude-haiku-5.5 listed a directory and then sent 93
// local_read calls in one answer: all 93 were refused, none ran. Now the 31
// that fit run, the 62 past the budget are answered "over budget" and the
// turn pauses at the limit, without another model request.
func TestABatchOverTheBudgetRunsTheCallsThatFit(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	if err := s.BeginTurn("open every file", execTools()); err != nil {
		t.Fatal(err)
	}
	var requests []root.CompletionRequest
	executed := 0
	u := budgetAgent(store, 32, &requests, func(n int) int {
		if n == 1 {
			return 1
		}
		return 93
	})
	u.Tools = executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		executed++
		return domain.ToolOutcome{Content: "ok"}, nil
	})
	err := u.Execute(context.Background(), &s, ignoreEvent)
	if !errors.Is(err, domain.ErrToolCallLimit) || s.Status() != domain.StatusInterrupted || len(s.Pending()) != 0 {
		t.Fatalf("err %v, status %s, pending %d", err, s.Status(), len(s.Pending()))
	}
	if executed != 32 || len(requests) != 2 {
		t.Fatalf("executed %d, requests %d", executed, len(requests))
	}
	activity := s.Export().Activity
	refused := 0
	for i, record := range activity {
		over := strings.HasPrefix(string(record.Outcome.Content), domain.OverBudgetOutcomePrefix)
		if over != (i >= 32) {
			t.Fatalf("call %d: %+v", i, record)
		}
		if over {
			refused++
			if record.Decision != domain.DecisionDeny || !strings.Contains(string(record.Outcome.Content), "31 of this answer's 93 calls ran") {
				t.Fatalf("refusal %d: %s", i, record.Outcome.Content)
			}
		}
	}
	if refused != 62 || s.Export().TurnCallCount != 32 {
		t.Fatalf("refused %d, turn count %d", refused, s.Export().TurnCallCount)
	}
	if _, err := domain.RestoreSession(store.states[len(store.states)-1]); err != nil {
		t.Fatal(err)
	}
	// Ctrl+R starts a new budget and the model sees which calls did not run.
	if err := s.ResumeTurn(); err != nil {
		t.Fatal(err)
	}
	u.Continue.Models = streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		requests = append(requests, req)
		return assistant("read the rest"), nil
	})
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil || len(requests) != 3 || s.Status() != domain.StatusComplete {
		t.Fatalf("resume: %v, requests %d, status %s", err, len(requests), s.Status())
	}
}

// The model is told the per-turn budget up front, in words that never change
// within a console, so the prompt cache keeps the system prompt.
func TestTheModelIsToldTheTurnBudgetUpFront(t *testing.T) {
	var prompts []root.Text
	s := turnSession(t)
	if err := s.BeginTurn("go", execTools()); err != nil {
		t.Fatal(err)
	}
	u := ContinueTurnUseCase{Store: &memoryStore{}, TurnToolCalls: 48, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		prompts = append(prompts, req.Messages[0].Content)
		return assistant("", call(root.ToolCallID(fmt.Sprintf("c%d", len(prompts))), "exec")), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompts[0]), "at most 48 tool calls") {
		t.Fatalf("budget not stated: %s", prompts[0])
	}
}
