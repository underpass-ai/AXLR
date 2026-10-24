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
	maxSessionExecutionBytes = 16 << 10
)

// sessionExecution is kept in <id>.execution beside the snapshot, like
// <id>.policy: the execution's budget with its account, and its
// correlation. Like the policy, a file that cannot be read refuses the
// session: a lost budget must not widen what the execution may spend.
type sessionExecution struct {
	Version     int                       `json:"version"`
	Budget      *sessionBudget            `json:"budget,omitempty"`
	Correlation *dto.ExecutionCorrelation `json:"correlation,omitempty"`
}

func (s *SessionStore) executionPath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".execution")
}

// readExecution returns nil values when the file is missing and an error
// when it is present but unreadable, oversized or invalid.
func (s *SessionStore) readExecution(id domain.SessionID) (*domain.ExecutionAccount, *domain.ExecutionCorrelation, error) {
	file, err := openNoFollow(s.executionPath(id), os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("the session's execution file cannot be read: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionExecutionBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("the session's execution file cannot be read: %w", err)
	}
	if len(data) > maxSessionExecutionBytes {
		return nil, nil, errors.New("the session's execution file exceeds 16 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record sessionExecution
	if err := decoder.Decode(&record); err != nil || record.Version != sessionExecutionVersion {
		return nil, nil, errors.New("the session's execution file is not one this console reads")
	}
	var account *domain.ExecutionAccount
	if record.Budget != nil {
		limits, err := contract.DomainExecutionBudget(record.Budget.Limits)
		if err != nil {
			return nil, nil, fmt.Errorf("the session's budget is invalid: %w", err)
		}
		account = &domain.ExecutionAccount{Budget: limits, StartedAt: record.Budget.StartedAt, Carried: record.Budget.Carried.domain(), Base: record.Budget.Base.domain(), Stopped: domain.BudgetLimit(record.Budget.Stopped)}
		if err := account.Validate(); err != nil {
			return nil, nil, fmt.Errorf("the session's budget is invalid: %w", err)
		}
	}
	var correlation *domain.ExecutionCorrelation
	if record.Correlation != nil {
		parsed, err := contract.DomainExecutionCorrelation(*record.Correlation)
		if err != nil {
			return nil, nil, fmt.Errorf("the session's correlation is invalid: %w", err)
		}
		correlation = &parsed
	}
	return account, correlation, nil
}

func (s *SessionStore) writeExecution(id domain.SessionID, account *domain.ExecutionAccount, correlation *domain.ExecutionCorrelation) error {
	if account == nil && correlation == nil {
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
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.writeSidecar(".execution-*", s.executionPath(id), data)
}
