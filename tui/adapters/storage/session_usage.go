package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const sessionUsageVersion = 1
const maxSessionUsageBytes = 64 << 10

var _ application.SessionUsagePort = (*SessionStore)(nil)

// sessionUsage is a session's usage ledger, kept in <id>.usage.json beside
// its snapshot for the same reason as <id>.times: the snapshot rejects
// unknown fields, and older consoles must keep loading it. Durations are
// milliseconds and costs dollars.
type sessionUsage struct {
	Version          int                           `json:"version"`
	Requests         int                           `json:"requests"`
	PromptTokens     int                           `json:"prompt_tokens"`
	CachedTokens     int                           `json:"cached_tokens"`
	CacheWriteTokens int                           `json:"cache_write_tokens"`
	CompletionTokens int                           `json:"completion_tokens"`
	ReasoningTokens  int                           `json:"reasoning_tokens"`
	CostUSD          float64                       `json:"cost_usd"`
	CostRequests     int                           `json:"cost_requests"`
	Providers        map[string]providerUsageEntry `json:"providers,omitempty"`
	FirstByte        latencyEntry                  `json:"first_byte"`
	Total            latencyEntry                  `json:"total"`
	RaisedUSD        float64                       `json:"raised_usd,omitempty"`
	Warned           bool                          `json:"warned,omitempty"`
}

type providerUsageEntry struct {
	Requests         int     `json:"requests"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	CostRequests     int     `json:"cost_requests"`
}

type latencyEntry struct {
	Count int   `json:"count"`
	SumMS int64 `json:"sum_ms"`
	MaxMS int64 `json:"max_ms"`
}

func (s *SessionStore) usagePath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".usage.json")
}

// LoadUsage returns the session's ledger; an empty one when it has none.
func (s *SessionStore) LoadUsage(ctx context.Context, id domain.SessionID) (domain.UsageLedger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return domain.UsageLedger{}, err
	}
	if _, err := domain.NewSessionID(string(id)); err != nil {
		return domain.UsageLedger{}, err
	}
	return s.readUsage(id)
}

// UpdateUsage applies update to the saved ledger and replaces the file.
func (s *SessionStore) UpdateUsage(ctx context.Context, id domain.SessionID, update func(*domain.UsageLedger)) (domain.UsageLedger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return domain.UsageLedger{}, err
	}
	if _, err := domain.NewSessionID(string(id)); err != nil {
		return domain.UsageLedger{}, err
	}
	ledger, err := s.readUsage(id)
	if err != nil {
		return domain.UsageLedger{}, err
	}
	update(&ledger)
	data, err := json.Marshal(usageRecord(ledger))
	if err != nil {
		return domain.UsageLedger{}, err
	}
	if err := s.writeSidecar(".usage-*", s.usagePath(id), data); err != nil {
		return domain.UsageLedger{}, err
	}
	return ledger, nil
}

// readUsage fails only when the file cannot be read. A missing file is an
// empty ledger, and so is a damaged one or one from another version, as a
// damaged calibration file is: it only loses what was counted.
func (s *SessionStore) readUsage(id domain.SessionID) (domain.UsageLedger, error) {
	file, err := openNoFollow(s.usagePath(id), os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return domain.UsageLedger{}, nil
	}
	if err != nil {
		return domain.UsageLedger{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionUsageBytes+1))
	if err != nil {
		return domain.UsageLedger{}, err
	}
	var record sessionUsage
	if len(data) > maxSessionUsageBytes || json.Unmarshal(data, &record) != nil || record.Version != sessionUsageVersion {
		return domain.UsageLedger{}, nil
	}
	ledger := domain.UsageLedger{
		Requests: record.Requests, PromptTokens: record.PromptTokens, CachedTokens: record.CachedTokens, CacheWriteTokens: record.CacheWriteTokens,
		CompletionTokens: record.CompletionTokens, ReasoningTokens: record.ReasoningTokens, Cost: record.CostUSD, CostRequests: record.CostRequests,
		FirstByte: record.FirstByte.stats(), Total: record.Total.stats(), Raised: record.RaisedUSD, Warned: record.Warned,
	}
	if len(record.Providers) > 0 {
		ledger.Providers = make(map[string]domain.ProviderUsage, len(record.Providers))
		for name, p := range record.Providers {
			ledger.Providers[name] = domain.ProviderUsage{Requests: p.Requests, PromptTokens: p.PromptTokens, CompletionTokens: p.CompletionTokens, Cost: p.CostUSD, CostRequests: p.CostRequests}
		}
	}
	return ledger, nil
}

func usageRecord(ledger domain.UsageLedger) sessionUsage {
	record := sessionUsage{
		Version: sessionUsageVersion, Requests: ledger.Requests, PromptTokens: ledger.PromptTokens, CachedTokens: ledger.CachedTokens, CacheWriteTokens: ledger.CacheWriteTokens,
		CompletionTokens: ledger.CompletionTokens, ReasoningTokens: ledger.ReasoningTokens, CostUSD: ledger.Cost, CostRequests: ledger.CostRequests,
		FirstByte: latency(ledger.FirstByte), Total: latency(ledger.Total), RaisedUSD: ledger.Raised, Warned: ledger.Warned,
	}
	if len(ledger.Providers) > 0 {
		record.Providers = make(map[string]providerUsageEntry, len(ledger.Providers))
		for name, p := range ledger.Providers {
			record.Providers[name] = providerUsageEntry{Requests: p.Requests, PromptTokens: p.PromptTokens, CompletionTokens: p.CompletionTokens, CostUSD: p.Cost, CostRequests: p.CostRequests}
		}
	}
	return record
}

func latency(s domain.LatencyStats) latencyEntry {
	return latencyEntry{Count: s.Count, SumMS: s.Sum.Milliseconds(), MaxMS: s.Max.Milliseconds()}
}

func (e latencyEntry) stats() domain.LatencyStats {
	return domain.LatencyStats{Count: e.Count, Sum: time.Duration(e.SumMS) * time.Millisecond, Max: time.Duration(e.MaxMS) * time.Millisecond}
}
