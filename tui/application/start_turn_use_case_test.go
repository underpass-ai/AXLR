package application

import (
	"context"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"testing"
)

type streamFunc func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error)

func (f streamFunc) Stream(c context.Context, r root.CompletionRequest, e func(root.Text) error) (root.CompletionResult, error) {
	return f(c, r, e)
}

type catalogStub struct {
	calls int
	err   error
}

func (c *catalogStub) Snapshot(context.Context) ([]domain.AvailableTool, error) {
	c.calls++
	return turnTools(), c.err
}

type memoryStore struct {
	states []domain.SessionState
	err    error
}

func (s *memoryStore) Save(ctx context.Context, v domain.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	s.states = append(s.states, v.Export())
	return nil
}
func (s *memoryStore) Load(context.Context, domain.SessionID) (domain.Session, error) {
	return domain.Session{}, errors.New("not used")
}
func turnTools() []domain.AvailableTool {
	schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
	id, _ := domain.NewLocalToolIdentity("read")
	return []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "read", Parameters: schema}, Identity: id}}
}
func turnSession(t *testing.T) domain.Session {
	t.Helper()
	s, e := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "test/model")
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func assistant(text root.Text, calls ...root.ToolCall) root.CompletionResult {
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: text, ToolCalls: calls}}
}
func call(id root.ToolCallID, name root.ToolName) root.ToolCall {
	args, _ := root.NewJSONObject([]byte(`{}`))
	return root.ToolCall{ID: id, Name: name, Arguments: args}
}
func ignoreEvent(Event) error { return nil }

func TestStartTurnPersistsUserBeforeStreamingAndAssistantAfter(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	catalog := &catalogStub{}
	var deltas []root.Text
	model := streamFunc(func(ctx context.Context, r root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
		if len(store.states) != 1 || store.states[0].Messages[0].Content != "hello" {
			t.Fatal("user not saved before model")
		}
		if r.Model != "test/model" || len(r.Tools) != 1 || r.Tools[0].Name != "read" {
			t.Fatalf("request: %+v", r)
		}
		for _, d := range []root.Text{"hi", " there"} {
			if err := emit(d); err != nil {
				return root.CompletionResult{}, err
			}
			if len(deltas) == 0 || deltas[len(deltas)-1] != d {
				t.Fatal("delta not delivered synchronously")
			}
		}
		if len(s.Messages()) != 1 || len(s.Pending()) != 0 {
			t.Fatal("stream callback changed stable history")
		}
		return assistant("hi there"), nil
	})
	u := StartTurnUseCase{Catalog: catalog, Store: store, Continue: ContinueTurnUseCase{Models: model, Store: store}}
	if err := u.Execute(context.Background(), &s, "hello", func(e Event) error {
		if e.Kind == EventTextDelta {
			deltas = append(deltas, e.Text)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if catalog.calls != 1 || len(deltas) != 2 || len(store.states) != 2 || s.Status() != domain.StatusComplete || s.Messages()[1].Content != "hi there" {
		t.Fatalf("state: %+v, saves %d", s.Export(), len(store.states))
	}
}
func TestStartTurnFailuresDoNotCallModel(t *testing.T) {
	for _, failure := range []string{"catalog", "store"} {
		t.Run(failure, func(t *testing.T) {
			s := turnSession(t)
			catalog := &catalogStub{}
			store := &memoryStore{}
			boom := errors.New("failed")
			if failure == "catalog" {
				catalog.err = boom
			} else {
				store.err = boom
			}
			u := StartTurnUseCase{Catalog: catalog, Store: store, Continue: ContinueTurnUseCase{Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
				t.Fatal("model called after failure")
				return assistant("bad"), nil
			}), Store: store}}
			if !errors.Is(u.Execute(context.Background(), &s, "hello", ignoreEvent), boom) {
				t.Fatal("failure lost")
			}
			if len(s.Messages()) != 0 {
				t.Fatal("failed submission changed session")
			}
		})
	}
}
