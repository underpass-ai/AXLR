package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// InstallID is this install's stable id, kept in <dir>/install-id: 32 hex
// characters drawn once. It names the console's KMP agent registration
// (axlr:<install-id>) and its MADE host identity.
func InstallID(dir string) (string, error) {
	path := filepath.Join(dir, "install-id")
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); isHexID(id) {
			return id, nil
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func isHexID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// KMPAgentStore keeps, per KMP store, the agent the console registered
// there (<state>/axlr/kmp-agents.json): KMP keeps agents inside the store,
// so each store has its own, and a lost entry is recovered by registering
// the same key again.
type KMPAgentStore struct {
	path string
	mu   sync.Mutex
}

type kmpAgentsRecord struct {
	Version int                      `json:"version"`
	Agents  map[string]kmpAgentEntry `json:"agents"`
}

type kmpAgentEntry struct {
	AgentID         string `json:"agent_id"`
	RegistrationKey string `json:"registration_key"`
	RegisteredAt    string `json:"registered_at"`
}

func NewKMPAgentStore(path string) *KMPAgentStore { return &KMPAgentStore{path: path} }

// Agent is the agent registered in a store, if any.
func (s *KMPAgentStore) Agent(_ context.Context, store string) (string, bool) {
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
func (s *KMPAgentStore) Remember(_ context.Context, store, agentID, registrationKey string) error {
	if store == "" || agentID == "" {
		return errors.New("a store and an agent id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.read()
	if err != nil {
		return err
	}
	record.Agents[filepath.Clean(store)] = kmpAgentEntry{AgentID: agentID, RegistrationKey: registrationKey, RegisteredAt: time.Now().UTC().Format(time.RFC3339)}
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

func (s *KMPAgentStore) read() (kmpAgentsRecord, error) {
	record := kmpAgentsRecord{Version: 1, Agents: map[string]kmpAgentEntry{}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return record, nil
	}
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil || record.Version != 1 {
		return kmpAgentsRecord{Version: 1, Agents: map[string]kmpAgentEntry{}}, nil
	}
	if record.Agents == nil {
		record.Agents = map[string]kmpAgentEntry{}
	}
	return record, nil
}
