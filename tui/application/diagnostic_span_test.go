package application

import (
	"context"
	"sync"
	"testing"
)

type spanCollector struct {
	mu     sync.Mutex
	events []DiagnosticEvent
}

func (s *spanCollector) Record(e DiagnosticEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}
func TestDiagnosticSpanCorrelatesParentsAndEndsExactlyOnce(t *testing.T) {
	trace := &spanCollector{}
	ctx, outer := StartDiagnosticSpan(context.Background(), trace, DiagnosticActionModel, DiagnosticEvent{OperationID: 9})
	childCtx, child := StartDiagnosticSpan(ctx, trace, DiagnosticActionHTTP, DiagnosticEvent{RequestID: 7})
	if CurrentDiagnosticSpan(ctx) == 0 || CurrentDiagnosticSpan(childCtx) == CurrentDiagnosticSpan(ctx) {
		t.Fatal("span context absent")
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); child.End(DiagnosticErrorProvider) }()
	}
	wg.Wait()
	outer.End(DiagnosticErrorNone)
	if len(trace.events) != 4 {
		t.Fatal(trace.events)
	}
	started, ended := trace.events[1], trace.events[2]
	if started.SpanID != ended.SpanID || started.ParentSpanID != trace.events[0].SpanID || ended.RequestID != 7 || ended.ErrorClass != DiagnosticErrorProvider || ended.Stage != DiagnosticActionEnd || ended.ElapsedMicroseconds < 0 {
		t.Fatalf("bad span: %+v %+v", started, ended)
	}
}
func TestNoTraceDoesNotAlterContext(t *testing.T) {
	ctx := context.Background()
	got, span := StartDiagnosticSpan(ctx, nil, DiagnosticActionModel, DiagnosticEvent{})
	if got != ctx || span != nil || CurrentDiagnosticSpan(nil) != 0 {
		t.Fatal("unexpected instrumentation")
	}
	span.End(DiagnosticErrorNone)
	ctx, span = StartDiagnosticSpan(nil, nil, DiagnosticActionModel, DiagnosticEvent{})
	if ctx == nil || span != nil {
		t.Fatal("nil context")
	}
	for _, a := range []DiagnosticAction{"", "secret"} {
		if a.Valid() {
			t.Fatal("raw action accepted")
		}
	}
}
