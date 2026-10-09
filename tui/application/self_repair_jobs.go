package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The kinds of job the person starts from /jobs.
const (
	JobRepair      = "repair"
	JobImprovement = "improvement"
)

// maxJobBrief bounds the brief the person types, so the line the console
// adds still fits the brief the clone's marker and the first prompt carry.
const maxJobBrief = maxRepairBrief - 1<<10

// StartJob starts a repair or an improvement from a brief the person typed
// on /jobs, as one more background session of this console. The evidence an
// agent's request must cite does not apply: the person is the evidence. The
// slots of jobs.max_active, the duplicate and interruption rules and, for a
// repair, repair.max_attempts still do; the cap of three agent improvements
// per build does not, since those are the person's to start. The job's
// session uses the parent session's model.
func (r *SelfRepair) StartJob(ctx context.Context, parent domain.SessionState, kind, brief string) (domain.RepairRecord, error) {
	if err := r.configured(); err != nil {
		return domain.RepairRecord{}, err
	}
	improvement := kind == JobImprovement
	definition := "axlr_repair"
	if improvement {
		definition = "axlr_improve"
	}
	brief = strings.TrimSpace(brief)
	switch {
	case kind != JobRepair && kind != JobImprovement:
		return domain.RepairRecord{}, fmt.Errorf("a job is a repair or an improvement, not %q", kind)
	case len([]rune(brief)) < minRepairDescription:
		return domain.RepairRecord{}, fmt.Errorf("the brief needs at least %d characters: say what the %s must achieve", minRepairDescription, kind)
	case len(brief) > maxJobBrief:
		return domain.RepairRecord{}, fmt.Errorf("the brief takes at most %d bytes", maxJobBrief)
	case parent.Model == "":
		return domain.RepairRecord{}, errors.New("this session has no model; choose one with /model, since the job's session uses it")
	case parent.Mode == domain.ModeRepair || parent.Mode == domain.ModeImprove:
		return domain.RepairRecord{}, errors.New("a repair or improvement session cannot start a job; start it from a session outside the clones")
	}
	if dir := r.Settings.Directory; dir != "" && strings.HasPrefix(string(parent.Workspace)+string(filepath.Separator), filepath.Clean(dir)+string(filepath.Separator)) {
		return domain.RepairRecord{}, errors.New("this workspace is a repair or improvement clone; start jobs from a session outside the clones")
	}
	signature := jobSignature(r.Settings.Repository, kind, brief)
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return domain.RepairRecord{}, fmt.Errorf("read the repair registry: %w", err)
	}
	if refusal := r.admitJob(records, parent.ID, signature, improvement); refusal != "" {
		return domain.RepairRecord{}, errors.New(refusal)
	}
	if err := r.Engine.Ready(ctx, definition, "1.0"); err != nil {
		return domain.RepairRecord{}, fmt.Errorf("MADE cannot run the %s ceremony: %w", definition, err)
	}
	attempts := 0
	for _, record := range records {
		if record.Signature == signature && record.Session != "" {
			attempts++
		}
	}
	record := domain.RepairRecord{Improvement: improvement, Signature: signature, Brief: jobBrief(parent, improvement, brief, r.Build), Attempt: attempts + 1}
	if err := r.start(ctx, parent, records, &record, brief); err != nil {
		return record, err
	}
	return record, nil
}

// admitJob applies to the person's job the rules that still make sense
// without an agent: a job's own session does not start jobs, the slots are
// shared, an active or interrupted job with the same brief is not started
// twice, and a repair brief is started at most repair.max_attempts times.
func (r *SelfRepair) admitJob(records []domain.RepairRecord, session domain.SessionID, signature string, improvement bool) string {
	active, attempts := 0, 0
	for _, record := range records {
		if record.Session == session {
			return fmt.Sprintf("this session is the %s session of %s; start jobs from a session outside the clones", record.Kind(), record.ID)
		}
		if record.Active() {
			active++
		}
		if record.Signature != signature {
			continue
		}
		switch {
		case record.Active():
			return fmt.Sprintf("duplicate: %s %s is already %s for this brief", record.Kind(), record.ID, record.Status)
		case record.Status == domain.RepairInterrupted:
			return fmt.Sprintf("%s %s for this brief was interrupted; recover it with r instead of starting another", record.Kind(), record.ID)
		}
		if record.Session != "" {
			attempts++
		}
	}
	if limit := r.maxActive(); active >= limit {
		return fmt.Sprintf("%d of %d jobs are active (jobs.max_active); wait for one to end or raise the limit", active, limit)
	}
	if !improvement && attempts >= r.maxAttempts() {
		return fmt.Sprintf("attempts exhausted: %d repair sessions already ran for this brief (repair.max_attempts); change the brief or repair it by hand with axlr-tui --repair", attempts)
	}
	return ""
}

// jobSignature identifies the person's job by the repository, its kind and
// the brief's first words, as an agent's improvement is identified.
func jobSignature(repository, kind, brief string) string {
	sum := sha256.Sum256([]byte(repository + "\njob\n" + kind + "\n" + slugWords(brief)))
	return hex.EncodeToString(sum[:8])
}

// jobBrief is the brief the job's session starts with: the person's text,
// where it came from, and the closing line of its kind.
func jobBrief(parent domain.SessionState, improvement bool, brief, build string) string {
	closing := repairClosing
	if improvement {
		closing = improvementClosing
	}
	return bounded(fmt.Sprintf("%s\n\nStarted by the person from /jobs in session %s (AXLR build %s, workspace %s).\n\n%s", brief, parent.ID, build, parent.Workspace, closing), maxRepairBrief)
}

// Live reports whether this console drives the record's session now.
func (r *SelfRepair) Live(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.runs[id]
	return ok
}

// ActiveLimit is jobs.max_active as the coordinator applies it.
func (r *SelfRepair) ActiveLimit() int {
	if r == nil {
		return DefaultMaxActiveJobs
	}
	return r.maxActive()
}

// PullRequestStatus reads the job's pull request from the forge, when the
// person asks on /jobs; renders never do.
func (r *SelfRepair) PullRequestStatus(ctx context.Context, id string) (PullRequestStatus, error) {
	if r == nil || r.Registry == nil || r.Forge == nil {
		return PullRequestStatus{}, errors.New("the forge is not configured in this console")
	}
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return PullRequestStatus{}, err
	}
	for _, record := range records {
		if record.ID != id {
			continue
		}
		if record.PullRequest == 0 {
			return PullRequestStatus{}, fmt.Errorf("%s %s has no pull request yet", record.Kind(), id)
		}
		return r.Forge.Status(ctx, record.Repository, record.PullRequest)
	}
	return PullRequestStatus{}, fmt.Errorf("job %s is unknown", id)
}

// checkSummary is the record's line about the last check the ceremony
// reported: the verdict on the pull request's checks, or the check
// command's exit and its last output line.
func checkSummary(report map[string]any) string {
	if watch, ok := report["watch"].(map[string]any); ok {
		switch verdict, _ := watch["verdict"].(string); verdict {
		case "green":
			return bounded(fmt.Sprintf("pull request checks green: %v passed", watch["passed"]), 300)
		case "red":
			return bounded("pull request checks red: "+strings.Join(toStrings(watch["failed"]), "; "), 300)
		case "blocked":
			reason, _ := watch["reason"].(string)
			return bounded("pull request checks blocked: "+reason, 300)
		}
	}
	check, ok := report["check"].(map[string]any)
	if !ok {
		return ""
	}
	program, _ := check["program"].(string)
	command := strings.TrimSpace(program + " " + strings.Join(toStrings(check["args"]), " "))
	tail, _ := check["output_tail"].(string)
	lines := strings.Split(strings.TrimSpace(tail), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	summary := fmt.Sprintf("%s exited %v", command, check["exit_code"])
	if last != "" {
		summary += ": " + last
	}
	return bounded(singleLineText(summary), 300)
}
