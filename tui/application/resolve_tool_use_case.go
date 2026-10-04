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
	Validation  ToolArgumentValidationPort
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
func (u ResolveToolUseCase) resolveOne(ctx context.Context, s *domain.Session, id root.ToolCallID, decision domain.ToolDecision, emit func(Event) error) (returnErr error) {
	ctx, span := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionToolResolve, DiagnosticEvent{ToolOrdinal: toolDiagnosticOrdinal(s, id)})
	spanClass := DiagnosticErrorNone
	defer func() {
		if returnErr != nil {
			spanClass = DiagnosticErrorTool
			if errors.Is(returnErr, context.Canceled) {
				spanClass = DiagnosticErrorCancelled
			} else if errors.Is(returnErr, context.DeadlineExceeded) {
				spanClass = DiagnosticErrorTimeout
			}
		}
		span.End(spanClass)
	}()
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

	tool, toolArgs, known, resolveErr := ResolveToolCall(s.ToolSnapshot(), pending[0].Call)
	if !known || resolveErr != nil {
		return rejectUnknown(ctx, s, u.Store, emit, u.Diagnostics)
	}
	if verdict, _ := s.Mode().Judge(tool.Identity, toolArgs); verdict == domain.VerdictDeny {
		return rejectUnknown(ctx, s, u.Store, emit, u.Diagnostics)
	}
	if decision == domain.DecisionAutoApprove && !approvesInSession(u.Approval, *s, tool.Identity, toolArgs) {
		return errors.New("tool is not configured for automatic approval")
	}
	if tool.Identity.Kind == domain.ToolKindPlugin && decision != domain.DecisionDeny {
		_, validationSpan := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionToolValidation, DiagnosticEvent{Bytes: len(toolArgs.Bytes())})
		validationErr := errors.New("plugin argument validator is required")
		if u.Validation != nil {
			validationErr = u.Validation.Validate(tool.Definition, toolArgs)
		}
		if validationErr != nil {
			validationSpan.End(DiagnosticErrorInvalidState)
			if err := rejectUnknownCall(ctx, s, u.Store, u.Diagnostics, pending[0], validationErr); err != nil {
				return err
			}
			if err := emitTool(s, id, emit); err != nil {
				return err
			}
			return emitSession(s, emit)
		}
		validationSpan.End(DiagnosticErrorNone)
	}
	if u.Diagnostics != nil {
		stage := DiagnosticToolApproved
		if decision == domain.DecisionDeny {
			stage = DiagnosticToolRejected
		}
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: stage, SpanID: CurrentDiagnosticSpan(ctx)})
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
		if u.Tools == nil && tool.Identity.Kind != domain.ToolKindHost {
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
		var outcome domain.ToolOutcome
		var runErr error
		var step *StepResult
		if tool.Identity.Kind == domain.ToolKindHost && tool.Identity.LocalOperation == domain.HostOperationStepDone {
			var result StepResult
			result, runErr = u.Continue.Ceremonies.StepDone(ctx, *s, toolArgs)
			outcome, step = result.Outcome, &result
		} else if tool.Identity.Kind == domain.ToolKindHost {
			hostCtx, hostSpan := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionToolExecution, DiagnosticEvent{Bytes: len(toolArgs.Bytes())})
			outcome, runErr = (HostToolUseCase{Skills: u.Continue.PluginSkills, Labels: u.Continue.SessionLabels}).Execute(hostCtx, *s, tool.Identity, toolArgs)
			class := DiagnosticErrorNone
			if runErr != nil || outcome.IsError {
				class = DiagnosticErrorTool
			}
			hostSpan.End(class)
		} else {
			outcome, runErr = u.Tools.Execute(ctx, tool.Identity, toolArgs)
		}
		if outcome.IsError || outcome.Uncertain {
			spanClass = DiagnosticErrorTool
		}
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
		if runErr == nil && step != nil && step.Accepted {
			if step.Run == nil {
				next.FinishCeremony()
			} else if err := next.SetCeremony(*step.Run); err != nil {
				return err
			}
			next.RestartTurnBudget()
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
			u.recordToolCompletion(ctx, started, DiagnosticErrorTool)
			return errors.Join(runErr, emitTool(s, id, emit), emit(Event{Kind: EventState, State: s.Status()}))
		}
	}
	if err := emitTool(s, id, emit); err != nil {
		return err
	}
	u.recordToolCompletion(ctx, started, spanClass)
	return emitSession(s, emit)
}
func (u ResolveToolUseCase) recordToolCompletion(ctx context.Context, started time.Time, class DiagnosticErrorClass) {
	if u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticToolCompleted, SpanID: CurrentDiagnosticSpan(ctx), ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
	}
}
func (u ResolveToolUseCase) advance(ctx context.Context, s *domain.Session, emit func(Event) error) error {
	continuation := u.Continue
	if continuation.Validation == nil {
		continuation.Validation = u.Validation
	}
	continuation.Store = u.Store
	if err := (AgentTurnUseCase{Continue: continuation, Tools: u.Tools, Approval: u.Approval}).Execute(ctx, s, emit); err != nil {
		return err
	}
	return emit(Event{Kind: EventState, State: s.Status()})
}
