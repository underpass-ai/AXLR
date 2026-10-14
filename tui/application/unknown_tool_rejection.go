package application

import (
	"context"
	"encoding/json"
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
	if err := localCommandError(tool.Identity.LocalOperation, arguments); err != nil {
		return fmt.Errorf("%s: %w", tool.Definition.Name, err)
	}
	return nil
}

// localCommandError runs, before the card, the runtime's checks of a local
// call that read no file, with the runtime's own functions and so its
// exact messages: a path relative to the workspace, a digest that is one,
// create without and replace with expected_sha256, an edit with old_text.
// On 14 October 2026 a local_write replace without expected_sha256 raised
// a card, the person approved it, the runtime refused it, and the person
// approved the same content a second time. The filesystem checks (the
// file exists, its digest matches) stay with the runtime.
func localCommandError(operation string, arguments root.JSONValue) error {
	var args struct {
		Path     *string `json:"path"`
		Mode     string  `json:"mode"`
		Expected string  `json:"expected_sha256"`
		OldText  *string `json:"old_text"`
	}
	switch operation {
	case "read", "write", "edit":
	default:
		return nil
	}
	if json.Unmarshal(arguments.Bytes(), &args) != nil || args.Path == nil {
		return nil // the schema check names what is missing
	}
	path, err := root.NewRelativePath(*args.Path)
	if err != nil {
		return err
	}
	var expected root.Digest
	if args.Expected != "" && operation != "read" {
		if expected, err = root.NewDigest(args.Expected); err != nil {
			return err
		}
	}
	switch operation {
	case "write":
		mode, err := root.NewWriteMode(args.Mode)
		if err != nil {
			return err
		}
		return root.CheckDigestForMode(path, mode, expected)
	case "edit":
		if args.OldText != nil {
			return root.CheckOldText(root.Text(*args.OldText))
		}
	}
	return nil
}
