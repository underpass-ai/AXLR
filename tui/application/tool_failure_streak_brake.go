package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// stopFailureStreak ends the turn, before its next request, when its last
// limit answers failed alike (domain.Session.ToolFailureStreak): the
// turn is interrupted and saved as it stands, the state told, and a
// domain.ToolFailureStreakError goes back to end the execution failed. A
// limit of zero never stops: the console's person sees each failure.
func stopFailureStreak(ctx context.Context, store SessionStorePort, s *domain.Session, limit int, emit func(Event) error) error {
	if limit <= 0 {
		return nil
	}
	streak := s.ToolFailureStreak()
	if streak.Count < limit {
		return nil
	}
	stop := &domain.ToolFailureStreakError{Tool: streak.Tool, Count: streak.Count, Failure: streak.Failure}
	next := *s
	if err := next.PauseTurn(); err != nil {
		return joinCause(stop, err)
	}
	if err := store.Save(context.WithoutCancel(ctx), next); err != nil {
		return joinCause(stop, err)
	}
	*s = next
	return joinCause(stop, emit(Event{Kind: EventState, State: next.Status()}))
}
