package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const repairRegistryVersion = 1
const maxRepairRegistryBytes = 1 << 20

type repairRegistryFile struct {
	Version int           `json:"version"`
	Repairs []repairEntry `json:"repairs"`
}

type repairEntry struct {
	ID          string    `json:"id"`
	Improvement bool      `json:"improvement,omitempty"`
	Signature   string    `json:"signature"`
	Repository  string    `json:"repository"`
	Parent      string    `json:"parent"`
	Session     string    `json:"session,omitempty"`
	Clone       string    `json:"clone,omitempty"`
	Brief       string    `json:"brief,omitempty"`
	Build       string    `json:"build,omitempty"`
	Status      string    `json:"status"`
	Step        string    `json:"step,omitempty"`
	State       string    `json:"state,omitempty"`
	Instance    string    `json:"instance,omitempty"`
	StepAttempt int       `json:"step_attempt,omitempty"`
	StepLimit   int       `json:"step_limit,omitempty"`
	Check       string    `json:"check,omitempty"`
	PullRequest int       `json:"pull_request,omitempty"`
	URL         string    `json:"url,omitempty"`
	MergeSHA    string    `json:"merge_sha,omitempty"`
	Pending     string    `json:"pending,omitempty"`
	Memory      string    `json:"memory,omitempty"`
	Error       string    `json:"error,omitempty"`
	Attempt     int       `json:"attempt,omitempty"`
	RunToken    string    `json:"run_token,omitempty"`
	Notice      string    `json:"notice,omitempty"`
	Notified    bool      `json:"notified,omitempty"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}

// RepairRegistry keeps the self-repair records in one private file beside
// the sessions. It is lenient on unknown fields, so an older console still
// lists repairs a newer one recorded.
type RepairRegistry struct {
	path string
	mu   sync.Mutex
}

var _ application.RepairRegistryPort = (*RepairRegistry)(nil)

func NewRepairRegistry(path string) (*RepairRegistry, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("repair registry path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return &RepairRegistry{path: path}, nil
}

func (r *RepairRegistry) Load(ctx context.Context) ([]domain.RepairRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := r.read()
	if err != nil {
		return nil, err
	}
	records := make([]domain.RepairRecord, 0, len(file.Repairs))
	for _, entry := range file.Repairs {
		record := toRecord(entry)
		if record.Validate() != nil {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *RepairRegistry) Save(ctx context.Context, record domain.RepairRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	release, err := acquireMCPConfigLock(ctx, r.path)
	if err != nil {
		return err
	}
	defer release()
	file, err := r.read()
	if err != nil {
		return err
	}
	entry := fromRecord(record)
	replaced := false
	for i := range file.Repairs {
		if file.Repairs[i].ID == record.ID {
			file.Repairs[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		file.Repairs = append(file.Repairs, entry)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxRepairRegistryBytes {
		return errors.New("repair registry is full")
	}
	return r.write(ctx, data)
}

func (r *RepairRegistry) read() (repairRegistryFile, error) {
	file := repairRegistryFile{Version: repairRegistryVersion}
	handle, err := openNoFollow(r.path, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return file, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxRepairRegistryBytes {
		return file, errors.New("invalid repair registry file")
	}
	data, err := io.ReadAll(io.LimitReader(handle, maxRepairRegistryBytes+1))
	if err != nil {
		return file, err
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&file); err != nil || file.Version != repairRegistryVersion {
		return repairRegistryFile{}, errors.New("invalid repair registry file")
	}
	return file, nil
}

func (r *RepairRegistry) write(ctx context.Context, data []byte) error {
	dir := filepath.Dir(r.path)
	file, err := os.CreateTemp(dir, ".repairs-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), r.path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return syncDirectoryFile(directory)
}

func fromRecord(record domain.RepairRecord) repairEntry {
	return repairEntry{ID: record.ID, Improvement: record.Improvement, Signature: record.Signature, Repository: record.Repository, Parent: string(record.Parent), Session: string(record.Session), Clone: record.Clone, Brief: record.Brief, Build: record.Build,
		Status: string(record.Status), Step: record.Step, State: record.State, Instance: record.Instance, StepAttempt: record.StepAttempt, StepLimit: record.StepLimit, Check: record.Check, PullRequest: record.PullRequest, URL: record.URL, MergeSHA: record.MergeSHA, Pending: record.Pending, Memory: record.Memory, Error: record.Error,
		Attempt: record.Attempt, RunToken: record.RunToken, Notice: record.Notice, Notified: record.Notified, Created: record.Created.UTC(), Updated: record.Updated.UTC()}
}

func toRecord(entry repairEntry) domain.RepairRecord {
	return domain.RepairRecord{ID: entry.ID, Improvement: entry.Improvement, Signature: entry.Signature, Repository: entry.Repository, Parent: domain.SessionID(entry.Parent), Session: domain.SessionID(entry.Session), Clone: entry.Clone, Brief: entry.Brief, Build: entry.Build,
		Status: domain.RepairStatus(entry.Status), Step: entry.Step, State: entry.State, Instance: entry.Instance, StepAttempt: entry.StepAttempt, StepLimit: entry.StepLimit, Check: entry.Check, PullRequest: entry.PullRequest, URL: entry.URL, MergeSHA: entry.MergeSHA, Pending: entry.Pending, Memory: entry.Memory, Error: entry.Error,
		Attempt: entry.Attempt, RunToken: entry.RunToken, Notice: entry.Notice, Notified: entry.Notified, Created: entry.Created, Updated: entry.Updated}
}
