package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	sessionReceiptVersion  = 1
	maxSessionReceiptBytes = 4 << 20
)

var _ application.SessionReceiptPort = (*SessionStore)(nil)

// errReceiptFull refuses an entry past the journal's bound; the receipt
// then says it is not complete.
var errReceiptFull = errors.New("the session's receipt journal reached 4 MiB")

// sessionReceiptLine is one line of <id>.receipt.jsonl beside the snapshot:
// appended and synced as a request or a call starts and ends, never
// rewritten, so a crash loses at most the line it was writing. Durations
// are milliseconds and costs dollars.
type sessionReceiptLine struct {
	Version          int      `json:"v"`
	Kind             string   `json:"kind"`
	At               string   `json:"at"`
	Model            string   `json:"model,omitempty"`
	Status           string   `json:"status,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	PromptTokens     int      `json:"prompt_tokens,omitempty"`
	CachedTokens     int      `json:"cached_tokens,omitempty"`
	CacheWriteTokens int      `json:"cache_write_tokens,omitempty"`
	CompletionTokens int      `json:"completion_tokens,omitempty"`
	ReasoningTokens  int      `json:"reasoning_tokens,omitempty"`
	CostUSD          *float64 `json:"cost_usd,omitempty"`
	FirstByteMS      int64    `json:"first_byte_ms,omitempty"`
	TotalMS          int64    `json:"total_ms,omitempty"`
	Unmetered        bool     `json:"unmetered,omitempty"`
	Call             string   `json:"call_id,omitempty"`
}

func (s *SessionStore) receiptPath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".receipt.jsonl")
}

// AppendReceipt adds one entry to the session's journal and syncs it.
func (s *SessionStore) AppendReceipt(ctx context.Context, id domain.SessionID, entry domain.ReceiptEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return err
	}
	if _, err := domain.NewSessionID(string(id)); err != nil {
		return err
	}
	if err := entry.Validate(); err != nil {
		return err
	}
	line, err := json.Marshal(receiptLine(entry))
	if err != nil {
		return err
	}
	file, err := openNoFollow(s.receiptPath(id), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if size := info.Size(); size > 0 {
		// A line a crash cut ends without its newline: the next one starts
		// on a line of its own, so only the cut one is lost.
		last := make([]byte, 1)
		if _, err := file.ReadAt(last, size-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			line = append([]byte{'\n'}, line...)
		}
	}
	if info.Size()+int64(len(line)) > maxSessionReceiptBytes {
		return errReceiptFull
	}
	// The Windows open ignores O_APPEND: the line goes at the end either
	// way, and the store's lock keeps appends apart.
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err := file.Write(line); err != nil {
		return err
	}
	return file.Sync()
}

// LoadReceipt returns the session's journal in order; none when it has
// none. A line that does not read, such as the one a crash cut, is
// skipped: the receipt then lists what the others say.
func (s *SessionStore) LoadReceipt(ctx context.Context, id domain.SessionID) ([]domain.ReceiptEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	if _, err := domain.NewSessionID(string(id)); err != nil {
		return nil, err
	}
	file, err := openNoFollow(s.receiptPath(id), os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionReceiptBytes))
	if err != nil {
		return nil, err
	}
	var entries []domain.ReceiptEntry
	for _, raw := range bytes.Split(data, []byte("\n")) {
		var line sessionReceiptLine
		if len(raw) == 0 || json.Unmarshal(raw, &line) != nil || line.Version != sessionReceiptVersion {
			continue
		}
		if entry, err := line.entry(); err == nil {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func receiptLine(entry domain.ReceiptEntry) sessionReceiptLine {
	line := sessionReceiptLine{Version: sessionReceiptVersion, Kind: string(entry.Kind), At: entry.At.UTC().Format(time.RFC3339Nano), Model: string(entry.Model), Call: string(entry.Call)}
	if entry.Kind == domain.ReceiptRequestEnded {
		usage := entry.Usage
		line.Status, line.Provider = string(entry.Status), usage.Provider
		line.PromptTokens, line.CachedTokens, line.CacheWriteTokens = usage.PromptTokens, usage.CachedTokens, usage.CacheWriteTokens
		line.CompletionTokens, line.ReasoningTokens = usage.CompletionTokens, usage.ReasoningTokens
		line.FirstByteMS, line.TotalMS, line.Unmetered = usage.FirstByte.Milliseconds(), usage.Total.Milliseconds(), usage.Unmetered
		if usage.CostKnown {
			cost := usage.Cost
			line.CostUSD = &cost
		}
	}
	return line
}

func (l sessionReceiptLine) entry() (domain.ReceiptEntry, error) {
	at, err := time.Parse(time.RFC3339Nano, l.At)
	if err != nil {
		return domain.ReceiptEntry{}, err
	}
	entry := domain.ReceiptEntry{Kind: domain.ReceiptEntryKind(l.Kind), At: at, Model: root.ModelID(l.Model), Status: domain.ReceiptRequestStatus(l.Status), Call: root.ToolCallID(l.Call)}
	entry.Usage = domain.UsageSample{Provider: l.Provider, PromptTokens: l.PromptTokens, CachedTokens: l.CachedTokens, CacheWriteTokens: l.CacheWriteTokens, CompletionTokens: l.CompletionTokens, ReasoningTokens: l.ReasoningTokens, FirstByte: time.Duration(l.FirstByteMS) * time.Millisecond, Total: time.Duration(l.TotalMS) * time.Millisecond, Unmetered: l.Unmetered}
	if l.CostUSD != nil {
		entry.Usage.Cost, entry.Usage.CostKnown = *l.CostUSD, true
	}
	return entry, entry.Validate()
}
