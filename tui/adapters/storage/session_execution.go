package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/adapters/contract"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

const (
	sessionExecutionVersion  = 1
	maxSessionExecutionBytes = 64 << 10
)

// sessionExecution is kept in <id>.execution beside the snapshot, like
// <id>.policy: the execution's budget with its account, its correlation
// and its sandbox. Like the policy, a file that cannot be read refuses the
// session: a lost budget must not widen what the execution may spend, nor
// a lost sandbox what its commands may reach.
type sessionExecution struct {
	Version     int                       `json:"version"`
	Budget      *sessionBudget            `json:"budget,omitempty"`
	Correlation *dto.ExecutionCorrelation `json:"correlation,omitempty"`
	Sandbox     *dto.ExecutionSandbox     `json:"sandbox,omitempty"`
}

func (s *SessionStore) executionPath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".execution")
}

// readExecution returns nil values when the file is missing and an error
// when it is present but unreadable, oversized or invalid.
func (s *SessionStore) readExecution(id domain.SessionID) (*domain.ExecutionAccount, *domain.ExecutionCorrelation, *domain.ExecutionSandbox, error) {
	file, err := openNoFollow(s.executionPath(id), os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("the session's execution file cannot be read: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionExecutionBytes+1))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("the session's execution file cannot be read: %w", err)
	}
	if len(data) > maxSessionExecutionBytes {
		return nil, nil, nil, errors.New("the session's execution file exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record sessionExecution
	if err := decoder.Decode(&record); err != nil || record.Version != sessionExecutionVersion {
		return nil, nil, nil, errors.New("the session's execution file is not one this console reads")
	}
	var account *domain.ExecutionAccount
	if record.Budget != nil {
		limits, err := contract.DomainExecutionBudget(record.Budget.Limits)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("the session's budget is invalid: %w", err)
		}
		account = &domain.ExecutionAccount{Budget: limits, StartedAt: record.Budget.StartedAt, Carried: record.Budget.Carried.domain(), Base: record.Budget.Base.domain(), Stopped: domain.BudgetLimit(record.Budget.Stopped)}
		if err := account.Validate(); err != nil {
			return nil, nil, nil, fmt.Errorf("the session's budget is invalid: %w", err)
		}
	}
	var correlation *domain.ExecutionCorrelation
	if record.Correlation != nil {
		parsed, err := contract.DomainExecutionCorrelation(*record.Correlation)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("the session's correlation is invalid: %w", err)
		}
		correlation = &parsed
	}
	var sandbox *domain.ExecutionSandbox
	if record.Sandbox != nil {
		parsed, err := contract.DomainExecutionSandbox(*record.Sandbox)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("the session's sandbox is invalid: %w", err)
		}
		sandbox = &parsed
	}
	return account, correlation, sandbox, nil
}

func (s *SessionStore) writeExecution(id domain.SessionID, account *domain.ExecutionAccount, correlation *domain.ExecutionCorrelation, sandbox *domain.ExecutionSandbox) error {
	if account == nil && correlation == nil && sandbox == nil {
		if err := os.Remove(s.executionPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	record := sessionExecution{Version: sessionExecutionVersion}
	if account != nil {
		record.Budget = &sessionBudget{Limits: contract.ExecutionBudget(account.Budget), StartedAt: account.StartedAt.UTC(), Carried: spendRecord(account.Carried), Base: spendRecord(account.Base), Stopped: string(account.Stopped)}
	}
	if correlation != nil {
		wire := contract.ExecutionCorrelation(*correlation)
		record.Correlation = &wire
	}
	if sandbox != nil {
		wire := contract.ExecutionSandbox(*sandbox)
		record.Sandbox = &wire
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.writeSidecar(".execution-*", s.executionPath(id), data)
}
