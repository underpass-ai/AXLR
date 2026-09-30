package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ResolveToolUseCase struct {
	Approval    ToolApprovalPolicyPort
	Tools       ToolExecutionPort
	Store       SessionStorePort
	Continue    ContinueTurnUseCase
	Diagnostics DiagnosticPort
}

func (u ResolveToolUseCase) Execute(ctx context.Context, s *domain.Session, id root.ToolCallID, decision domain.ToolDecision, emit func(Event) error) error {
	if decision != domain.DecisionApprove && decision != domain.DecisionDeny {
		return errors.New("invalid human tool decision")
	}
	if emit == nil {
		emit = func(Event) error { return nil }
	}
	if err := u.resolveOne(ctx, s, id, decision, emit); err != nil {
		return err
	}
	return u.advance(ctx, s, emit)
}
func (u ResolveToolUseCase) resolveOne(ctx context.Context, s *domain.Session, id root.ToolCallID, decision domain.ToolDecision, emit func(Event) error) error {
	if s == nil || u.Store == nil {
		return errors.New("resolve tool requires session and store")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if emit == nil {
		emit = func(Event) error { return nil }
	}
	pending := s.Pending()
	if s.Status() != domain.StatusApproval || len(pending) == 0 || pending[0].Call.ID != id {
		return errors.New("decision must resolve the first pending call")
	}
	if decision != domain.DecisionApprove && decision != domain.DecisionAutoApprove && decision != domain.DecisionDeny {
		return errors.New("invalid tool decision")
	}
	started := time.Now()
	if u.Diagnostics != nil {
		stage := DiagnosticToolApproved
		if decision == domain.DecisionDeny {
			stage = DiagnosticToolRejected
		}
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: stage})
	}
	tool, known := findTool(s, pending[0].Call.Name)
	if !known {
		return rejectUnknown(ctx, s, u.Store, emit)
	}
	if decision == domain.DecisionAutoApprove && (u.Approval == nil || !u.Approval.AutoApproves(tool.Identity)) {
		return errors.New("tool is not configured for automatic approval")
	}
	next := *s
	if decision == domain.DecisionDeny {
		if err := next.RecordToolOutcome(id, decision, domain.ToolOutcome{Content: "tool call denied by user", IsError: true}); err != nil {
			return err
		}
		if err := u.Store.Save(ctx, next); err != nil {
			return err
		}
		*s = next
	} else {
		if u.Tools == nil {
			return errors.New("approval requires tool executor")
		}
		// Persist before the effect: a crash must never reopen this call for execution.
		if err := next.RecordToolOutcome(id, decision, domain.ToolOutcome{Content: "tool execution started; effect unknown", IsError: true, Uncertain: true}); err != nil {
			return err
		}
		if err := next.PauseTurn(); err != nil {
			return err
		}
		if err := u.Store.Save(ctx, next); err != nil {
			return err
		}
		*s = next
		if err := emit(Event{Kind: EventToolExecutionStarted, Tool: domain.PendingTool{Call: pending[0].Call, Decision: decision}, Memory: tool.Identity.Kind == domain.ToolKindPlugin && tool.Identity.Plugin.PluginID == "kmp"}); err != nil {
			return err
		}
		outcome, runErr := u.Tools.Execute(ctx, tool.Identity, pending[0].Call.Arguments)
		runErr = errors.Join(runErr, ctx.Err())
		if runErr != nil {
			outcome = domain.ToolOutcome{Content: root.Text(fmt.Sprintf("tool execution failed; effect unknown: %v", runErr)), IsError: true, Uncertain: true}
		}
		if outcome.Uncertain && runErr == nil {
			runErr = errors.New("tool effect is uncertain; turn paused")
		}
		next = *s
		if err := next.FinishToolExecution(id, outcome); err != nil {
			return errors.Join(runErr, err)
		}
		if runErr != nil {
			if ctx.Err() != nil {
				if err := next.CancelPending(); err != nil {
					return errors.Join(runErr, err)
				}
			} else if err := next.PauseTurn(); err != nil {
				return errors.Join(runErr, err)
			}
		}
		if err := u.Store.Save(context.WithoutCancel(ctx), next); err != nil {
			return errors.Join(runErr, err)
		}
		*s = next
		if runErr != nil {
			u.recordToolCompletion(started, DiagnosticErrorTool)
			return errors.Join(runErr, emitTool(s, id, emit), emit(Event{Kind: EventState, State: s.Status()}))
		}
	}
	if err := emitTool(s, id, emit); err != nil {
		return err
	}
	u.recordToolCompletion(started, DiagnosticErrorNone)
	return emitSession(s, emit)
}
func (u ResolveToolUseCase) recordToolCompletion(started time.Time, class DiagnosticErrorClass) {
	if u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticToolCompleted, ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
	}
}
func (u ResolveToolUseCase) advance(ctx context.Context, s *domain.Session, emit func(Event) error) error {
	continuation := u.Continue
	continuation.Store = u.Store
	if err := (AgentTurnUseCase{Continue: continuation, Tools: u.Tools, Approval: u.Approval}).Execute(ctx, s, emit); err != nil {
		return err
	}
	return emit(Event{Kind: EventState, State: s.Status()})
}
