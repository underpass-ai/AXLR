package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type toolCall struct {
	ID       string              `json:"call_id"`
	Owner    string              `json:"owner"`
	Tool     string              `json:"tool"`
	Identity domain.ToolIdentity `json:"identity"`
	Args     json.RawMessage     `json:"arguments"`
	Status   string              `json:"status"`
	Revision uint64              `json:"revision"`
	Decision string              `json:"decision,omitempty"`
	Result   *domain.ToolOutcome `json:"result,omitempty"`
}

type callStore struct {
	mu  sync.Mutex
	dir string
}

func newCallStore(dir string) (*callStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &callStore{dir: dir}, nil
}

func (s *callStore) path(id string) string { return filepath.Join(s.dir, id+".json") }

func (s *callStore) Load(id string) (toolCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

func (s *callStore) read(id string) (toolCall, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return toolCall{}, err
	}
	var call toolCall
	if err := json.Unmarshal(data, &call); err != nil {
		return toolCall{}, err
	}
	if call.ID != id {
		return toolCall{}, errors.New("call ID mismatch")
	}
	return call, nil
}

func (s *callStore) Save(call toolCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := domain.NewSessionID(call.ID); err != nil {
		return err
	}
	if prior, err := s.read(call.ID); err == nil {
		if prior.Owner != call.Owner || prior.Tool != call.Tool || prior.Revision+1 != call.Revision {
			return errors.New("invalid call transition")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else if call.Revision != 1 {
		return errors.New("new call must have revision 1")
	}
	data, err := json.Marshal(call)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".call-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), s.path(call.ID)); err != nil {
		return err
	}
	d, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	d.Close()
	return err
}

// Recover preserves unknown effects for inspection without retrying them.
func (s *callStore) Recover() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-5]
		if _, err := domain.NewSessionID(id); err != nil {
			continue
		}
		call, err := s.Load(id)
		if err != nil {
			return err
		}
		if call.Status == "running" || (call.Status == "pending_approval" && call.Decision == "approve") {
			call.Status = "uncertain"
			call.Revision++
			call.Result = &domain.ToolOutcome{Content: "execution state after restart is unknown", IsError: true, Uncertain: true}
			if err := s.Save(call); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) runDirectCall(id string) {
	select {
	case s.directSlots <- struct{}{}:
	case <-s.root.Done():
		return
	}
	defer func() { <-s.directSlots }()
	lock := s.callLock(id)
	lock.Lock()
	defer lock.Unlock()
	call, err := s.calls.Load(id)
	if err != nil {
		return
	}
	if call.Status != "pending_approval" || call.Decision != "approve" {
		return
	}
	// A persisted uncertain checkpoint prevents automatic replay after a crash.
	call.Status = "running"
	call.Revision++
	call.Result = &domain.ToolOutcome{Content: "tool execution started; effect unknown", IsError: true, Uncertain: true}
	if s.calls.Save(call) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.root, 30*time.Second)
	defer cancel()
	args, err := root.NewJSONObject(call.Args)
	if err != nil {
		return
	}
	if s.deps.Tools == nil {
		return
	}
	result, runErr := s.deps.Tools.Execute(ctx, call.Identity, args)
	if runErr != nil {
		result = domain.ToolOutcome{Content: "tool execution failed; effect unknown", IsError: true, Uncertain: true}
	}
	if result.Uncertain {
		call.Status = "uncertain"
	} else if result.IsError {
		call.Status = "failed"
	} else {
		call.Status = "completed"
	}
	call.Result = &result
	call.Revision++
	_ = s.calls.Save(call)
}
