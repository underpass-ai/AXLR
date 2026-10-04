package application

import (
	"context"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"testing"
)

func lifecyclePair(t *testing.T, trace *diagnosticCollector, action DiagnosticAction, parent uint64, class DiagnosticErrorClass) (DiagnosticEvent, DiagnosticEvent) {
	t.Helper()
	var starts, ends []DiagnosticEvent
	for _, e := range trace.events {
		if e.Action == action {
			if e.Stage == DiagnosticActionStart {
				starts = append(starts, e)
			}
			if e.Stage == DiagnosticActionEnd {
				ends = append(ends, e)
			}
		}
	}
	if len(starts) != 1 || len(ends) != 1 {
		t.Fatalf("%s lifecycle is not balanced: starts=%v ends=%v", action, starts, ends)
	}
	a, b := starts[0], ends[0]
	if a.SpanID == 0 || a.SpanID != b.SpanID || a.ParentSpanID != parent || b.ParentSpanID != parent || b.ErrorClass != class || b.ElapsedMicroseconds < 0 {
		t.Fatalf("%s correlation/error lost: %+v %+v", action, a, b)
	}
	return a, b
}

func TestContinueLifecycleCorrelatesModelAndContextIncludingStreamFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		class DiagnosticErrorClass
	}{
		{"success", nil, DiagnosticErrorNone}, {"provider", errors.New("private provider error"), DiagnosticErrorProvider}, {"cancel", context.Canceled, DiagnosticErrorCancelled}, {"timeout", context.DeadlineExceeded, DiagnosticErrorTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace := &diagnosticCollector{}
			ctx, parent := StartDiagnosticSpan(context.Background(), trace, DiagnosticActionOperation, DiagnosticEvent{})
			parentID := CurrentDiagnosticSpan(ctx)
			s := turnSession(t)
			_ = s.BeginTurn("go", turnTools())
			var modelID uint64
			u := ContinueTurnUseCase{Diagnostics: trace, Store: &memoryStore{}, Models: streamFunc(func(c context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
				modelID = CurrentDiagnosticSpan(c)
				if modelID == 0 || modelID == parentID {
					t.Fatal("model port did not receive its child span")
				}
				if err := emit("one"); err != nil {
					return root.CompletionResult{}, err
				}
				return assistant("one"), tc.err
			})}
			err := u.Execute(ctx, &s, nil)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error changed: %v", err)
			}
			parent.End(DiagnosticErrorNone)
			lifecyclePair(t, trace, DiagnosticActionContext, parentID, DiagnosticErrorNone)
			start, _ := lifecyclePair(t, trace, DiagnosticActionModel, parentID, tc.class)
			if start.SpanID != modelID || start.Messages != 2 || start.Tools != 6 {
				t.Fatal("model measurements or port context do not identify request", start)
			}
			for _, e := range trace.events {
				if e.Stage == DiagnosticProviderStart || e.Stage == DiagnosticProviderProgress || e.Stage == DiagnosticProviderDone {
					if e.SpanID != modelID {
						t.Fatal("provider measurement cannot be correlated", e)
					}
				}
			}
		})
	}
}

func TestToolDiscoveryLifecycleClosesOnFailure(t *testing.T) {
	for _, tc := range []struct {
		err   error
		class DiagnosticErrorClass
	}{{nil, DiagnosticErrorNone}, {errors.New("offline"), DiagnosticErrorTool}, {context.Canceled, DiagnosticErrorCancelled}, {context.DeadlineExceeded, DiagnosticErrorTimeout}} {
		trace := &diagnosticCollector{}
		s := turnSession(t)
		store := &memoryStore{}
		u := StartTurnUseCase{Catalog: &catalogStub{err: tc.err}, Store: store, Continue: ContinueTurnUseCase{Diagnostics: trace, Store: store, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
			return assistant("done"), nil
		})}}
		err := u.Execute(context.Background(), &s, "go", nil)
		if !errors.Is(err, tc.err) {
			t.Fatal(err)
		}
		lifecyclePair(t, trace, DiagnosticActionTools, 0, tc.class)
	}
}

func TestToolResolutionLifecycleIncludesDefiniteToolError(t *testing.T) {
	trace := &diagnosticCollector{}
	s := queued(t, call("a", "read"))
	store := &memoryStore{}
	var resolveID uint64
	u := ResolveToolUseCase{Diagnostics: trace, Store: store, Tools: executionFunc(func(ctx context.Context, _ domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
		resolveID = CurrentDiagnosticSpan(ctx)
		return domain.ToolOutcome{Content: "failed definitively", IsError: true}, nil
	})}
	if err := u.resolveOne(context.Background(), &s, "a", domain.DecisionApprove, nil); err != nil {
		t.Fatal(err)
	}
	start, _ := lifecyclePair(t, trace, DiagnosticActionToolResolve, 0, DiagnosticErrorTool)
	if start.SpanID != resolveID {
		t.Fatal("tool port lost resolution span")
	}
	for _, e := range trace.events {
		if e.Stage == DiagnosticToolCompleted && (e.SpanID != resolveID || e.ErrorClass != DiagnosticErrorTool) {
			t.Fatal("definite MCP error mislabeled", e)
		}
	}
}

func TestModelCatalogLifecyclePropagatesContextAndClosesFailure(t *testing.T) {
	for _, tc := range []struct {
		err   error
		class DiagnosticErrorClass
	}{{nil, DiagnosticErrorNone}, {errors.New("offline"), DiagnosticErrorProvider}, {context.Canceled, DiagnosticErrorCancelled}, {context.DeadlineExceeded, DiagnosticErrorTimeout}} {
		trace := &diagnosticCollector{}
		var id uint64
		u := ListModelsUseCase{Diagnostics: trace, Catalog: modelCatalogFunc(func(ctx context.Context) ([]domain.AvailableModel, error) {
			id = CurrentDiagnosticSpan(ctx)
			return []domain.AvailableModel{catalogModel("test/model", "Test", true, true)}, tc.err
		})}
		_, err := u.Execute(context.Background())
		if !errors.Is(err, tc.err) {
			t.Fatal(err)
		}
		start, _ := lifecyclePair(t, trace, DiagnosticActionCatalog, 0, tc.class)
		if start.SpanID != id {
			t.Fatal("catalog context lost")
		}
	}
}
