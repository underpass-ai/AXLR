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

// maxSessionInstanceBytes bounds <id>.instance; the last turn record is
// capped before it is stored.
const maxSessionInstanceBytes = 64 << 10

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
	KMPAgent      string   `json:"kmp_agent,omitempty"`
	KMPContext    string   `json:"kmp_context,omitempty"`
	Recalled      string   `json:"recalled,omitempty"`
	OperationID   string   `json:"operation_id,omitempty"`
	ClaimedAt     string   `json:"claimed_at,omitempty"`
	LeaseMS       int64    `json:"lease_ms,omitempty"`
	ReportedAt    string   `json:"reported_at,omitempty"`
	Reports       int      `json:"reports,omitempty"`
	Incarnation   string   `json:"claim_incarnation,omitempty"`
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
	instance := domain.SessionInstance{Instance: record.Instance, Turns: record.Turns, Fence: record.Fence, TurnIndex: record.TurnIndex, ClaimRequests: record.ClaimRequests, ClaimCost: record.ClaimCost, Closed: record.Closed, LastRecord: record.LastRecord, Unavailable: record.Unavailable, Pending: record.Pending, KMPAgent: record.KMPAgent, KMPContext: record.KMPContext, Recalled: record.Recalled, OperationID: record.OperationID, ClaimedAt: parseInstant(record.ClaimedAt), LeaseMS: record.LeaseMS, Reported: parseInstant(record.ReportedAt), Reports: record.Reports, ClaimIncarnation: record.Incarnation}
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
	record := sessionInstance{Version: sessionInstanceVersion, Instance: instance.Instance, Turns: instance.Turns, Fence: instance.Fence, TurnIndex: instance.TurnIndex, ClaimRequests: instance.ClaimRequests, ClaimCost: instance.ClaimCost, Closed: instance.Closed, LastRecord: instance.LastRecord, Unavailable: instance.Unavailable, Pending: instance.Pending, KMPAgent: instance.KMPAgent, KMPContext: instance.KMPContext, Recalled: instance.Recalled, OperationID: instance.OperationID, ClaimedAt: formatInstant(instance.ClaimedAt), LeaseMS: instance.LeaseMS, ReportedAt: formatInstant(instance.Reported), Reports: instance.Reports, Incarnation: instance.ClaimIncarnation}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.writeSidecar(".instance-*", s.instancePath(id), data)
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
