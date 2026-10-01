package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// auditRecord intentionally has no argument, prompt, transcript, result or
// credential field. Every record is synced before its action can continue.
type auditRecord struct {
	Time        time.Time `json:"time"`
	Principal   string    `json:"principal"`
	RequestID   string    `json:"request_id,omitempty"`
	Action      string    `json:"action"`
	Tool        string    `json:"tool,omitempty"`
	Decision    string    `json:"decision,omitempty"`
	Status      string    `json:"status"`
	SessionID   string    `json:"session_id,omitempty"`
	CallID      string    `json:"call_id,omitempty"`
	OperationID string    `json:"operation_id,omitempty"`
}

type auditStore struct {
	mu   sync.Mutex
	file *os.File
}

func newAuditStore(dir string) (*auditStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "audit.jsonl")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("audit log must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return nil, err
	}
	return &auditStore{file: file}, nil
}

func (s *auditStore) Append(record auditRecord) error {
	record.Time = time.Now().UTC()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return errors.New("audit store is closed")
	}
	n, err := s.file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return errors.New("short audit write")
	}
	return s.file.Sync()
}

func (s *auditStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}
