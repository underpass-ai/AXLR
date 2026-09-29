package application

import (
	"context"
	"errors"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"testing"
)

func TestContinueTurnQueuesCompleteAssistantBeforeToolActivity(t *testing.T) {
	s := turnSession(t)
	_ = s.BeginTurn("read", turnTools())
	store := &memoryStore{}
	u := ContinueTurnUseCase{Store: store, Models: streamFunc(func(_ context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
		if err := emit("checking"); err != nil {
			return root.CompletionResult{}, err
		}
		if len(s.Pending()) != 0 {
			t.Fatal("tools queued inside stream callback")
		}
		return assistant("checking", call("model-call-1", "read"), call("model-call-2", "read")), nil
	})}
	var ids []root.ToolCallID
	if err := u.Execute(context.Background(), &s, func(e Event) error {
		if e.Kind == EventToolActivity {
			if len(store.states) == 0 || len(s.Messages()) != 2 || len(s.Messages()[1].ToolCalls) != 2 {
				t.Fatal("tool event precedes complete persisted assistant")
			}
			ids = append(ids, e.Tool.Call.ID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if s.Status() != domain.StatusApproval || len(ids) != 2 || ids[0] != "model-call-1" || ids[1] != "model-call-2" {
		t.Fatalf("activity: %v", ids)
	}
	// Continue after human outcomes uses the same saved definitions, without discovery.
	for _, p := range s.Pending() {
		if err := s.RecordToolOutcome(p.Call.ID, domain.DecisionDeny, domain.ToolOutcome{Content: "denied"}); err != nil {
			t.Fatal(err)
		}
	}
	u.Models = streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		if len(r.Tools) != 1 || r.Tools[0].Name != "read" || len(r.Messages) != 4 || r.Messages[2].ToolCallID != "model-call-1" || r.Messages[3].ToolCallID != "model-call-2" {
			t.Fatalf("continued request: %+v", r)
		}
		return assistant("done"), nil
	})
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
}
func TestContinueTurnRejectsUnknownWithoutDiscardingValidCall(t *testing.T) {
	s := turnSession(t)
	_ = s.BeginTurn("go", turnTools())
	store := &memoryStore{}
	u := ContinueTurnUseCase{Store: store, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("checking", call("unknown-id", "unknown"), call("valid-id", "read")), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	m := s.Messages()
	if len(m) != 3 || m[1].Role != root.RoleAssistant || m[2].ToolCallID != "unknown-id" || len(s.Pending()) != 1 || s.Pending()[0].Call.ID != "valid-id" || !s.Export().Activity[0].Outcome.IsError {
		t.Fatalf("state: %+v", s.Export())
	}
	if len(store.states) != 2 {
		t.Fatalf("saves: %d", len(store.states))
	}
}
func TestContinueTurnInterruptedDraft(t *testing.T) {
	for _, kind := range []string{"provider", "cancel", "callback"} {
		t.Run(kind, func(t *testing.T) {
			s := turnSession(t)
			_ = s.BeginTurn("go", turnTools())
			store := &memoryStore{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			boom := errors.New("stream failed")
			u := ContinueTurnUseCase{Store: store, Models: streamFunc(func(_ context.Context, _ root.CompletionRequest, e func(root.Text) error) (root.CompletionResult, error) {
				if err := e("partial"); err != nil {
					return root.CompletionResult{}, err
				}
				if kind == "cancel" {
					cancel()
					return root.CompletionResult{}, ctx.Err()
				}
				return root.CompletionResult{}, boom
			})}
			emit := ignoreEvent
			if kind == "callback" {
				emit = func(e Event) error {
					if e.Kind == EventTextDelta {
						return boom
					}
					return nil
				}
			}
			err := u.Execute(ctx, &s, emit)
			if err == nil || s.Status() != domain.StatusInterrupted || s.Export().Draft != "partial" || len(s.Messages()) != 1 || len(store.states) != 1 {
				t.Fatalf("err %v state %+v saves %d", err, s.Export(), len(store.states))
			}
		})
	}
}
func TestContinueTurnRejectsInvalidStateAndMalformedCompletion(t *testing.T) {
	s := turnSession(t)
	u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("bad", root.ToolCall{ID: "id", Name: "read"}), nil
	})}
	if u.Execute(context.Background(), &s, ignoreEvent) == nil {
		t.Fatal("idle accepted")
	}
	_ = s.BeginTurn("go", turnTools())
	if u.Execute(context.Background(), &s, ignoreEvent) == nil || s.Status() != domain.StatusInterrupted || len(s.Messages()) != 1 {
		t.Fatalf("malformed accepted: %+v", s.Export())
	}
}

func TestContinueTurnDefersUnknownBehindValidCall(t *testing.T) {
	s := turnSession(t)
	_ = s.BeginTurn("go", turnTools())
	u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("checking", call("valid", "read"), call("unknown", "not-advertised")), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if len(s.Messages()) != 2 || len(s.Pending()) != 2 || s.Pending()[0].Call.ID != "valid" || s.Pending()[1].Call.ID != "unknown" {
		t.Fatalf("order changed: %+v", s.Export())
	}
	// No execution dependency exists in this use case. Neither call can execute,
	// and continuing the model is blocked until ordered outcomes are recorded.
	if err := u.Execute(context.Background(), &s, ignoreEvent); err == nil {
		t.Fatal("continued with unresolved calls")
	}
}
func TestContinueTurnCallLimitPersistsPause(t *testing.T) {
	s := turnSession(t)
	_ = s.BeginTurn("go", turnTools())
	store := &memoryStore{}
	calls := make([]root.ToolCall, domain.MaxTurnToolCalls+1)
	for i := range calls {
		calls[i] = call(root.ToolCallID(fmt.Sprintf("call-%d", i)), "read")
	}
	u := ContinueTurnUseCase{Store: store, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("", calls...), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); !errors.Is(err, domain.ErrToolCallLimit) {
		t.Fatalf("limit error: %v", err)
	}
	if s.Status() != domain.StatusInterrupted || len(s.Pending()) != 0 || len(store.states) != 1 {
		t.Fatalf("limit state: %+v", s.Export())
	}
}
func TestContinueTurnUnknownOnlyReturnsAfterOneStream(t *testing.T) {
	s := turnSession(t)
	_ = s.BeginTurn("go", turnTools())
	store := &memoryStore{}
	requests := 0
	u := ContinueTurnUseCase{Store: store, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		requests++
		return assistant("", call("unknown", "not-advertised")), nil
	})}
	if err := u.Execute(context.Background(), &s, nil); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(s.Pending()) != 0 || s.Status() != domain.StatusStreaming || len(s.Messages()) != 3 {
		t.Fatalf("state: %+v requests %d", s.Export(), requests)
	}
}
func TestContinueTurnSaveFailurePreservesLastStableSession(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(fmt.Sprint(interrupted), func(t *testing.T) {
			s := turnSession(t)
			_ = s.BeginTurn("go", turnTools())
			boom := errors.New("disk full")
			store := &memoryStore{err: boom}
			u := ContinueTurnUseCase{Store: store, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
				if interrupted {
					return root.CompletionResult{}, errors.New("network")
				}
				return assistant("done"), nil
			})}
			if err := u.Execute(context.Background(), &s, ignoreEvent); !errors.Is(err, boom) {
				t.Fatalf("save failure lost: %v", err)
			}
			if len(s.Messages()) != 1 || s.Status() != domain.StatusStreaming {
				t.Fatalf("unsaved state published: %+v", s.Export())
			}
		})
	}
}
