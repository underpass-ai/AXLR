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

const planRegistryVersion = 1

// A plan carries up to twelve context packs of 12 KiB each.
const maxPlanRegistryBytes = 8 << 20

type planRegistryFile struct {
	Version int         `json:"version"`
	Plans   []planEntry `json:"plans"`
}

type planCommand struct {
	Program string   `json:"program"`
	Args    []string `json:"args,omitempty"`
}

type planCitation struct {
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Quote string `json:"quote"`
}

type planNote struct {
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text"`
}

type planHandback struct {
	Summary   string     `json:"summary,omitempty"`
	SummaryEN string     `json:"summary_en,omitempty"`
	Notes     []planNote `json:"notes,omitempty"`
	Questions []string   `json:"questions,omitempty"`
	Changed   []string   `json:"changed,omitempty"`
	Revision  string     `json:"revision,omitempty"`
}

type planTaskEntry struct {
	ID        string         `json:"id"`
	Goal      string         `json:"goal"`
	Scope     []string       `json:"scope"`
	New       []string       `json:"new,omitempty"`
	Context   []planCitation `json:"context,omitempty"`
	UnitCheck planCommand    `json:"unit_check"`
	Baseline  int            `json:"baseline"`
	DependsOn []string       `json:"depends_on,omitempty"`
	TestFirst bool           `json:"test_first,omitempty"`
	Protect   []string       `json:"protect,omitempty"`
	Wave      int            `json:"wave"`
	Pack      string         `json:"pack,omitempty"`
	Status    string         `json:"status"`
	Session   string         `json:"session,omitempty"`
	Instance  string         `json:"instance,omitempty"`
	Step      string         `json:"step,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Handback  *planHandback  `json:"handback,omitempty"`
}

type planSyncEntry struct {
	Wave     int    `json:"wave"`
	Instance string `json:"instance,omitempty"`
	Rounds   int    `json:"rounds,omitempty"`
	Verdict  string `json:"verdict,omitempty"`
	Output   string `json:"output,omitempty"`
}

type planEntry struct {
	ID          string          `json:"id"`
	Session     string          `json:"session"`
	Brief       string          `json:"brief,omitempty"`
	Workspace   string          `json:"workspace,omitempty"`
	Planner     string          `json:"planner,omitempty"`
	Worker      string          `json:"worker,omitempty"`
	Instance    string          `json:"instance,omitempty"`
	Status      string          `json:"status"`
	Tasks       []planTaskEntry `json:"tasks,omitempty"`
	E2E         planCommand     `json:"e2e_check"`
	E2EBaseline int             `json:"e2e_baseline"`
	Interfaces  string          `json:"interfaces,omitempty"`
	SummaryEN   string          `json:"summary_en,omitempty"`
	Waves       int             `json:"waves,omitempty"`
	Defects     []string        `json:"defects,omitempty"`
	Decision    string          `json:"decision,omitempty"`
	Reason      string          `json:"reason,omitempty"`
	Syncs       []planSyncEntry `json:"syncs,omitempty"`
	Error       string          `json:"error,omitempty"`
	RunToken    string          `json:"run_token,omitempty"`
	Created     time.Time       `json:"created"`
	Updated     time.Time       `json:"updated"`
}

// PlanRegistry keeps the plans in one private file beside the sessions, as
// RepairRegistry does for repairs. Unknown fields are ignored, so an older
// console still lists plans a newer one recorded.
type PlanRegistry struct {
	path string
	mu   sync.Mutex
}

var _ application.PlanRegistryPort = (*PlanRegistry)(nil)

func NewPlanRegistry(path string) (*PlanRegistry, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("plan registry path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return &PlanRegistry{path: path}, nil
}

func (r *PlanRegistry) Load(ctx context.Context) ([]domain.PlanRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := r.read()
	if err != nil {
		return nil, err
	}
	records := make([]domain.PlanRecord, 0, len(file.Plans))
	for _, entry := range file.Plans {
		if record := toPlanRecord(entry); record.Validate() == nil {
			records = append(records, record)
		}
	}
	return records, nil
}

func (r *PlanRegistry) Save(ctx context.Context, record domain.PlanRecord) error {
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
	entry := fromPlanRecord(record)
	replaced := false
	for i := range file.Plans {
		if file.Plans[i].ID == record.ID {
			file.Plans[i], replaced = entry, true
			break
		}
	}
	if !replaced {
		file.Plans = append(file.Plans, entry)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxPlanRegistryBytes {
		return errors.New("plan registry is full")
	}
	return writePrivateFile(ctx, r.path, ".plans-*", data)
}

func (r *PlanRegistry) read() (planRegistryFile, error) {
	file := planRegistryFile{Version: planRegistryVersion}
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
	if !info.Mode().IsRegular() || info.Size() > maxPlanRegistryBytes {
		return file, errors.New("invalid plan registry file")
	}
	data, err := io.ReadAll(io.LimitReader(handle, maxPlanRegistryBytes+1))
	if err != nil {
		return file, err
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&file); err != nil || file.Version != planRegistryVersion {
		return planRegistryFile{}, errors.New("invalid plan registry file")
	}
	return file, nil
}

// writePrivateFile replaces path atomically with an owner-only file.
func writePrivateFile(ctx context.Context, path, pattern string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, pattern)
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
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return syncDirectoryFile(directory)
}

func toCommand(c planCommand) domain.CheckCommand {
	return domain.CheckCommand{Program: c.Program, Args: append([]string(nil), c.Args...)}
}

func fromCommand(c domain.CheckCommand) planCommand {
	return planCommand{Program: c.Program, Args: append([]string(nil), c.Args...)}
}

func fromPlanRecord(record domain.PlanRecord) planEntry {
	entry := planEntry{ID: record.ID, Session: string(record.Session), Brief: record.Brief, Workspace: string(record.Workspace), Planner: record.Planner, Worker: record.Worker, Instance: record.Instance,
		Status: string(record.Status), E2E: fromCommand(record.E2E), E2EBaseline: record.E2EBaseline, Interfaces: record.Interfaces, SummaryEN: record.SummaryEN, Waves: record.Waves,
		Defects: record.Defects, Decision: record.Decision, Reason: record.Reason, Error: record.Error, RunToken: record.RunToken, Created: record.Created.UTC(), Updated: record.Updated.UTC()}
	for _, task := range record.Tasks {
		t := planTaskEntry{ID: task.ID, Goal: task.Goal, Scope: task.Scope, New: task.New, UnitCheck: fromCommand(task.UnitCheck), Baseline: task.Baseline, DependsOn: task.DependsOn,
			TestFirst: task.TestFirst, Protect: task.Protect, Wave: task.Wave, Pack: task.Pack, Status: string(task.Status), Session: string(task.Session), Instance: task.Instance, Step: task.Step, Reason: task.Reason}
		for _, c := range task.Context {
			t.Context = append(t.Context, planCitation{Path: c.Path, Line: c.Line, Quote: c.Quote})
		}
		if h := task.Handback; h != nil {
			t.Handback = &planHandback{Summary: h.Summary, SummaryEN: h.SummaryEN, Questions: h.Questions, Changed: h.Changed, Revision: h.Revision}
			for _, n := range h.Notes {
				t.Handback.Notes = append(t.Handback.Notes, planNote{From: n.From, To: n.To, Text: n.Text})
			}
		}
		entry.Tasks = append(entry.Tasks, t)
	}
	for _, s := range record.Syncs {
		entry.Syncs = append(entry.Syncs, planSyncEntry{Wave: s.Wave, Instance: s.Instance, Rounds: s.Rounds, Verdict: s.Verdict, Output: s.Output})
	}
	return entry
}

func toPlanRecord(entry planEntry) domain.PlanRecord {
	record := domain.PlanRecord{ID: entry.ID, Session: domain.SessionID(entry.Session), Brief: entry.Brief, Workspace: domain.Workspace(entry.Workspace), Planner: entry.Planner, Worker: entry.Worker, Instance: entry.Instance,
		Status: domain.PlanStatus(entry.Status), E2E: toCommand(entry.E2E), E2EBaseline: entry.E2EBaseline, Interfaces: entry.Interfaces, SummaryEN: entry.SummaryEN, Waves: entry.Waves,
		Defects: entry.Defects, Decision: entry.Decision, Reason: entry.Reason, Error: entry.Error, RunToken: entry.RunToken, Created: entry.Created, Updated: entry.Updated}
	for _, t := range entry.Tasks {
		task := domain.PlanTask{ID: t.ID, Goal: t.Goal, Scope: t.Scope, New: t.New, UnitCheck: toCommand(t.UnitCheck), Baseline: t.Baseline, DependsOn: t.DependsOn, TestFirst: t.TestFirst,
			Protect: t.Protect, Wave: t.Wave, Pack: t.Pack, Status: domain.TaskStatus(t.Status), Session: domain.SessionID(t.Session), Instance: t.Instance, Step: t.Step, Reason: t.Reason}
		for _, c := range t.Context {
			task.Context = append(task.Context, domain.Citation{Path: c.Path, Line: c.Line, Quote: c.Quote})
		}
		if h := t.Handback; h != nil {
			task.Handback = &domain.TaskHandback{Summary: h.Summary, SummaryEN: h.SummaryEN, Questions: h.Questions, Changed: h.Changed, Revision: h.Revision}
			for _, n := range h.Notes {
				task.Handback.Notes = append(task.Handback.Notes, domain.TaskNote{From: n.From, To: n.To, Text: n.Text})
			}
		}
		record.Tasks = append(record.Tasks, task)
	}
	for _, s := range entry.Syncs {
		record.Syncs = append(record.Syncs, domain.SyncRecord{Wave: s.Wave, Instance: s.Instance, Rounds: s.Rounds, Verdict: s.Verdict, Output: s.Output})
	}
	return record
}
