package application

import (
	"context"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// AgentTurnUseCase advances until a final answer or a human decision is needed.
// ContinueTurnUseCase remains responsible for exactly one model stream.
type AgentTurnUseCase struct {
	Continue ContinueTurnUseCase
	Tools    ToolExecutionPort
	Approval ToolApprovalPolicyPort
}

func (u AgentTurnUseCase) Execute(ctx context.Context, s *domain.Session, emit func(Event) error) error {
	if s == nil || u.Continue.Store == nil {
		return errors.New("agent turn requires session and store")
	}
	if emit == nil {
		emit = func(Event) error { return nil }
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := rejectUnknown(ctx, s, u.Continue.Store, emit, u.Continue.Diagnostics); err != nil {
			return err
		}
		if s.Status() == domain.StatusApproval && len(s.Pending()) > 0 {
			pending := s.Pending()[0]
			tool, args, known, resolveErr := ResolveToolCall(s.ToolSnapshot(), pending.Call)
			if known && resolveErr == nil && approvesInSession(u.Approval, *s, tool.Identity, args) {
				resolver := ResolveToolUseCase{Tools: u.Tools, Store: u.Continue.Store, Diagnostics: u.Continue.Diagnostics, Approval: u.Approval, Validation: u.Continue.Validation, Continue: u.Continue}
				if err := resolver.resolveOne(ctx, s, pending.Call.ID, domain.DecisionAutoApprove, emit); err != nil {
					return err
				}
				continue
			}
		}
		if s.Status() != domain.StatusStreaming {
			reminded, err := remindOpenStep(ctx, s, u.Continue.Store, emit)
			if err != nil {
				return err
			}
			if !reminded {
				reminded, err = checkFinalAnswer(ctx, s, u.Continue, emit)
			}
			if err != nil || !reminded {
				return err
			}
			continue
		}
		if err := u.Continue.Execute(ctx, s, emit); err != nil {
			return err
		}
	}
}
func findTool(s *domain.Session, name root.ToolName) (domain.AvailableTool, bool) {
	for _, tool := range s.ToolSnapshot() {
		if tool.Definition.Name == name {
			return tool, true
		}
	}
	return domain.AvailableTool{}, false
}
func rejectUnknown(ctx context.Context, s *domain.Session, store SessionStorePort, emit func(Event) error, trace DiagnosticPort) error {
	for s.Status() == domain.StatusApproval && len(s.Pending()) > 0 {
		p := s.Pending()[0]
		tool, args, known, resolveErr := ResolveToolCall(s.ToolSnapshot(), p.Call)
		if known && resolveErr == nil {
			verdict, reason := s.Mode().Judge(tool.Identity, args)
			if verdict != domain.VerdictDeny {
				return nil
			}
			resolveErr = ModeDenial{Mode: s.Mode(), Reason: reason}
		}
		if err := rejectUnknownCall(ctx, s, store, trace, p, resolveErr); err != nil {
			return err
		}
		if err := emitTool(s, p.Call.ID, emit); err != nil {
			return err
		}
	}
	return nil
}
func emitTool(s *domain.Session, id root.ToolCallID, emit func(Event) error) error {
	for _, p := range s.Export().Activity {
		if p.Call.ID == id {
			return emit(Event{Kind: EventToolActivity, Tool: p})
		}
	}
	return nil
}

func emitSession(s *domain.Session, emit func(Event) error) error {
	snapshot := s.Export()
	return emit(Event{Kind: EventSession, Snapshot: &snapshot, State: s.Status()})
}

// remindOpenStep starts one console turn when the model ended its answer with
// a ceremony step still open, a pattern seen repeatedly with small models: the
// work is done but never handed back. It reminds once per claimed step; the
// reminder is a visible message, not hidden guidance.
func remindOpenStep(ctx context.Context, s *domain.Session, store SessionStorePort, emit func(Event) error) (bool, error) {
	run, live := s.Ceremony()
	if !live || run.Reminded || run.AwaitingPerson() || s.Status() != domain.StatusComplete {
		return false, nil
	}
	run.Reminded = true
	next := *s
	if err := next.SetCeremony(run); err != nil {
		return false, err
	}
	if err := next.BeginTurn(stepReminder(run), next.ToolSnapshot()); err != nil {
		return false, err
	}
	if err := store.Save(ctx, next); err != nil {
		return false, err
	}
	*s = next
	return true, emitSession(s, emit)
}
