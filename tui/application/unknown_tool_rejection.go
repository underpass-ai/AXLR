package application

import (
	"context"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func rejectUnknownCall(ctx context.Context, s *domain.Session, store SessionStorePort, trace DiagnosticPort, p domain.PendingTool, invalid ...error) (returnErr error) {
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
	reason := fmt.Sprintf("unknown tool %q rejected", p.Call.Name)
	if len(invalid) > 0 && invalid[0] != nil {
		reason = "invalid tool invocation rejected: " + invalid[0].Error()
		var denial ModeDenial
		if errors.As(invalid[0], &denial) {
			reason = denial.Error()
		}
		// The reason may quote non-ASCII argument text: cut on a rune
		// boundary so the outcome stays valid UTF-8.
		reason = utf8Prefix(reason, 1024)
	}
	if err := next.RecordToolOutcome(p.Call.ID, domain.DecisionDeny, domain.ToolOutcome{Content: root.Text(reason), IsError: true}); err != nil {
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

// localArgumentError checks a local tool's arguments against its schema
// before the card: on 10 October 2026 claude-haiku-5.5 sent local_exec
// timeout_ms 600000, the person approved it and the runtime then refused
// it. Plugin arguments are still validated when the call is resolved.
func localArgumentError(validation ToolArgumentValidationPort, tool domain.AvailableTool, arguments root.JSONValue) error {
	if validation == nil || tool.Identity.Kind != domain.ToolKindLocal {
		return nil
	}
	if err := validation.Validate(tool.Definition, arguments); err != nil {
		return fmt.Errorf("%s: %w", tool.Definition.Name, err)
	}
	return nil
}
