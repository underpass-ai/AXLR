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

// execTools is a session surface whose one tool runs through the tool port
// as it is, unlike local_read, which the resolver pages.
func execTools() []domain.AvailableTool {
	schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
	id, _ := domain.NewLocalToolIdentity("exec")
	return []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "exec", Parameters: schema}, Identity: id}}
}

// budgetAgent auto-approves and runs every call; each request answers with
// calls(request number) exec calls, or a final answer when it returns 0.
func budgetAgent(store *memoryStore, limit int, requests *[]root.CompletionRequest, calls func(int) int) AgentTurnUseCase {
	model := streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		*requests = append(*requests, req)
		n := calls(len(*requests))
		toolCalls := make([]root.ToolCall, n)
		for i := range toolCalls {
			toolCalls[i] = call(root.ToolCallID(fmt.Sprintf("r%dc%d", len(*requests), i)), "exec")
		}
		if n == 0 {
			return assistant("done"), nil
		}
		return assistant("", toolCalls...), nil
	})
	return AgentTurnUseCase{
		Continue: ContinueTurnUseCase{Store: store, Models: model, TurnToolCalls: limit, Validation: argumentValidationFunc(func(root.ToolDefinition, root.JSONValue) error { return nil })},
		Approval: approvalFunc(func(domain.ToolIdentity) bool { return true }),
		Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
			return domain.ToolOutcome{Content: "ok"}, nil
		}),
	}
}

func budgetNotes(messages []root.Message) []string {
	var notes []string
	for _, message := range messages {
		if message.Role == root.RoleUser && domain.BudgetNote(message.Content) {
			notes = append(notes, string(message.Content))
		}
	}
	return notes
}

// With a fifth of the budget left the model is told once, in a note that
// is never rewritten, and the budget goes on: the note is not the person's
// word, which would restart it.
func TestTheModelIsWarnedOnceBeforeTheCallLimit(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	if err := s.BeginTurn("go", execTools()); err != nil {
		t.Fatal(err)
	}
	var requests []root.CompletionRequest
	u := budgetAgent(store, 10, &requests, func(n int) int {
		if n <= 9 {
			return 1
		}
		return 0
	})
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	notes := budgetNotes(s.Messages())
	want := domain.BudgetNotePrefix + " 2 tool calls left in this turn. Finish the task with what you have, or stop and tell the user what remains; they can continue with a new budget."
	if len(notes) != 1 || notes[0] != want {
		t.Fatalf("notes = %q", notes)
	}
	// The ninth request, with 8 of 10 calls made, was the first to carry it,
	// as its last message; the tenth kept it where it was.
	for i, req := range requests {
		last := req.Messages[len(req.Messages)-1]
		if noted := domain.BudgetNote(last.Content); noted != (i == 8) {
			t.Fatalf("request %d ends with %q", i+1, last.Content)
		}
	}
	if count := s.Export().TurnCallCount; count != 9 || s.Status() != domain.StatusComplete {
		t.Fatalf("turn count %d, status %s", count, s.Status())
	}
	// The saved transcript replays with the note inside the turn.
	if _, err := domain.RestoreSession(store.states[len(store.states)-1]); err != nil {
		t.Fatal(err)
	}
}

// A console-driven step that reaches the limit continues with a new budget
// twice, telling the model each time; the third time the person decides.
func TestACeremonyStepContinuesTwiceAtTheCallLimit(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "i", Step: "repair", Iteration: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("fix it", append(execTools(), HostTools()...)); err != nil {
		t.Fatal(err)
	}
	var requests []root.CompletionRequest
	u := budgetAgent(store, 8, &requests, func(int) int { return 3 })
	err := u.Execute(context.Background(), &s, ignoreEvent)
	if !errors.Is(err, domain.ErrToolCallLimit) || s.Status() != domain.StatusInterrupted {
		t.Fatalf("err = %v, status %s", err, s.Status())
	}
	var continued []string
	for _, note := range budgetNotes(s.Messages()) {
		if strings.Contains(note, "the console continued it with a new budget") {
			continued = append(continued, note)
		}
	}
	if len(continued) != 2 || !strings.Contains(continued[0], "reached its 8 tool-call limit") || !strings.Contains(continued[1], "(2 of 2)") || !strings.Contains(continued[1], "axlr_step_done") {
		t.Fatalf("continued = %q", continued)
	}
	if run, _ := s.Ceremony(); run.LimitStep != "repair#1" || run.LimitResumes != 2 {
		t.Fatalf("run = %+v", run)
	}
	// Three budgets of three requests each, the last one cut at its limit.
	if len(requests) != 9 {
		t.Fatalf("requests = %d", len(requests))
	}
	if _, err := domain.RestoreSession(store.states[len(store.states)-1]); err != nil {
		t.Fatal(err)
	}
}

// Outside a ceremony the person decides at the limit, as before.
func TestAnOrdinaryTurnStillPausesAtTheCallLimit(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("go", execTools()); err != nil {
		t.Fatal(err)
	}
	var requests []root.CompletionRequest
	u := budgetAgent(&memoryStore{}, 8, &requests, func(int) int { return 3 })
	if err := u.Execute(context.Background(), &s, ignoreEvent); !errors.Is(err, domain.ErrToolCallLimit) || len(requests) != 3 {
		t.Fatalf("err = %v after %d requests", err, len(requests))
	}
}
