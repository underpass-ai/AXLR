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
	if u.Approval != nil {
		u.Continue.Approval = u.Approval
	}
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
	person := string(prompt)
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
	// A ceremony the person stopped since the last prompt is told once,
	// on this prompt; a mode that starts a new ceremony makes it moot.
	if stopped := next.TakeStoppedCeremony(); stopped != "" && !next.Mode().StartsCeremony() {
		prompt = root.Text(string(prompt) + "\n\n" + StoppedCeremonyNote(stopped))
	}
	// Files the person restored since the last answer are told once, on
	// this prompt, so the model does not take them for someone else's
	// change; the history itself stays as it was.
	if note := restoreNote(next.Export()); note != "" {
		prompt = root.Text(string(prompt) + "\n\n" + note)
	}
	began := false
	if _, live := next.Ceremony(); next.Mode().StartsCeremony() && !live {
		// The ceremony starts before the turn so its first step instruction
		// reaches the model's first request.
		if err := u.Continue.Ceremonies.Begin(ctx, &next, prompt); err != nil {
			return err
		}
		began = true
	}
	if run, live := next.Ceremony(); live {
		// A standard ceremony's step travels on the person's prompt, not in
		// the system prompt; the planner and compact steps keep theirs there.
		if _, _, focused := focusedRun(next); !focused {
			prompt = root.Text(string(prompt) + "\n\n" + CurrentStepNote(run, began))
		} else if began && run.Plan != nil {
			prompt = root.Text(string(prompt) + "\n\n" + planBriefNote)
		}
	}
	// The workspace's memory, recalled by the console, rides the first
	// prompt (and a prompt that changes topic) as a console note.
	if note := u.Continue.Session.Recall(ctx, &next, person); note != "" {
		prompt = root.Text(string(prompt) + "\n\n" + note)
	}
	if err := next.BeginTurn(prompt, tools); err != nil {
		return err
	}
	// The session's own MADE step is claimed before the model is asked.
	u.Continue.Session.BeginTurn(ctx, &next)
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*session = next
	return (AgentTurnUseCase{Continue: u.Continue, Tools: u.Tools, Approval: u.Approval}).Execute(ctx, session, emit)
}
