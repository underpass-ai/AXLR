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
const maxSessionCeremonyBytes = 16 << 10

// sessionCeremony is the live ceremony kept in <id>.ceremony, beside the
// snapshot for the same reason as <id>.mode.
type sessionCeremony struct {
	Version    int              `json:"version"`
	Definition string           `json:"definition"`
	Release    string           `json:"release"`
	Instance   string           `json:"instance"`
	Step       string           `json:"step"`
	Iteration  int              `json:"iteration"`
	Fence      string           `json:"fence"`
	Program    string           `json:"check_program,omitempty"`
	Args       []string         `json:"check_args,omitempty"`
	About      string           `json:"about,omitempty"`
	Memory     string           `json:"memory,omitempty"`
	BudgetBase int              `json:"budget_base,omitempty"`
	Reminded   bool             `json:"reminded,omitempty"`
	Incident   *sessionIncident `json:"incident,omitempty"`
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
	run := domain.CeremonyRun{Definition: record.Definition, Version: record.Release, Instance: record.Instance, Step: record.Step, Iteration: record.Iteration, Fence: record.Fence, Check: domain.CheckCommand{Program: record.Program, Args: record.Args}, About: record.About, Memory: record.Memory, BudgetBase: record.BudgetBase, Reminded: record.Reminded}
	if i := record.Incident; i != nil {
		run.Incident = &domain.IncidentRun{Slug: i.Slug, Service: i.Service, Severity: i.Severity, DraftPath: i.DraftPath, DraftDigest: i.DraftDigest, Findings: i.Findings, ReturnReason: i.ReturnReason, Returns: i.Returns, ReviewFailures: i.ReviewFailures, Awaiting: i.Awaiting, Decided: i.Decided, Granted: i.Granted, Published: i.Published}
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
	record := sessionCeremony{Version: sessionCeremonyVersion, Definition: run.Definition, Release: run.Version, Instance: run.Instance, Step: run.Step, Iteration: run.Iteration, Fence: run.Fence, Program: run.Check.Program, Args: run.Check.Args, About: run.About, Memory: run.Memory, BudgetBase: run.BudgetBase, Reminded: run.Reminded}
	if i := run.Incident; i != nil {
		record.Incident = &sessionIncident{Slug: i.Slug, Service: i.Service, Severity: i.Severity, DraftPath: i.DraftPath, DraftDigest: i.DraftDigest, Findings: i.Findings, ReturnReason: i.ReturnReason, Returns: i.Returns, ReviewFailures: i.ReviewFailures, Awaiting: i.Awaiting, Decided: i.Decided, Granted: i.Granted, Published: i.Published}
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.writeSidecar(".ceremony-*", s.ceremonyPath(id), data)
}
