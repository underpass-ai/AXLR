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

func (u AgentTurnUseCase) Execute(ctx context.Context, s *domain.Session, emit func(Event) error) (returnErr error) {
	if u.Approval != nil {
		u.Continue.Approval = u.Approval
	}
	if s == nil || u.Continue.Store == nil {
		return errors.New("agent turn requires session and store")
	}
	if emit == nil {
		emit = func(Event) error { return nil }
	}
	// The execution's budget bounds the turn's time, and its correlation
	// goes on the loop's trace records (execution_budget.go).
	ctx, cancel := executionDeadline(ctx, *s)
	defer cancel()
	defer func() { returnErr = settleBudget(ctx, u.Continue.Store, s, returnErr) }()
	u.Continue.Diagnostics = sessionDiagnostics(u.Continue.Diagnostics, *s)
	for {
		if err := ctx.Err(); err != nil {
			if stop := budgetCause(ctx, err); stop != nil {
				return pauseAtBudget(ctx, u.Continue.Store, s, stop, emit)
			}
			return err
		}
		if err := rejectUnknown(ctx, s, u.Continue.admission(), emit); err != nil {
			return err
		}
		if s.Status() == domain.StatusApproval && len(s.Pending()) > 0 {
			pending := s.Pending()[0]
			tool, args, known, resolveErr := ResolveToolCall(s.ToolSnapshot(), pending.Call)
			approved := false
			if known && resolveErr == nil {
				approved, _ = approvesCall(ctx, u.Approval, u.Continue.Ceremonies, *s, tool.Identity, args)
			}
			if approved {
				// The execution's budget: no call past its tool calls. The
				// call stays pending, for a run under a new budget.
				if err := u.Continue.toolCallRefusal(ctx, *s); err != nil {
					if stop := budgetCause(ctx, err); stop != nil {
						return pauseAtBudget(ctx, u.Continue.Store, s, stop, emit)
					}
					return err
				}
				resolver := ResolveToolUseCase{Tools: u.Tools, Store: u.Continue.Store, Diagnostics: u.Continue.Diagnostics, Approval: u.Approval, Validation: u.Continue.Validation, Continue: u.Continue}
				run, live := s.Ceremony()
				wasTask := live && run.Task != nil
				if err := resolver.resolveOne(ctx, s, pending.Call.ID, domain.DecisionAutoApprove, emit); err != nil {
					return err
				}
				// Workers run without the person, so a task's last hand-back
				// is approved here, not in ResolveToolUseCase.Execute: its
				// turn closes at DONE the same way.
				if _, still := s.Ceremony(); wasTask && !still && s.Status() == domain.StatusStreaming {
					return resolver.closeTaskTurn(ctx, s, emit)
				}
				continue
			}
		}
		if s.Status() != domain.StatusStreaming {
			// A console follow-up (a step reminder, Jev's check, the memory
			// reminder) asks the model again: none when the execution's
			// budget has no request left, so an answered prompt ends
			// completed rather than at the budget.
			followUps := u.Continue.requestsLeft(ctx, *s)
			reminded, err := false, error(nil)
			if followUps {
				reminded, err = remindOpenStep(ctx, s, u.Continue.Store, emit)
			}
			if followUps && !reminded && err == nil {
				reminded, err = cancelStalled(ctx, s, u.Continue, emit)
			}
			if followUps && !reminded && err == nil {
				reminded, err = checkFinalAnswer(ctx, s, u.Continue, emit)
			}
			if followUps && !reminded && err == nil {
				reminded, err = remindMemory(ctx, s, u.Continue, emit)
			}
			if err != nil {
				return err
			}
			if !reminded {
				// The turn is over: its record goes to the session's MADE
				// step and its results to KMP, after any memory the model
				// recorded on the reminder.
				if err := u.Continue.Session.EndTurn(ctx, s); err != nil {
					return err
				}
				titleUntitled(ctx, *s, u.Continue.SessionLabels)
				return nil
			}
			continue
		}
		if err := u.Continue.Execute(ctx, s, emit); err != nil {
			if !atCallLimit(err) {
				return err
			}
			resumed, resumeErr := resumeStepAtLimit(ctx, s, u.Continue, emit)
			if resumeErr != nil || !resumed {
				return errors.Join(err, resumeErr)
			}
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

func (u ContinueTurnUseCase) admission() headAdmission {
	return headAdmission{Store: u.Store, Trace: u.Diagnostics, Validation: u.Validation, Forge: u.Forge, Publisher: u.Publisher, TurnLimit: u.turnLimit(), Approval: u.Approval}
}

func rejectUnknown(ctx context.Context, s *domain.Session, admit headAdmission, emit func(Event) error) error {
	for s.Status() == domain.StatusApproval && len(s.Pending()) > 0 {
		p := s.Pending()[0]
		// A call past the budget is answered "over budget" before anything
		// else: no card, no mode verdict, no effect.
		if s.HeadOverBudget(admit.turnLimit()) {
			next := *s
			if _, err := next.RefuseOverBudget(admit.turnLimit()); err != nil {
				return err
			}
			if err := admit.Store.Save(ctx, next); err != nil {
				return err
			}
			*s = next
			if admit.Trace != nil {
				_ = admit.Trace.Record(DiagnosticEvent{Stage: DiagnosticToolRejected, SpanID: CurrentDiagnosticSpan(ctx), ToolOrdinal: toolDiagnosticOrdinal(s, p.Call.ID)})
			}
			if err := emitTool(s, p.Call.ID, emit); err != nil {
				return err
			}
			continue
		}
		tool, args, known, resolveErr := ResolveToolCall(s.ToolSnapshot(), p.Call)
		if known && resolveErr == nil {
			// The execution's policy answers before the mode and before
			// any card: a forbidden call is refused outside the model.
			if refusal, forbidden := policyRefusal(*s, tool, args); forbidden {
				resolveErr = refusal
			} else if verdict, reason := s.Mode().Judge(tool.Identity, args); verdict != domain.VerdictDeny {
				if resolveErr = compactRefusal(*s, p); resolveErr == nil {
					// The missing why is named before the schema check, so
					// the model reads what to send rather than a schema rule.
					var checked domain.AvailableTool
					if checked, resolveErr = admit.whyError(*s, tool, args); resolveErr == nil {
						if resolveErr = localArgumentError(admit.Validation, checked, args); resolveErr == nil {
							if resolveErr = forgedArgumentError(ctx, admit.Forge, admit.Validation, *s, checked, args); resolveErr == nil {
								if resolveErr = publishArgumentError(ctx, admit.Publisher, *s, checked, args); resolveErr == nil {
									return nil
								}
							}
						}
					}
				}
			} else {
				resolveErr = ModeDenial{Mode: s.Mode(), Reason: reason}
			}
		}
		if err := rejectUnknownCall(ctx, s, admit.Store, admit.Trace, p, resolveErr); err != nil {
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
	if err := next.BeginTurn(stepReminder(run)+root.Text(leakNote(*s)), next.ToolSnapshot()); err != nil {
		return false, err
	}
	if err := store.Save(ctx, next); err != nil {
		return false, err
	}
	*s = next
	return true, emitSession(s, emit)
}
