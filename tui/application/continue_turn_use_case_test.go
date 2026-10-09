package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
		if len(r.Tools) != 9 || !requestHasTool(r, "read") || len(r.Messages) != 5 || r.Messages[0].Role != root.RoleSystem || r.Messages[3].ToolCallID != "model-call-1" || r.Messages[4].ToolCallID != "model-call-2" {
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
	s := sessionAtCallLimit(t)
	store := &memoryStore{}
	calls := []root.ToolCall{call("over-0", "read"), call("over-1", "read")}
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

// sessionAtCallLimit is a turn that already made all of its calls: an
// answer with calls is refused whole. (With some left, the calls that fit
// run; see turn_budget_batch_test.go.)
func sessionAtCallLimit(t *testing.T) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn("go", turnTools()); err != nil {
		t.Fatal(err)
	}
	calls := make([]root.ToolCall, domain.MaxTurnToolCalls)
	for i := range calls {
		calls[i] = call(root.ToolCallID(fmt.Sprintf("c%d", i)), "read")
	}
	if err := s.CompleteAssistant(assistant("", calls...)); err != nil {
		t.Fatal(err)
	}
	for _, c := range calls {
		if err := s.RecordToolOutcome(c.ID, domain.DecisionApprove, domain.ToolOutcome{Content: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// The answer that crosses the call limit was streamed to the person: it is
// kept, its calls are answered as not run, and the turn pauses.
func TestTheAnswerAtTheCallLimitIsKeptWithItsCallsNotRun(t *testing.T) {
	s := sessionAtCallLimit(t)
	before := len(s.Messages())
	store := &memoryStore{}
	const text = "I found the cause; reading two more files"
	u := ContinueTurnUseCase{Store: store, Models: streamFunc(func(_ context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
		_ = emit(text)
		return assistant(text, call("x1", "read"), call("x2", "read")), nil
	})}
	shown := 0
	record := func(e Event) error {
		if e.Kind == EventSession && e.Snapshot != nil {
			shown = len(e.Snapshot.Messages)
		}
		return nil
	}
	if err := u.Execute(context.Background(), &s, record); !errors.Is(err, domain.ErrToolCallLimit) {
		t.Fatalf("limit error: %v", err)
	}
	messages := s.Messages()
	if s.Status() != domain.StatusInterrupted || len(s.Pending()) != 0 || len(messages) != before+3 || messages[before].Content != text || len(messages[before].ToolCalls) != 2 {
		t.Fatalf("the streamed answer was dropped: %s %+v", s.Status(), messages[before:])
	}
	if shown != before+3 {
		t.Fatalf("the transcript shown has %d messages, not the kept answer's %d", shown, before+3)
	}
	for _, record := range s.Export().Activity[len(s.Export().Activity)-2:] {
		if record.Decision != domain.DecisionDeny || !record.Outcome.IsError || !strings.HasPrefix(string(record.Outcome.Content), domain.OverBudgetOutcomePrefix+" of 32 calls: none of this answer's 2 calls ran") {
			t.Fatalf("call over the limit: %+v", record)
		}
	}
	saved := store.states[len(store.states)-1]
	if _, err := domain.RestoreSession(saved); err != nil || len(saved.Messages) != before+3 {
		t.Fatalf("the kept answer was not saved: %v", err)
	}
}

// Resuming is the person's word to go on, like a steered message: the retry
// gets a new call budget instead of tripping the limit on every answer with
// two calls (Ctrl+R, and the plan and repair loops that continue a turn).
func TestResumingAfterTheCallLimitRestartsTheTurnBudget(t *testing.T) {
	s := sessionAtCallLimit(t)
	requests := 0
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		requests++
		return assistant("", call(root.ToolCallID(fmt.Sprintf("x%d", requests)), "read"), call(root.ToolCallID(fmt.Sprintf("y%d", requests)), "read")), nil
	})}}
	if err := u.Execute(context.Background(), &s, ignoreEvent); !errors.Is(err, domain.ErrToolCallLimit) {
		t.Fatalf("limit error: %v", err)
	}
	if err := s.ResumeTurn(); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatalf("the retry tripped the limit again: %v", err)
	}
	if requests != 2 || s.Status() != domain.StatusApproval || len(s.Pending()) != 2 {
		t.Fatalf("requests %d, status %s, pending %d", requests, s.Status(), len(s.Pending()))
	}
	if _, err := domain.RestoreSession(s.Export()); err != nil {
		t.Fatalf("a resumed turn no longer restores: %v", err)
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

func requestHasTool(r root.CompletionRequest, name root.ToolName) bool {
	for _, tool := range r.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func TestContinueTurnIncludesInstalledAXLRSkillIndex(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("use the package", turnTools()); err != nil {
		t.Fatal(err)
	}
	u := ContinueTurnUseCase{Store: &memoryStore{}, PluginGuidance: func(context.Context) (string, error) { return "\nInstalled AXLR plugin skill: sample/example\n", nil }, Models: streamFunc(func(_ context.Context, request root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		if len(request.Messages) == 0 || request.Messages[0].Role != root.RoleSystem || !strings.Contains(string(request.Messages[0].Content), "sample/example") {
			t.Fatalf("package skill index missing: %+v", request.Messages)
		}
		return assistant("done"), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
}
