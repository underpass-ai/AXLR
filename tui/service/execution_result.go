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

// executionReceipt is the runtime contract's receipt of a session as it
// stands, read from its journal and its ledger; nil, like the result,
// until a turn ends.
func (s *Server) executionReceipt(session domain.Session) *dto.ExecutionReceipt {
	if status := session.Status(); status == domain.StatusIdle || status == domain.StatusStreaming {
		return nil
	}
	read := domain.ReceiptSession{Session: session}
	if s.base != nil {
		if ledger, err := s.base.LoadUsage(context.Background(), session.ID()); err == nil {
			read.Ledger = ledger
		}
		read.Entries, _ = s.base.LoadReceipt(context.Background(), session.ID())
	}
	result := domain.NewExecutionResult(session, nil)
	wire := contract.ExecutionReceipt(domain.NewExecutionReceipt(result, []domain.ReceiptSession{read}))
	return &wire
}

// executionBudget is the session's budget as GetSession shows it: the
// limits the execution was given, nil for a session without any.
func executionBudget(session domain.Session) *dto.ExecutionBudget {
	account, has := session.Budget()
	if !has {
		return nil
	}
	wire := contract.ExecutionBudget(account.Budget)
	return &wire
}

// executionCorrelation is the session's correlation, nil without one.
// executionSandbox is the session's sandbox on the wire, null without.
func executionSandbox(session domain.Session) *dto.ExecutionSandbox {
	sandbox, has := session.Sandbox()
	if !has {
		return nil
	}
	wire := contract.ExecutionSandbox(sandbox)
	return &wire
}

func executionRouting(session domain.Session) *dto.ExecutionRouting {
	routing, has := session.Routing()
	if !has {
		return nil
	}
	wire := contract.ExecutionRouting(routing)
	return &wire
}

func executionCorrelation(session domain.Session) *dto.ExecutionCorrelation {
	correlation, has := session.Correlation()
	if !has {
		return nil
	}
	wire := contract.ExecutionCorrelation(correlation)
	return &wire
}

// correlated adds the session's correlation to a journal event's payload,
// so every event of an execution carries its ids (the runtime contract).
func correlated(payload map[string]any, session *domain.Session) map[string]any {
	if session == nil {
		return payload
	}
	if correlation := executionCorrelation(*session); correlation != nil {
		payload["correlation"] = correlation
	}
	return payload
}

// budgetStopped reports an error that is the execution's budget ending
// the turn: the loop journaled the turn's end with the result already,
// so no operation.failed follows.
func budgetStopped(err error) bool {
	var stop *domain.BudgetExceededError
	return errors.As(err, &stop)
}

// executionPolicy is the session's policy as GetSession shows it, nil
// for a session without one.
func executionPolicy(session domain.Session) *dto.ExecutionPolicy {
	policy, has := session.Policy()
	if !has {
		return nil
	}
	wire := contract.ExecutionPolicy(policy)
	return &wire
}
