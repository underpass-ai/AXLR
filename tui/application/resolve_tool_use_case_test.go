package application

import (
	"context"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"testing"
)

type executionFunc func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error)

func (f executionFunc) Execute(c context.Context, i domain.ToolIdentity, a root.JSONValue) (domain.ToolOutcome, error) {
	return f(c, i, a)
}
func queued(t *testing.T, calls ...root.ToolCall) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn("go", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", calls...)); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestResolveToolOrderedDecisionsAndContinuation(t *testing.T) {
	s := queued(t, call("a", "read"), call("bad", "unknown"), call("b", "read"))
	store := &memoryStore{}
	executions := 0
	u := ResolveToolUseCase{Store: store, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		executions++
		checkpoint := store.states[len(store.states)-1]
		if checkpoint.Activity[0].Outcome == nil || !checkpoint.Activity[0].Outcome.Uncertain || checkpoint.Status != domain.StatusInterrupted {
			t.Fatal("missing durable execution checkpoint")
		}
		return domain.ToolOutcome{Content: "ok"}, nil
	}), Continue: ContinueTurnUseCase{Store: store, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		if len(r.Tools) != 4 || len(r.Messages) != 6 {
			t.Fatalf("request %+v", r)
		}
		for i, id := range []root.ToolCallID{"a", "bad", "b"} {
			if r.Messages[i+3].ToolCallID != id {
				t.Fatal("lost correlation")
			}
		}
		return assistant("done"), nil
	})}}
	for _, id := range []root.ToolCallID{"missing", "b"} {
		if u.Execute(context.Background(), &s, id, domain.DecisionApprove, nil) == nil {
			t.Fatal("invalid decision accepted")
		}
	}
	if executions != 0 {
		t.Fatal("execution before approval")
	}
	if err := u.Execute(context.Background(), &s, "a", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Pending()) != 1 || s.Pending()[0].Call.ID != "b" || !s.Export().Activity[1].Outcome.IsError {
		t.Fatalf("unknown not rejected %+v", s.Export())
	}
	if u.Execute(context.Background(), &s, "a", domain.DecisionApprove, nil) == nil {
		t.Fatal("duplicate accepted")
	}
	if err := u.Execute(context.Background(), &s, "b", domain.DecisionDeny, nil); err != nil {
		t.Fatal(err)
	}
	if executions != 1 || s.Status() != domain.StatusComplete {
		t.Fatalf("executions %d status %s", executions, s.Status())
	}
}
func TestResolveToolExecutionOutcomes(t *testing.T) {
	for _, kind := range []string{"mcp-error", "timeout", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			s := queued(t, call("a", "read"), call("b", "read"))
			store := &memoryStore{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			u := ResolveToolUseCase{Store: store, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
				if kind == "cancel" {
					cancel()
					return domain.ToolOutcome{}, context.Canceled
				}
				if kind == "timeout" {
					return domain.ToolOutcome{}, context.DeadlineExceeded
				}
				return domain.ToolOutcome{Content: "MCP failure", IsError: true}, nil
			})}
			err := u.Execute(ctx, &s, "a", domain.DecisionApprove, nil)
			a := s.Export().Activity[0]
			if a.Outcome == nil || !a.Outcome.IsError {
				t.Fatal("missing error outcome")
			}
			if kind == "mcp-error" {
				if err != nil || a.Outcome.Uncertain || len(s.Pending()) != 1 {
					t.Fatalf("MCP outcome %+v %v", a, err)
				}
				return
			}
			if err == nil || !a.Outcome.Uncertain || s.Status() != domain.StatusInterrupted {
				t.Fatalf("uncertain outcome %+v %v", s.Export(), err)
			}
			restored, e := domain.RestoreSession(store.states[len(store.states)-1])
			if e != nil {
				t.Fatal(e)
			}
			if kind == "cancel" && len(restored.Pending()) != 0 {
				t.Fatal("queued call not cancelled")
			}
			if u.Execute(context.Background(), &restored, "a", domain.DecisionApprove, nil) == nil {
				t.Fatal("retried after resume")
			}
		})
	}
}
func TestResolveToolSaveFailuresPreventRetry(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			s := queued(t, call("a", "read"))
			store := &memoryStore{}
			boom := errors.New("disk full")
			n := 0
			if !after {
				store.err = boom
			}
			u := ResolveToolUseCase{Store: store, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
				n++
				store.err = boom
				return domain.ToolOutcome{Content: "done"}, nil
			})}
			if !errors.Is(u.Execute(context.Background(), &s, "a", domain.DecisionApprove, nil), boom) {
				t.Fatal("save error lost")
			}
			if after {
				if n != 1 || len(s.Pending()) != 0 || !s.Export().Activity[0].Outcome.Uncertain {
					t.Fatal("execution can retry")
				}
				restored, err := domain.RestoreSession(store.states[0])
				if err != nil || len(restored.Pending()) != 0 {
					t.Fatalf("checkpoint invalid %v", err)
				}
			} else if n != 0 {
				t.Fatal("executed without checkpoint")
			}
		})
	}
}
func TestResolveToolAllUnknownAutomaticallyContinues(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	n := 0
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Store: store, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		n++
		if n == 1 {
			return assistant("", call("bad", "unknown")), nil
		}
		if len(r.Tools) != 4 || len(r.Messages) != 4 || r.Messages[3].ToolCallID != "bad" {
			t.Fatalf("request %+v", r)
		}
		return assistant("done"), nil
	})}}
	if err := u.Execute(context.Background(), &s, "go", nil); err != nil {
		t.Fatal(err)
	}
	if n != 2 || s.Status() != domain.StatusComplete {
		t.Fatalf("stalled: %d %s", n, s.Status())
	}
}
func TestResolveToolAgentLoopCallCap(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	n := 0
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Store: store, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		n++
		return assistant("", call(root.ToolCallID(fmt.Sprintf("bad-%d", n)), "unknown")), nil
	})}}
	err := u.Execute(context.Background(), &s, "go", nil)
	if !errors.Is(err, domain.ErrToolCallLimit) || n != 33 || s.Export().TurnCallCount != 32 || s.Status() != domain.StatusInterrupted {
		t.Fatalf("cap %d %v %+v", n, err, s.Export())
	}
}

func TestResolveToolCheckpointRestoreOnlyResumesUnexecutedCalls(t *testing.T) {
	s := queued(t, call("a", "read"), call("b", "read"))
	store := &memoryStore{}
	executions := 0
	u := ResolveToolUseCase{Store: store, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		executions++
		restored, err := domain.RestoreSession(store.states[0])
		if err != nil {
			t.Fatal(err)
		}
		if err = restored.ResumePending(); err != nil {
			t.Fatal(err)
		}
		if len(restored.Pending()) != 1 || restored.Pending()[0].Call.ID != "b" {
			t.Fatal("started call reopened")
		}
		return domain.ToolOutcome{Content: "ok"}, nil
	})}
	if err := u.Execute(context.Background(), &s, "a", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	if executions != 1 {
		t.Fatal("wrong execution count")
	}
}
func TestResolveToolUnknownNeverExecutes(t *testing.T) {
	s := queued(t, call("bad", "unknown"))
	store := &memoryStore{}
	u := ResolveToolUseCase{Store: store, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		t.Fatal("unknown executed")
		return domain.ToolOutcome{}, nil
	}), Continue: ContinueTurnUseCase{Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		if r.Messages[3].ToolCallID != "bad" {
			t.Fatal("missing rejection")
		}
		return assistant("done"), nil
	})}}
	if err := u.Execute(context.Background(), &s, "bad", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	if s.Export().Activity[0].Decision != domain.DecisionDeny {
		t.Fatal("unknown approved")
	}
}

func TestResolveToolUncertainAdapterOutcomePauses(t *testing.T) {
	s := queued(t, call("a", "read"))
	store := &memoryStore{}
	u := ResolveToolUseCase{Store: store, Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		return domain.ToolOutcome{Content: "timed out", IsError: true, Uncertain: true}, nil
	}), Continue: ContinueTurnUseCase{Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		t.Fatal("continued uncertain execution")
		return assistant("bad"), nil
	})}}
	if err := u.Execute(context.Background(), &s, "a", domain.DecisionApprove, nil); err == nil {
		t.Fatal("uncertainty must surface")
	}
	if s.Status() != domain.StatusInterrupted || s.Export().Activity[0].Outcome.Content != "timed out" {
		t.Fatal("lost uncertain adapter result")
	}
}
