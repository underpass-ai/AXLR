package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const sessionInstanceVersion = 1

// maxSessionInstanceBytes bounds <id>.instance. It covers what the domain
// bounds: LastRecord at 16 KiB (twice that escaped as a string), fifty
// pending entries of about 9 KiB each (eight evidence lines of 1000 bytes;
// up to seven times that once JSON escapes them twice, a "<" in a command
// line becoming \\u003c) and eight recalled lines of 300 bytes. At 64 KiB a
// full queue was written and then read as no sidecar, losing the entries,
// the memory context and the fence (14 October 2026); a sidecar over the
// bound is trimmed when written (fitInstance).
const maxSessionInstanceBytes = 4 << 20

// sessionInstance is the session's MADE instance kept in <id>.instance,
// beside the snapshot for the same reason as <id>.ceremony: a pointer to a
// durable instance, read leniently.
type sessionInstance struct {
	Version       int      `json:"version"`
	Instance      string   `json:"instance"`
	Turns         int      `json:"turns"`
	Fence         string   `json:"fence,omitempty"`
	TurnIndex     int      `json:"turn_index"`
	ClaimRequests int      `json:"claim_requests,omitempty"`
	ClaimCost     float64  `json:"claim_cost,omitempty"`
	Closed        bool     `json:"closed,omitempty"`
	LastRecord    string   `json:"last_record,omitempty"`
	Unavailable   string   `json:"unavailable,omitempty"`
	Pending       []string `json:"pending,omitempty"`
	// The memory ids keep the keys the sidecars on disk were written
	// with before the engine names left the domain (14 October 2026).
	MemoryAgent   string   `json:"kmp_agent,omitempty"`
	MemoryContext string   `json:"kmp_context,omitempty"`
	Recalled      string   `json:"recalled,omitempty"`
	OperationID   string   `json:"operation_id,omitempty"`
	ClaimAttempt  int      `json:"claim_attempt,omitempty"`
	ClaimedAt     string   `json:"claimed_at,omitempty"`
	LeaseMS       int64    `json:"lease_ms,omitempty"`
	ReportedAt    string   `json:"reported_at,omitempty"`
	Reports       int      `json:"reports,omitempty"`
	Incarnation   string   `json:"claim_incarnation,omitempty"`
	Recalled2     []string `json:"recalled_memories,omitempty"`
	Predecessor   string   `json:"predecessor,omitempty"`
}

func (s *SessionStore) instancePath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".instance")
}

// readInstance returns nil when the file is missing or unreadable.
func (s *SessionStore) readInstance(id domain.SessionID) *domain.SessionInstance {
	file, err := openNoFollow(s.instancePath(id), os.O_RDONLY, 0)
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionInstanceBytes+1))
	if err != nil || len(data) > maxSessionInstanceBytes {
		return nil
	}
	var record sessionInstance
	if json.NewDecoder(bytes.NewReader(data)).Decode(&record) != nil || record.Version != sessionInstanceVersion {
		return nil
	}
	instance := domain.SessionInstance{Instance: record.Instance, Turns: record.Turns, Fence: record.Fence, TurnIndex: record.TurnIndex, ClaimRequests: record.ClaimRequests, ClaimCost: record.ClaimCost, Closed: record.Closed, LastRecord: record.LastRecord, Unavailable: record.Unavailable, Pending: record.Pending, MemoryAgent: domain.MemoryAgentID(record.MemoryAgent), MemoryContext: domain.MemoryContextID(record.MemoryContext), Recalled: record.Recalled, OperationID: record.OperationID, ClaimAttempt: record.ClaimAttempt, ClaimedAt: parseInstant(record.ClaimedAt), LeaseMS: record.LeaseMS, Reported: parseInstant(record.ReportedAt), Reports: record.Reports, ClaimIncarnation: record.Incarnation, RecalledMemories: record.Recalled2, Predecessor: domain.SessionID(record.Predecessor)}
	if instance.Validate() != nil {
		return nil
	}
	return &instance
}

func (s *SessionStore) writeInstance(id domain.SessionID, instance *domain.SessionInstance) error {
	if instance == nil {
		if err := os.Remove(s.instancePath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := fitInstance(sessionInstance{Version: sessionInstanceVersion, Instance: instance.Instance, Turns: instance.Turns, Fence: instance.Fence, TurnIndex: instance.TurnIndex, ClaimRequests: instance.ClaimRequests, ClaimCost: instance.ClaimCost, Closed: instance.Closed, LastRecord: instance.LastRecord, Unavailable: instance.Unavailable, Pending: instance.Pending, MemoryAgent: string(instance.MemoryAgent), MemoryContext: string(instance.MemoryContext), Recalled: instance.Recalled, OperationID: instance.OperationID, ClaimAttempt: instance.ClaimAttempt, ClaimedAt: formatInstant(instance.ClaimedAt), LeaseMS: instance.LeaseMS, ReportedAt: formatInstant(instance.Reported), Reports: instance.Reports, Incarnation: instance.ClaimIncarnation, Recalled2: instance.RecalledMemories, Predecessor: string(instance.Predecessor)})
	if err != nil {
		return err
	}
	return s.writeSidecar(".instance-*", s.instancePath(id), data)
}

// fitInstance encodes the sidecar within the bound readInstance reads: the
// last record goes first (it is for /session), then the oldest pending
// entries, so what is written is always read back.
func fitInstance(record sessionInstance) ([]byte, error) {
	for {
		data, err := json.Marshal(record)
		if err != nil || len(data) <= maxSessionInstanceBytes {
			return data, err
		}
		switch {
		case record.LastRecord != "":
			record.LastRecord = ""
		case len(record.Pending) > 0:
			record.Pending = record.Pending[1:]
		default:
			return nil, errors.New("session instance is too large")
		}
	}
}

// parseInstant reads an RFC 3339 instant, zero when absent or unreadable.
func parseInstant(text string) time.Time {
	if text == "" {
		return time.Time{}
	}
	instant, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}
	}
	return instant
}

// formatInstant writes an instant as RFC 3339, "" when zero.
func formatInstant(instant time.Time) string {
	if instant.IsZero() {
		return ""
	}
	return instant.UTC().Format(time.RFC3339Nano)
}
