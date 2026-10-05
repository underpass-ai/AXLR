package application

import (
	"context"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type StartTurnUseCase struct {
	Tools    ToolExecutionPort
	Approval ToolApprovalPolicyPort
	Catalog  ToolCatalogPort
	Store    SessionStorePort
	Continue ContinueTurnUseCase
	// Notices hands the session what its self-repairs left for it; nil
	// without self-repair.
	Notices RepairNoticesPort
}

func (u StartTurnUseCase) Execute(ctx context.Context, session *domain.Session, prompt root.Text, emit func(Event) error) error {
	if session == nil || u.Catalog == nil || u.Store == nil {
		return errors.New("start turn requires session, catalog and store")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	toolsCtx, toolsSpan := StartDiagnosticSpan(ctx, u.Continue.Diagnostics, DiagnosticActionTools, DiagnosticEvent{})
	tools, err := u.Catalog.Snapshot(toolsCtx)
	class := DiagnosticErrorNone
	if err != nil {
		class = DiagnosticErrorTool
		if errors.Is(err, context.Canceled) {
			class = DiagnosticErrorCancelled
		} else if errors.Is(err, context.DeadlineExceeded) {
			class = DiagnosticErrorTimeout
		}
	}
	toolsSpan.End(class)
	if err != nil {
		return err
	}
	next := *session
	if u.Continue.Ceremonies != nil {
		// A console step interrupted earlier (a cancelled watch, a crash
		// after propose) continues before the model is asked anything.
		resumed, ok, err := u.Continue.Ceremonies.Resume(ctx, next)
		if err != nil {
			return err
		}
		if ok {
			switch {
			case resumed.Accepted && resumed.Run == nil:
				next.FinishCeremony()
			case resumed.Run != nil:
				if err := next.SetCeremony(*resumed.Run); err != nil {
					return err
				}
			}
			prompt = root.Text(string(prompt) + "\n\n[AXLR] Ceremony console step resumed before this turn: " + string(resumed.Outcome.Content))
		}
	}
	if u.Notices != nil {
		// A repair that ended while this session worked is reported with the
		// next prompt, as a visible console note, never as a hidden fact.
		notices, err := u.Notices.Drain(ctx, next.Export().ID)
		if err != nil {
			return err
		}
		for _, notice := range notices {
			prompt = root.Text(string(prompt) + "\n\n" + notice)
		}
	}
	if _, live := next.Ceremony(); next.Mode().StartsCeremony() && !live {
		// The ceremony starts before the turn so its first step instruction
		// reaches the model's first request.
		if err := u.Continue.Ceremonies.Begin(ctx, &next, prompt); err != nil {
			return err
		}
	}
	if err := next.BeginTurn(prompt, tools); err != nil {
		return err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*session = next
	return (AgentTurnUseCase{Continue: u.Continue, Tools: u.Tools, Approval: u.Approval}).Execute(ctx, session, emit)
}
