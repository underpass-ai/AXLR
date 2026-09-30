package axlr

import (
	"context"
	"encoding/json"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/diagnostics"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolRunnerRecordsBalancedExecutionWithDefiniteFailureAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cancel bool
		class  application.DiagnosticErrorClass
	}{{"definite-error", false, application.DiagnosticErrorTool}, {"cancelled", true, application.DiagnosticErrorCancelled}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			exe, err := runtime.New(runtime.Config{Root: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer exe.Close()
			path := filepath.Join(t.TempDir(), "trace.jsonl")
			trace, err := diagnostics.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer trace.Close()
			ctx, parent := application.StartDiagnosticSpan(context.Background(), trace, application.DiagnosticActionToolResolve, application.DiagnosticEvent{})
			parentID := application.CurrentDiagnosticSpan(ctx)
			if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			id, _ := domain.NewLocalToolIdentity("read")
			out, err := (ToolRunner{Executor: exe, Diagnostics: trace}).Execute(ctx, id, jsonValue(t, `{"path":"absent"}`))
			if err != nil || !out.IsError || out.Uncertain != tc.cancel {
				t.Fatal("tool semantics changed", out, err)
			}
			parent.End(application.DiagnosticErrorNone)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var starts, ends []application.DiagnosticEvent
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var e application.DiagnosticEvent
				if err = json.Unmarshal([]byte(line), &e); err != nil {
					t.Fatal(err)
				}
				if e.Action == application.DiagnosticActionToolExecution {
					if e.Stage == application.DiagnosticActionStart {
						starts = append(starts, e)
					}
					if e.Stage == application.DiagnosticActionEnd {
						ends = append(ends, e)
					}
				}
			}
			if len(starts) != 1 || len(ends) != 1 {
				t.Fatal("unbalanced execution", string(data))
			}
			a, b := starts[0], ends[0]
			if a.SpanID != b.SpanID || a.ParentSpanID != parentID || b.ParentSpanID != parentID || b.ErrorClass != tc.class || b.ElapsedMicroseconds < 0 || a.Bytes != 17 {
				t.Fatal("lost correlation or measurements", a, b)
			}
		})
	}
}
