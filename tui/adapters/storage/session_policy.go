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

const sessionPolicyVersion = 1

// sessionPolicy is kept in <id>.policy beside the snapshot, like <id>.mode,
// so older consoles keep loading the session. Unlike the mode, a policy
// that cannot be read refuses the session: a lost or corrupt file must
// not widen what the execution may do.
type sessionPolicy struct {
	Version int                 `json:"version"`
	Policy  dto.ExecutionPolicy `json:"policy"`
}

func (s *SessionStore) policyPath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".policy")
}

// readPolicy returns nil when the file is missing and an error when it is
// present but unreadable, oversized or invalid.
func (s *SessionStore) readPolicy(id domain.SessionID) (*domain.ExecutionPolicy, error) {
	file, err := openNoFollow(s.policyPath(id), os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the session's policy cannot be read: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, contract.MaxExecutionPolicyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("the session's policy cannot be read: %w", err)
	}
	if len(data) > contract.MaxExecutionPolicyBytes {
		return nil, errors.New("the session's policy exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record sessionPolicy
	if err := decoder.Decode(&record); err != nil || record.Version != sessionPolicyVersion {
		return nil, errors.New("the session's policy file is not one this console reads")
	}
	policy, err := contract.DomainExecutionPolicy(record.Policy)
	if err != nil {
		return nil, fmt.Errorf("the session's policy is invalid: %w", err)
	}
	return &policy, nil
}

func (s *SessionStore) writePolicy(id domain.SessionID, policy *domain.ExecutionPolicy) error {
	if policy == nil {
		if err := os.Remove(s.policyPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.Marshal(sessionPolicy{Version: sessionPolicyVersion, Policy: contract.ExecutionPolicy(*policy)})
	if err != nil {
		return err
	}
	return s.writeSidecar(".policy-*", s.policyPath(id), data)
}
