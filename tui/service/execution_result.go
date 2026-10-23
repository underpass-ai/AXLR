package service

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/tui/adapters/contract"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// executionResult is the runtime contract's result of a session as it
// stands (docs/runtime-contract.md), with the session's ledger when the
// store keeps one. A failure names the operation's error code, never its
// message: the service keeps internal errors out of its events. Without a
// failure, a session that has not finished a turn has no result yet.
func (s *Server) executionResult(session domain.Session, failure string) *dto.ExecutionResult {
	var err error
	if failure != "" {
		err = errors.New(failure)
	} else if status := session.Status(); status == domain.StatusIdle || status == domain.StatusStreaming {
		return nil
	}
	result := domain.NewExecutionResult(session, err)
	if s.base != nil {
		if ledger, loadErr := s.base.LoadUsage(context.Background(), session.ID()); loadErr == nil {
			result.AddUsage(ledger)
		}
	}
	wire := contract.ExecutionResult(result)
	return &wire
}
