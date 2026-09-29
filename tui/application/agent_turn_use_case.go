package application

import (
	"context"
	"errors"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// AgentTurnUseCase advances until a final answer or a human decision is needed.
// ContinueTurnUseCase remains responsible for exactly one model stream.
type AgentTurnUseCase struct{ Continue ContinueTurnUseCase }

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
		if err := rejectUnknown(ctx, s, u.Continue.Store, emit); err != nil {
			return err
		}
		if s.Status() != domain.StatusStreaming {
			return nil
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
func rejectUnknown(ctx context.Context, s *domain.Session, store SessionStorePort, emit func(Event) error) error {
	for s.Status() == domain.StatusApproval && len(s.Pending()) > 0 {
		p := s.Pending()[0]
		if _, known := findTool(s, p.Call.Name); known {
			return nil
		}
		next := *s
		if err := next.RecordToolOutcome(p.Call.ID, domain.DecisionDeny, domain.ToolOutcome{Content: root.Text(fmt.Sprintf("unknown tool %q rejected", p.Call.Name)), IsError: true}); err != nil {
			return err
		}
		if err := store.Save(ctx, next); err != nil {
			return err
		}
		*s = next
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
