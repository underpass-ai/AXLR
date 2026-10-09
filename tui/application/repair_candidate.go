package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// ErrNoCandidate reports a clone the console cannot build a console from,
// such as a repository that is not AXLR; the repair goes on without one.
var ErrNoCandidate = errors.New("the clone has no axlr-tui to build")

// RepairCandidate is the axlr-tui built from a repair clone: where it is,
// the version it reports and the commit it was built from.
type RepairCandidate struct {
	Path, Version, Revision string
}

// RepairInstallation is a candidate installed over the running console: the
// executable it replaced and where the replaced one was kept.
type RepairInstallation struct {
	Path, Backup string
}

// RepairCandidatePort builds the repaired console from a clone and installs
// it over the running one. Install never touches a running process's file
// in place: it writes beside it and renames, keeping the previous one.
type RepairCandidatePort interface {
	Build(ctx context.Context, clone, slug string) (RepairCandidate, error)
	Install(ctx context.Context, candidate RepairCandidate, slug string) (RepairInstallation, error)
}

// buildCandidate builds the repaired console once the checks are green, so
// the person can try it before approving the merge. It returns the reason
// to decline the merge when the repaired console does not build, as a red
// check would; a clone without a console, a console without a builder or a
// repair that already has its candidate return "".
func (r *SelfRepair) buildCandidate(ctx context.Context, run *repairRun) string {
	if r.Candidates == nil {
		return ""
	}
	r.mu.Lock()
	record := run.record
	r.mu.Unlock()
	if record.Candidate != "" || record.Clone == "" {
		return ""
	}
	candidate, err := r.Candidates.Build(ctx, record.Clone, record.ID)
	switch {
	case errors.Is(err, ErrNoCandidate) || ctx.Err() != nil:
		return ""
	case err != nil:
		reason := "the repaired axlr-tui does not build: " + bounded(err.Error(), 1500)
		r.update(ctx, run, func(record *domain.RepairRecord) { record.Error = reason })
		return reason
	}
	r.update(ctx, run, func(record *domain.RepairRecord) {
		record.Candidate, record.CandidateVersion, record.CandidateRevision = candidate.Path, candidate.Version, candidate.Revision
	})
	return ""
}

// tryCommand is how the person runs the candidate on the workspace of the
// session that asked for the repair, without installing it.
func (r *SelfRepair) tryCommand(ctx context.Context, record domain.RepairRecord) string {
	if record.Candidate == "" {
		return ""
	}
	command := shellQuote(record.Candidate)
	if r.Store != nil {
		if parent, err := r.Store.Load(ctx, record.Parent); err == nil {
			command += " --root " + shellQuote(string(parent.Export().Workspace))
		}
	}
	return command
}

// shellQuote quotes a path for a POSIX shell when it needs it.
func shellQuote(text string) string {
	if text != "" && strings.IndexFunc(text, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+:=@", r))
	}) < 0 {
		return text
	}
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// mergePending is the merge card's text, with the candidate to try first.
func (r *SelfRepair) mergePending(ctx context.Context, run *repairRun, pullRequest int, url string) string {
	pending := fmt.Sprintf("merge pull request #%d (%s): a approves and merges, d declines with a reason", pullRequest, url)
	r.mu.Lock()
	record := run.record
	r.mu.Unlock()
	if command := r.tryCommand(ctx, record); command != "" {
		pending += fmt.Sprintf("; try %s first: %s", record.CandidateVersion, command)
	}
	return pending
}

// Install replaces the running console's executable with a merged repair's
// candidate, keeping the previous one, and tells the session that asked for
// the repair to restart. Only a merged repair is installed: an unmerged
// candidate is tried by running it, not installed.
func (r *SelfRepair) Install(ctx context.Context, id string) error {
	if r == nil || r.Registry == nil || r.Candidates == nil {
		return errors.New("this console does not build repaired consoles")
	}
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.ID != id {
			continue
		}
		if reason := installRefusal(record); reason != "" {
			return errors.New(reason)
		}
		_, err := r.install(ctx, record)
		return err
	}
	return fmt.Errorf("repair %s is not in the registry", id)
}

// installRefusal says why a record's candidate cannot be installed.
func installRefusal(record domain.RepairRecord) string {
	switch {
	case record.Candidate == "":
		return fmt.Sprintf("repair %s has no built axlr-tui to install", record.ID)
	case record.Status != domain.RepairCompleted:
		return fmt.Sprintf("repair %s is %s: only a merged repair is installed; try its candidate with %s", record.ID, record.Status, record.Candidate)
	case record.Installed != "":
		return fmt.Sprintf("repair %s is already installed at %s (previous build kept as %s)", record.ID, record.Installed, record.Backup)
	}
	return ""
}

// Installable reports whether the panel's i installs the record.
func Installable(record domain.RepairRecord) bool {
	return installRefusal(record) == ""
}

func (r *SelfRepair) install(ctx context.Context, record domain.RepairRecord) (domain.RepairRecord, error) {
	installation, err := r.Candidates.Install(ctx, RepairCandidate{Path: record.Candidate, Version: record.CandidateVersion, Revision: record.CandidateRevision}, record.ID)
	if err != nil {
		return record, fmt.Errorf("install %s: %w", record.CandidateVersion, err)
	}
	record.Installed, record.Backup, record.Error = installation.Path, installation.Backup, ""
	// The session that asked hears about it again, now with both paths.
	record.Notice, record.Notified = repairNotice(record, r.Build), false
	return record, r.save(ctx, &record)
}

// installMerged installs a merged repair's candidate when settings ask for
// it; a failure stays in the record and the notice.
func (r *SelfRepair) installMerged(ctx context.Context, run *repairRun) {
	r.mu.Lock()
	record := run.record
	r.mu.Unlock()
	if !r.AutoInstall || !Installable(record) {
		return
	}
	installed, err := r.install(ctx, record)
	if err != nil {
		r.update(ctx, run, func(record *domain.RepairRecord) {
			record.Error = bounded(err.Error(), 600)
			record.Notice = repairNotice(*record, r.Build)
		})
		return
	}
	r.mu.Lock()
	run.record = installed
	r.mu.Unlock()
}
