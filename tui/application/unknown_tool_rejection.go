package application

import (
	"context"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func rejectUnknownCall(ctx context.Context, s *domain.Session, store SessionStorePort, trace DiagnosticPort, p domain.PendingTool) (returnErr error) {
	ctx, span := StartDiagnosticSpan(ctx, trace, DiagnosticActionToolResolve, DiagnosticEvent{ToolOrdinal: toolDiagnosticOrdinal(s, p.Call.ID)})
	defer func() {
		class := DiagnosticErrorNone
		if returnErr != nil {
			class = DiagnosticErrorTool
		}
		if errors.Is(returnErr, context.Canceled) {
			class = DiagnosticErrorCancelled
		}
		if errors.Is(returnErr, context.DeadlineExceeded) {
			class = DiagnosticErrorTimeout
		}
		span.End(class)
	}()
	next := *s
	if err := next.RecordToolOutcome(p.Call.ID, domain.DecisionDeny, domain.ToolOutcome{Content: root.Text(fmt.Sprintf("unknown tool %q rejected", p.Call.Name)), IsError: true}); err != nil {
		return err
	}
	if err := store.Save(ctx, next); err != nil {
		return err
	}
	*s = next
	if trace != nil {
		_ = trace.Record(DiagnosticEvent{Stage: DiagnosticToolRejected, SpanID: CurrentDiagnosticSpan(ctx), ToolOrdinal: toolDiagnosticOrdinal(s, p.Call.ID)})
	}
	return nil
}
