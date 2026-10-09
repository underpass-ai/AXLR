package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const sessionCeremonyVersion = 1

// maxSessionCeremonyBytes leaves room for the incident's findings and reason;
// a larger file is ignored, which would drop the pointer to the instance.
const maxSessionCeremonyBytes = 32 << 10

// sessionCeremony is the live ceremony kept in <id>.ceremony, beside the
// snapshot for the same reason as <id>.mode.
type sessionCeremony struct {
	Version    int            `json:"version"`
	Definition string         `json:"definition"`
	Release    string         `json:"release"`
	Instance   string         `json:"instance"`
	Step       string         `json:"step"`
	Iteration  int            `json:"iteration"`
	Fence      string         `json:"fence"`
	Program    string         `json:"check_program,omitempty"`
	Args       []string       `json:"check_args,omitempty"`
	Approved   []sessionCheck `json:"approved,omitempty"`
	About      string         `json:"about,omitempty"`
	Memory     string         `json:"memory,omitempty"`
	BudgetBase int            `json:"budget_base,omitempty"`
	Reminded   bool           `json:"reminded,omitempty"`
	// LimitStep and LimitResumes count the step's budget restarts at the
	// call limit; sidecars written before them have none.
	LimitStep    string           `json:"limit_step,omitempty"`
	LimitResumes int              `json:"limit_resumes,omitempty"`
	Compact      bool             `json:"compact,omitempty"`
	Ledger       []sessionLedger  `json:"ledger,omitempty"`
	StepCall     string           `json:"step_call,omitempty"`
	Model        string           `json:"model,omitempty"`
	Plan         *sessionPlan     `json:"plan,omitempty"`
	Task         *sessionTask     `json:"task,omitempty"`
	Incident     *sessionIncident `json:"incident,omitempty"`
	Repair       *sessionRepair   `json:"repair,omitempty"`
}

// sessionCheck is a command the person approved; sidecars written before
// approvals were kept have none.
type sessionCheck struct {
	Program string   `json:"program"`
	Args    []string `json:"args,omitempty"`
}

type sessionTask struct {
	Plan      string            `json:"plan"`
	Task      string            `json:"task"`
	Scope     []string          `json:"scope,omitempty"`
	Protect   []string          `json:"protect,omitempty"`
	Program   string            `json:"check_program,omitempty"`
	Args      []string          `json:"check_args,omitempty"`
	TestFirst bool              `json:"test_first,omitempty"`
	Start     map[string]string `json:"start,omitempty"`
	Frozen    map[string]string `json:"frozen,omitempty"`
	Git       bool              `json:"git,omitempty"`
}

type sessionPlan struct {
	ID           string   `json:"id"`
	Awaiting     string   `json:"awaiting,omitempty"`
	Decided      string   `json:"decided,omitempty"`
	Granted      bool     `json:"granted,omitempty"`
	Returns      int      `json:"returns,omitempty"`
	ReturnReason string   `json:"return_reason,omitempty"`
	Defects      []string `json:"defects,omitempty"`
}

type sessionLedger struct {
	Step         string `json:"step"`
	Iteration    int    `json:"iteration"`
	Text         string `json:"text"`
	FirstMessage int    `json:"first_message"`
	LastMessage  int    `json:"last_message"`
}

type sessionRepair struct {
	Improvement   bool     `json:"improvement,omitempty"`
	Repository    string   `json:"repository,omitempty"`
	Base          string   `json:"base,omitempty"`
	Branch        string   `json:"branch,omitempty"`
	Slug          string   `json:"slug,omitempty"`
	Title         string   `json:"title,omitempty"`
	CauseRef      string   `json:"cause_ref,omitempty"`
	PullRequest   int      `json:"pull_request,omitempty"`
	URL           string   `json:"url,omitempty"`
	HeadSHA       string   `json:"head_sha,omitempty"`
	Rounds        int      `json:"rounds,omitempty"`
	Feedback      string   `json:"feedback,omitempty"`
	Cause         string   `json:"cause,omitempty"`
	Fix           string   `json:"fix,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	Criteria      string   `json:"criteria,omitempty"`
	Scope         string   `json:"scope,omitempty"`
	WakeRefs      []string `json:"wake_refs,omitempty"`
	Awaiting      string   `json:"awaiting,omitempty"`
	Decided       string   `json:"decided,omitempty"`
	Granted       bool     `json:"granted,omitempty"`
	MergeSHA      string   `json:"merge_sha,omitempty"`
	CauseRecorded bool     `json:"cause_recorded,omitempty"`
}

type sessionIncident struct {
	Slug           string   `json:"slug,omitempty"`
	Service        string   `json:"service,omitempty"`
	Severity       string   `json:"severity,omitempty"`
	DraftPath      string   `json:"draft_path,omitempty"`
	DraftDigest    string   `json:"draft_digest,omitempty"`
	Findings       []string `json:"findings,omitempty"`
	ReturnReason   string   `json:"return_reason,omitempty"`
	Returns        int      `json:"returns,omitempty"`
	ReviewFailures int      `json:"review_failures,omitempty"`
	Awaiting       string   `json:"awaiting,omitempty"`
	Decided        string   `json:"decided,omitempty"`
	Granted        bool     `json:"granted,omitempty"`
	Published      string   `json:"published,omitempty"`
}

func (s *SessionStore) ceremonyPath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".ceremony")
}

// readCeremony returns nil when the file is missing or unreadable. The MADE
// instance itself is durable; a lost sidecar only loses the session's pointer.
func (s *SessionStore) readCeremony(id domain.SessionID) *domain.CeremonyRun {
	file, err := openNoFollow(s.ceremonyPath(id), os.O_RDONLY, 0)
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionCeremonyBytes+1))
	if err != nil || len(data) > maxSessionCeremonyBytes {
		return nil
	}
	// Lenient on purpose: this file is a pointer to a durable MADE instance,
	// so a console that does not know a newer field still finds the instance
	// instead of silently losing it. The version still guards the shape.
	decoder := json.NewDecoder(bytes.NewReader(data))
	var record sessionCeremony
	if decoder.Decode(&record) != nil || record.Version != sessionCeremonyVersion {
		return nil
	}
	run := domain.CeremonyRun{Definition: record.Definition, Version: record.Release, Instance: record.Instance, Step: record.Step, Iteration: record.Iteration, Fence: record.Fence, Check: domain.CheckCommand{Program: record.Program, Args: record.Args}, About: record.About, Memory: record.Memory, BudgetBase: record.BudgetBase, Reminded: record.Reminded, LimitStep: record.LimitStep, LimitResumes: record.LimitResumes, Compact: record.Compact, StepCall: record.StepCall, Model: record.Model}
	for _, command := range record.Approved {
		run.Approved = append(run.Approved, domain.CheckCommand{Program: command.Program, Args: command.Args})
	}
	if t := record.Task; t != nil {
		run.Task = &domain.TaskRun{Plan: t.Plan, Task: t.Task, Scope: t.Scope, Protect: t.Protect, Check: domain.CheckCommand{Program: t.Program, Args: t.Args}, TestFirst: t.TestFirst, Start: t.Start, Frozen: t.Frozen, Git: t.Git}
	}
	if p := record.Plan; p != nil {
		run.Plan = &domain.PlanRun{ID: p.ID, Awaiting: p.Awaiting, Decided: p.Decided, Granted: p.Granted, Returns: p.Returns, ReturnReason: p.ReturnReason, Defects: p.Defects}
	}
	for _, entry := range record.Ledger {
		run.Ledger = append(run.Ledger, domain.LedgerEntry{Step: entry.Step, Iteration: entry.Iteration, Text: entry.Text, FirstMessage: entry.FirstMessage, LastMessage: entry.LastMessage})
	}
	if i := record.Incident; i != nil {
		run.Incident = &domain.IncidentRun{Slug: i.Slug, Service: i.Service, Severity: i.Severity, DraftPath: i.DraftPath, DraftDigest: i.DraftDigest, Findings: i.Findings, ReturnReason: i.ReturnReason, Returns: i.Returns, ReviewFailures: i.ReviewFailures, Awaiting: i.Awaiting, Decided: i.Decided, Granted: i.Granted, Published: i.Published}
	}
	if r := record.Repair; r != nil {
		run.Repair = &domain.RepairRun{Improvement: r.Improvement, Repository: r.Repository, Base: r.Base, Branch: r.Branch, Slug: r.Slug, Title: r.Title, CauseRef: r.CauseRef, PullRequest: r.PullRequest, URL: r.URL, HeadSHA: r.HeadSHA, Rounds: r.Rounds, Feedback: r.Feedback, Cause: r.Cause, Fix: r.Fix, Summary: r.Summary, Criteria: r.Criteria, Scope: r.Scope, WakeRefs: r.WakeRefs, Awaiting: r.Awaiting, Decided: r.Decided, Granted: r.Granted, MergeSHA: r.MergeSHA, CauseRecorded: r.CauseRecorded}
	}
	if run.Validate() != nil {
		return nil
	}
	return &run
}

func (s *SessionStore) writeCeremony(id domain.SessionID, run *domain.CeremonyRun) error {
	if run == nil {
		if err := os.Remove(s.ceremonyPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	record := sessionCeremony{Version: sessionCeremonyVersion, Definition: run.Definition, Release: run.Version, Instance: run.Instance, Step: run.Step, Iteration: run.Iteration, Fence: run.Fence, Program: run.Check.Program, Args: run.Check.Args, About: run.About, Memory: run.Memory, BudgetBase: run.BudgetBase, Reminded: run.Reminded, LimitStep: run.LimitStep, LimitResumes: run.LimitResumes, Compact: run.Compact, StepCall: run.StepCall, Model: run.Model}
	for _, command := range run.Approved {
		record.Approved = append(record.Approved, sessionCheck{Program: command.Program, Args: command.Args})
	}
	if t := run.Task; t != nil {
		record.Task = &sessionTask{Plan: t.Plan, Task: t.Task, Scope: t.Scope, Protect: t.Protect, Program: t.Check.Program, Args: t.Check.Args, TestFirst: t.TestFirst, Start: t.Start, Frozen: t.Frozen, Git: t.Git}
	}
	if p := run.Plan; p != nil {
		record.Plan = &sessionPlan{ID: p.ID, Awaiting: p.Awaiting, Decided: p.Decided, Granted: p.Granted, Returns: p.Returns, ReturnReason: p.ReturnReason, Defects: p.Defects}
	}
	for _, entry := range run.Ledger {
		record.Ledger = append(record.Ledger, sessionLedger{Step: entry.Step, Iteration: entry.Iteration, Text: entry.Text, FirstMessage: entry.FirstMessage, LastMessage: entry.LastMessage})
	}
	if i := run.Incident; i != nil {
		record.Incident = &sessionIncident{Slug: i.Slug, Service: i.Service, Severity: i.Severity, DraftPath: i.DraftPath, DraftDigest: i.DraftDigest, Findings: i.Findings, ReturnReason: i.ReturnReason, Returns: i.Returns, ReviewFailures: i.ReviewFailures, Awaiting: i.Awaiting, Decided: i.Decided, Granted: i.Granted, Published: i.Published}
	}
	if r := run.Repair; r != nil {
		record.Repair = &sessionRepair{Improvement: r.Improvement, Repository: r.Repository, Base: r.Base, Branch: r.Branch, Slug: r.Slug, Title: r.Title, CauseRef: r.CauseRef, PullRequest: r.PullRequest, URL: r.URL, HeadSHA: r.HeadSHA, Rounds: r.Rounds, Feedback: r.Feedback, Cause: r.Cause, Fix: r.Fix, Summary: r.Summary, Criteria: r.Criteria, Scope: r.Scope, WakeRefs: r.WakeRefs, Awaiting: r.Awaiting, Decided: r.Decided, Granted: r.Granted, MergeSHA: r.MergeSHA, CauseRecorded: r.CauseRecorded}
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.writeSidecar(".ceremony-*", s.ceremonyPath(id), data)
}
