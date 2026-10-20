package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	// denyReason is what Deny tells the model with the denial.
	denyReason string
}

// MaxDenyReasonRunes bounds the reason a denial carries to the model.
const MaxDenyReasonRunes = 500

// cancelledByPerson is the result of a call the person cancelled (Esc)
// while it ran. It starts like the result of a pending call the person
// cancelled, "tool call cancelled", so every reader that tells a
// cancellation from a failure keeps doing so. On 14 October 2026 such a
// call read "tool execution failed; effect unknown: context canceled" and
// glm-5.3-flash ran it again on the next, unrelated prompt ("Retry the
// sleep command the user asked for after the canceled sleep 40").
const cancelledByPerson = "tool call cancelled by the person before it finished: its effect is unknown (it may have run in part). Do not run it again unless the person asks for it."

// Deny denies the first pending call and tells the model why in the call's
// result, "tool call denied by user: <reason>", so it can change course
// instead of guessing; on 12 October 2026 a bare denial left the model
// asking "should I change the text?". An empty reason is a bare denial.
// The reason is one line, at most MaxDenyReasonRunes.
func (u ResolveToolUseCase) Deny(ctx context.Context, s *domain.Session, id root.ToolCallID, reason string, emit func(Event) error) error {
	reason = strings.Join(strings.Fields(reason), " ")
	if runes := []rune(reason); len(runes) > MaxDenyReasonRunes {
		reason = string(runes[:MaxDenyReasonRunes-1]) + "…"
	}
	u.denyReason = reason
	return u.Execute(ctx, s, id, domain.DecisionDeny, emit)
}

func (u ResolveToolUseCase) Execute(ctx context.Context, s *domain.Session, id root.ToolCallID, decision domain.ToolDecision, emit func(Event) error) error {
	if decision != domain.DecisionApprove && decision != domain.DecisionDeny {
		return errors.New("invalid human tool decision")
	}
	if emit == nil {
		emit = func(Event) error { return nil }
	}
	run, live := s.Ceremony()
	wasTask := live && run.Task != nil
	if err := u.resolveOne(ctx, s, id, decision, emit); err != nil {
		return err
	}
	// A plan task's outcome is already in the plan panel: when its ceremony
	// has finished, close the turn here instead of asking the model again.
	if _, still := s.Ceremony(); wasTask && !still && s.Status() == domain.StatusStreaming {
		return u.closeTaskTurn(ctx, s, emit)
	}
	return u.advance(ctx, s, emit)
}

// closeTaskTurn ends a finished task's turn without a model request. The
// closing message is the console's own, not the model's.
func (u ResolveToolUseCase) closeTaskTurn(ctx context.Context, s *domain.Session, emit func(Event) error) error {
	next := *s
	if err := next.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "The task ended; its outcome is in the plan."}}); err != nil {
		return err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*s = next
	return emit(Event{Kind: EventState, State: s.Status()})
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
	admit := headAdmission{Store: u.Store, Trace: u.Diagnostics, Validation: u.Validation, Forge: u.Continue.Forge, TurnLimit: u.Continue.turnLimit()}
	if !known || resolveErr != nil || s.HeadOverBudget(admit.turnLimit()) {
		return rejectUnknown(ctx, s, admit, emit)
	}
	if verdict, _ := s.Mode().Judge(tool.Identity, toolArgs); verdict == domain.VerdictDeny || compactRefusal(*s, pending[0]) != nil || whyError(tool, toolArgs) != nil || localArgumentError(u.Validation, tool, toolArgs) != nil || (decision != domain.DecisionDeny && forgedArgumentError(ctx, admit.Forge, u.Validation, *s, tool, toolArgs) != nil) {
		return rejectUnknown(ctx, s, admit, emit)
	}
	if decision == domain.DecisionAutoApprove {
		approved, withheld := approvesCall(ctx, u.Approval, u.Continue.Ceremonies, *s, tool.Identity, toolArgs)
		if !approved {
			return errors.New("tool is not configured for automatic approval")
		}
		if withheld {
			ctx = withholdCommands(ctx)
		}
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
		denial := "tool call denied by user"
		if u.denyReason != "" {
			denial += ": " + u.denyReason
		}
		if err := next.RecordToolOutcome(id, decision, domain.ToolOutcome{Content: root.Text(denial), IsError: true}); err != nil {
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
		// A long turn reports to MADE and renews its lease on the way.
		u.Continue.Session.Heartbeat(ctx, &next)
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
		if err := emit(Event{Kind: EventToolExecutionStarted, Tool: domain.PendingTool{Call: pending[0].Call, Decision: decision}, Memory: tool.Identity.Kind == domain.ToolKindPlugin && tool.Surface.Role == domain.PluginRoleMemory}); err != nil {
			return err
		}
		// The runtime names the call in its result for the model to cite.
		ctx := WithToolCall(ctx, id)
		var outcome domain.ToolOutcome
		var runErr error
		var step *StepResult
		var approved []domain.CheckCommand
		if tool.Identity.Kind == domain.ToolKindHost && tool.Identity.LocalOperation == domain.HostOperationStepDone {
			if run, live := s.Ceremony(); live && decision == domain.DecisionApprove {
				approved = approvalChecks(run, toolArgs)
			}
			var result StepResult
			result, runErr = u.Continue.Ceremonies.StepDone(ctx, *s, toolArgs)
			outcome, step = result.Outcome, &result
		} else if tool.Identity.Kind == domain.ToolKindHost {
			hostCtx, hostSpan := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionToolExecution, DiagnosticEvent{Bytes: len(toolArgs.Bytes())})
			outcome, runErr = (HostToolUseCase{Skills: u.Continue.PluginSkills, Labels: u.Continue.SessionLabels, Repairs: u.Continue.SelfRepair, Judge: u.Continue.Judge.toolPort(), Windows: u.Continue.Windows, Memory: u.Continue.Remember, Recall: u.Continue.Recall, Forge: u.Continue.Forge, Catalog: u.Continue.Catalog, Tools: u.Tools, Validation: u.Validation, Logs: u.Continue.Logs}).Execute(hostCtx, *s, tool.Identity, toolArgs)
			class := DiagnosticErrorNone
			if runErr != nil || outcome.IsError {
				class = DiagnosticErrorTool
			}
			hostSpan.End(class)
		} else if tool.Identity.Kind == domain.ToolKindLocal && tool.Identity.LocalOperation == "read" {
			// A page is sized to what the projection keeps per tool result.
			outcome, runErr = boundedLocalRead(ctx, u.Tools, tool.Identity, toolArgs, projectionBudget(u.Continue.Windows, *s).ToolResultBytes())
		} else if tool.Identity.Kind == domain.ToolKindLocal && (tool.Identity.LocalOperation == "search" || tool.Identity.LocalOperation == "list") {
			// So is a page of matches or entries.
			outcome, runErr = boundedLocalListing(ctx, u.Tools, tool.Identity, toolArgs, projectionBudget(u.Continue.Windows, *s).ToolResultBytes())
		} else {
			if tool.Identity.Kind == domain.ToolKindLocal && tool.Identity.LocalOperation == "exec" && tolerantSession(*s) {
				toolArgs, _ = normalizeExec(toolArgs)
			}
			// The model's KMP calls run in the session's context (decision D3).
			outcome, runErr = u.Tools.Execute(ctx, tool.Identity, withSessionContext(*s, tool, toolArgs))
		}
		if outcome.IsError || outcome.Uncertain {
			spanClass = DiagnosticErrorTool
		}
		runErr = joinCause(runErr, ctx.Err())
		if runErr != nil {
			outcome = domain.ToolOutcome{Content: root.Text(fmt.Sprintf("tool execution failed; effect unknown: %v", runErr)), IsError: true, Uncertain: true}
			if errors.Is(ctx.Err(), context.Canceled) {
				outcome.Content = cancelledByPerson
			}
		}
		if outcome.Uncertain && runErr == nil {
			runErr = errors.New("tool effect is uncertain; turn paused")
		}
		outcome = citeCall(outcome, id)
		next = *s
		if err := next.FinishToolExecution(id, outcome); err != nil {
			return errors.Join(runErr, err)
		}
		if runErr == nil && step != nil {
			switch {
			case step.Accepted && step.Run == nil:
				next.FinishCeremony()
			case step.Run != nil:
				// A refusal can still carry state, such as a reviewer failure.
				if step.Accepted {
					// The compact projection starts the next step after this
					// call's results.
					step.Run.StepCall = string(id)
				}
				if err := next.SetCeremony(*step.Run); err != nil {
					return err
				}
			}
			if step.Accepted {
				next.RestartTurnBudget()
			}
		}
		// The person approved the card's commands: the same commands need no
		// second card in this ceremony, whether the step was accepted or not.
		if run, live := next.Ceremony(); live && len(approved) > 0 {
			run.Approve(approved...)
			if err := next.SetCeremony(run); err != nil {
				return errors.Join(runErr, err)
			}
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
			return joinCause(joinCause(runErr, emitTool(s, id, emit)), emit(Event{Kind: EventState, State: s.Status()}))
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
