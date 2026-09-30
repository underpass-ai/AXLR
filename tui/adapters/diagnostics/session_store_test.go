package diagnostics

import (
	"context"
	"errors"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"testing"
	"time"
)

func TestSessionStoreSpansCoverAllOperationsAndFailures(t *testing.T) {
	next, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	trace := &traceEvents{}
	store := SessionStore{Next: next, Trace: trace}
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "test/model")
	if err != nil {
		t.Fatal(err)
	}
	ctx, parent := application.StartDiagnosticSpan(context.Background(), trace, application.DiagnosticActionOperation, application.DiagnosticEvent{})
	parentID := application.CurrentDiagnosticSpan(ctx)
	if err = store.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Load(ctx, session.Export().ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Load(ctx, "11111111111111111111111111111111"); err == nil {
		t.Fatal("missing session accepted")
	}
	parent.End(application.DiagnosticErrorNone)
	starts := map[uint64]application.DiagnosticEvent{}
	ends := map[uint64]application.DiagnosticEvent{}
	for _, e := range trace.events {
		if e.Stage == application.DiagnosticActionStart && e.Action != application.DiagnosticActionOperation {
			starts[e.SpanID] = e
		}
		if e.Stage == application.DiagnosticActionEnd && e.Action != application.DiagnosticActionOperation {
			ends[e.SpanID] = e
		}
	}
	if len(starts) != 4 || len(ends) != 4 {
		t.Fatal("storage lifecycles unbalanced", trace.events)
	}
	failures := 0
	for id, a := range starts {
		b, ok := ends[id]
		if !ok || b.Action != a.Action || a.ParentSpanID != parentID || b.ElapsedMicroseconds < 0 {
			t.Fatal("storage action cannot be correlated", a, b)
		}
		if b.ErrorClass != application.DiagnosticErrorNone {
			failures++
			if b.ErrorClass != application.DiagnosticErrorStorage {
				t.Fatal(b)
			}
		}
	}
	if failures != 1 {
		t.Fatal("storage failure not recorded", trace.events)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err = store.Save(cancelCtx, session); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	last := trace.events[len(trace.events)-2]
	if last.Stage != application.DiagnosticActionEnd || last.ErrorClass != application.DiagnosticErrorCancelled {
		t.Fatal("cancelled save classified incorrectly", last)
	}
	deadlineCtx, stop := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer stop()
	if _, err = store.List(deadlineCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	last = trace.events[len(trace.events)-2]
	if last.Stage != application.DiagnosticActionEnd || last.ErrorClass != application.DiagnosticErrorTimeout {
		t.Fatal("deadline list classified incorrectly", last)
	}
}
