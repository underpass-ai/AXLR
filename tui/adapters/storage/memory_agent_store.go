package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemoryAgentStore keeps, per memory store, the agent the console
// registered there (application.MemoryAgentsPort; the file is still
// <state>/axlr/kmp-agents.json): the engine keeps agents inside the
// store, so each store has its own, and a lost entry is recovered by
// registering the same key again.
type MemoryAgentStore struct {
	path string
	mu   sync.Mutex
}

func NewMemoryAgentStore(path string) *MemoryAgentStore { return &MemoryAgentStore{path: path} }

// Agent is the agent registered in a store, if any.
func (s *MemoryAgentStore) Agent(_ context.Context, store string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.read()
	if err != nil {
		return "", false
	}
	entry, ok := record.Agents[filepath.Clean(store)]
	return entry.AgentID, ok && entry.AgentID != ""
}

// Remember records the agent registered in a store.
func (s *MemoryAgentStore) Remember(_ context.Context, store, agentID, registrationKey string) error {
	if store == "" || agentID == "" {
		return errors.New("a store and an agent id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.read()
	if err != nil {
		return err
	}
	record.Agents[filepath.Clean(store)] = memoryAgentEntry{AgentID: agentID, RegistrationKey: registrationKey, RegisteredAt: time.Now().UTC().Format(time.RFC3339)}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, s.path)
}

func (s *MemoryAgentStore) read() (memoryAgentsRecord, error) {
	record := memoryAgentsRecord{Version: 1, Agents: map[string]memoryAgentEntry{}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return record, nil
	}
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil || record.Version != 1 {
		return memoryAgentsRecord{Version: 1, Agents: map[string]memoryAgentEntry{}}, nil
	}
	if record.Agents == nil {
		record.Agents = map[string]memoryAgentEntry{}
	}
	return record, nil
}
