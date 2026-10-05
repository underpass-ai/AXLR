package application

import (
	"context"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// UseCaseWorkbench is RepairWorkbench over the turn use cases, built by the
// console for one clone: the same code path a person's session runs, with
// the tools rooted in the clone.
type UseCaseWorkbench struct {
	Start    StartTurnUseCase
	Resolver ResolveToolUseCase
	Agent    AgentTurnUseCase
	Closer   func() error
}

var _ RepairWorkbench = (*UseCaseWorkbench)(nil)

// Observe attaches the observer to the shared ceremony driver.
func (w *UseCaseWorkbench) Observe(observer CeremonyObserverPort) {
	if w.Start.Continue.Ceremonies != nil {
		w.Start.Continue.Ceremonies.Observer = observer
	}
}

func (w *UseCaseWorkbench) Begin(ctx context.Context, s *domain.Session, prompt root.Text, emit func(Event) error) error {
	return w.Start.Execute(ctx, s, prompt, emit)
}

func (w *UseCaseWorkbench) Resolve(ctx context.Context, s *domain.Session, id root.ToolCallID, decision domain.ToolDecision, emit func(Event) error) error {
	return w.Resolver.Execute(ctx, s, id, decision, emit)
}

func (w *UseCaseWorkbench) Decide(ctx context.Context, s *domain.Session, approve bool, reason string, emit func(Event) error) error {
	return w.Start.Decide(ctx, s, approve, reason, emit)
}

// Continue resumes an interrupted session: a console step first, from what
// MADE recorded, then the turn. Pending calls go back to approval, never to
// execution; completed effects are not replayed.
func (w *UseCaseWorkbench) Continue(ctx context.Context, s *domain.Session, emit func(Event) error) error {
	if s == nil || w.Start.Store == nil {
		return errors.New("continue needs a session and a store")
	}
	if emit == nil {
		emit = func(Event) error { return nil }
	}
	next := *s
	if driver := w.Start.Continue.Ceremonies; driver != nil {
		resumed, ok, err := driver.Resume(ctx, next)
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
			if err := w.Start.Store.Save(ctx, next); err != nil {
				return err
			}
			*s = next
		}
	}
	if next.Status() == domain.StatusInterrupted {
		var err error
		if len(next.Pending()) > 0 {
			err = next.ResumePending()
		} else {
			err = next.ResumeTurn()
		}
		if err != nil {
			return err
		}
		if err := w.Start.Store.Save(ctx, next); err != nil {
			return err
		}
		*s = next
	}
	if s.Status() != domain.StatusStreaming && s.Status() != domain.StatusApproval {
		return nil // nothing runs: the loop reads the session's state
	}
	return w.Agent.Execute(ctx, s, emit)
}

func (w *UseCaseWorkbench) Close() error {
	if w.Closer != nil {
		return w.Closer()
	}
	return nil
}
