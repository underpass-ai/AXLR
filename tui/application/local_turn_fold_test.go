package application

import (
	"context"
	"fmt"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// longTurn is a turn of steps one call each, every result a page long.
func longTurn(t *testing.T, prompt root.Text, steps int) (domain.Session, []root.ToolCallID) {
	t.Helper()
	session := turnSession(t)
	if err := session.BeginTurn(prompt, turnTools()); err != nil {
		t.Fatal(err)
	}
	ids := make([]root.ToolCallID, steps)
	for i := range ids {
		ids[i] = root.ToolCallID(fmt.Sprintf("step-%02d", i))
		if err := session.CompleteAssistantWithin(assistant("", call(ids[i], "read")), 100); err != nil {
			t.Fatal(err)
		}
		content := root.Text(fmt.Sprintf("result-%02d ", i) + strings.Repeat("page text ", 470))
		if err := session.RecordToolOutcome(ids[i], domain.DecisionApprove, domain.ToolOutcome{Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	return session, ids
}

func foldingUseCase(t *testing.T, cap int, check func(root.CompletionRequest)) (ContinueTurnUseCase, *memoryStore, *int) {
	store, counter, attempts := &memoryStore{}, &byteCounter{}, new(int)
	return ContinueTurnUseCase{
		Store: store, Successor: &successorStub{}, Tokens: counter, TurnToolCalls: 100,
		LocalModel: func(root.ModelID) bool { return true },
		ContextCap: func(root.ModelID) int { return cap },
		Windows:    windowsFunc(func(root.ModelID) domain.ContextWindow { return domain.ContextWindow(cap / 3 * 4) }),
		Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
			*attempts++
			if tokens := RequestTokens(counter, req.Messages[0], req.Tools, req.Messages[1:]); tokens > cap {
				t.Fatalf("sent %d tokens, input budget is %d", tokens, cap)
			}
			check(req)
			return assistant("done"), nil
		}),
	}, store, attempts
}

func TestLocalContextFoldsTheOldestStepsOfATurnThatOutgrowsTheWindow(t *testing.T) {
	const cap, steps = 8192, 60
	prompt := root.Text("Read every log and list its errors")
	session, ids := longTurn(t, prompt, steps)
	useCase, store, attempts := foldingUseCase(t, cap, func(req root.CompletionRequest) {
		if req.Messages[1].Content != prompt {
			t.Fatal("the turn's prompt was folded")
		}
		note := string(req.Messages[2].Content)
		// Each folded call keeps the start of its result (a bare call let a
		// model invent what it returned).
		if req.Messages[2].Role != root.RoleUser || !strings.HasPrefix(note, "[AXLR] Earlier steps of this turn") || !strings.Contains(note, "- read {} → ") || !strings.Contains(note, "result-00 page text") {
			t.Fatalf("no fold note after the prompt: %.300s", note)
		}
		last := req.Messages[len(req.Messages)-2:]
		if len(last[0].ToolCalls) != 1 || last[0].ToolCalls[0].ID != ids[steps-1] || last[1].ToolCallID != ids[steps-1] {
			t.Fatal("the latest step was not kept whole")
		}
		for _, message := range req.Messages[3:] {
			for _, sent := range message.ToolCalls {
				if strings.Contains(note, string(sent.ID)) {
					t.Fatal("a kept call is quoted in the note too")
				}
			}
		}
	})
	if err := useCase.Execute(context.Background(), &session, ignoreEvent); err != nil || *attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, *attempts)
	}
	saved := store.states[len(store.states)-1]
	if len(saved.Messages) != 2+2*steps || !strings.HasPrefix(string(saved.Messages[2].Content), "result-00 ") {
		t.Fatal("folding changed the stored transcript")
	}
}

func TestLocalContextResumesATurnStoppedAtTheWindowWhenThePersonSaysGoOn(t *testing.T) {
	const cap, steps = 8192, 60
	session, _ := longTurn(t, "Read every log and list its errors", steps)
	if err := session.InterruptDraft(""); err != nil {
		t.Fatal(err)
	}
	// "sigue" follows a tool result, so it reads as part of the same turn:
	// before 20 November 2026 it failed exactly as the turn had.
	if err := session.BeginTurn("sigue", turnTools()); err != nil {
		t.Fatal(err)
	}
	useCase, _, attempts := foldingUseCase(t, cap, func(req root.CompletionRequest) {
		last := req.Messages[len(req.Messages)-1]
		if last.Role != root.RoleUser || !strings.HasPrefix(string(last.Content), "sigue") {
			t.Fatalf("the person's message is not the last: %q", last.Content)
		}
		if !strings.Contains(string(req.Messages[2].Content), "[AXLR] Earlier steps") {
			t.Fatal("no fold note")
		}
	})
	if err := useCase.Execute(context.Background(), &session, ignoreEvent); err != nil || *attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, *attempts)
	}
}
