package application

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const jobBriefText = "local_search should list matches per file with their line numbers"

func TestStartJobRefusesWhatThePanelCannotStart(t *testing.T) {
	rig := newRepairRig(t, &mergeWorkbench{})
	parent := turnSession(t).Export()
	repairMode := parent
	repairMode.Mode = domain.ModeRepair
	noModel := parent
	noModel.Model = ""
	inClone := parent
	inClone.Workspace = domain.Workspace(filepath.Join(rig.repairsIn, "x"))
	for name, tc := range map[string]struct {
		parent      domain.SessionState
		kind, brief string
		want        string
	}{
		"kind":      {parent, "incident", jobBriefText, `not "incident"`},
		"empty":     {parent, JobRepair, "   ", "at least 20 characters"},
		"short":     {parent, JobImprovement, "faster grep", "at least 20 characters"},
		"long":      {parent, JobRepair, strings.Repeat("x", maxJobBrief+1), "at most"},
		"no model":  {noModel, JobRepair, jobBriefText, "no model"},
		"repair":    {repairMode, JobImprovement, jobBriefText, "cannot start a job"},
		"clone":     {inClone, JobRepair, jobBriefText, "clone"},
		"recursion": {domain.SessionState{ID: "s1", Model: "test/model", Workspace: parent.Workspace}, JobRepair, jobBriefText, "is the repair session of r1"},
	} {
		t.Run(name, func(t *testing.T) {
			rig.registry.records = []domain.RepairRecord{{ID: "r1", Signature: "s", Repository: "o/r", Parent: "q", Session: "s1", Status: domain.RepairBlocked}}
			if _, err := rig.repair.StartJob(context.Background(), tc.parent, tc.kind, tc.brief); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
	if len(rig.clones.requests) != 0 {
		t.Fatal("a refused job must not clone")
	}
	rig.repair.Engine = nil
	if _, err := rig.repair.StartJob(context.Background(), parent, JobRepair, jobBriefText); err == nil || !strings.Contains(err.Error(), "MADE is not connected") {
		t.Fatalf("no engine: %v", err)
	}
}

func TestStartJobRunsThePersonsBriefAndKeepsTheRulesThatStillApply(t *testing.T) {
	rig := newRepairRig(t, &mergeWorkbench{})
	rig.repair.Settings.MaxActive = 2
	// Three improvements by agents of this build exhaust their cap, not
	// the person's.
	for i, id := range []string{"i1", "i2", "i3"} {
		rig.registry.records = append(rig.registry.records, domain.RepairRecord{ID: id, Improvement: true, Signature: "agent" + id, Repository: "o/r", Parent: "q", Session: domain.SessionID("s" + string(rune('1'+i))), Build: "0.3.0-test", Status: domain.RepairCompleted})
	}
	parent := turnSession(t).Export()
	record, err := rig.repair.StartJob(context.Background(), parent, JobImprovement, "  "+jobBriefText+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !record.Improvement || record.Attempt != 1 || record.Parent != parent.ID || !strings.HasPrefix(record.ID, "20261005-1200-localsearch-should") {
		t.Fatalf("record: %+v", record)
	}
	for _, want := range []string{jobBriefText + "\n\nStarted by the person from /jobs in session " + string(parent.ID), "AXLR build 0.3.0-test", "feasible=false"} {
		if !strings.Contains(record.Brief, want) {
			t.Fatalf("brief lacks %q: %s", want, record.Brief)
		}
	}
	waited := waitStatus(t, rig.registry, record.ID, domain.RepairAwaitingMerge)
	child, err := rig.store.Load(context.Background(), waited.Session)
	if err != nil || child.Mode() != domain.ModeImprove || child.Export().Model != parent.Model {
		t.Fatalf("child: %+v %v", child.Export(), err)
	}
	if request := rig.clones.requests[0]; request.Kind != ImproveKind || request.Origin != parent.ID {
		t.Fatalf("clone request: %+v", request)
	}
	if !rig.repair.Live(record.ID) || rig.repair.Live("i1") || rig.repair.ActiveLimit() != 2 {
		t.Fatal("live or limit")
	}
	if _, err := rig.repair.StartJob(context.Background(), parent, JobImprovement, jobBriefText); err == nil || !strings.Contains(err.Error(), "duplicate: improvement") {
		t.Fatalf("duplicate: %v", err)
	}
	rig.repair.Settings.MaxActive = 1
	if _, err := rig.repair.StartJob(context.Background(), parent, JobRepair, "the transcript loses the last line after a resize"); err == nil || !strings.Contains(err.Error(), "1 of 1 jobs are active (jobs.max_active)") {
		t.Fatalf("limit: %v", err)
	}
	rig.repair.Close()
	waitStatus(t, rig.registry, record.ID, domain.RepairInterrupted)
	rig.repair.closed = false
	rig.repair.Settings.MaxActive = 2
	if _, err := rig.repair.StartJob(context.Background(), parent, JobImprovement, jobBriefText); err == nil || !strings.Contains(err.Error(), "was interrupted; recover it") {
		t.Fatalf("interrupted: %v", err)
	}
	// A repair brief that already ran repair.max_attempts sessions is not
	// started again.
	signature := jobSignature("o/r", JobRepair, "the transcript loses the last line after a resize")
	for _, id := range []string{"r1", "r2"} {
		rig.registry.records = append(rig.registry.records, domain.RepairRecord{ID: id, Signature: signature, Repository: "o/r", Parent: "q", Session: domain.SessionID("x" + id), Status: domain.RepairBlocked})
	}
	if _, err := rig.repair.StartJob(context.Background(), parent, JobRepair, "the transcript loses the last line after a resize"); err == nil || !strings.Contains(err.Error(), "attempts exhausted: 2") {
		t.Fatalf("attempts: %v", err)
	}
}

func TestJobRecordsKeepTheStepAttemptAndTheLastCheck(t *testing.T) {
	rig := newRepairRig(t, &mergeWorkbench{})
	run := &repairRun{record: domain.RepairRecord{ID: "j", Signature: "s", Repository: "o/r", Parent: "p", Status: domain.RepairRunning}}
	observer := runObserver{repair: rig.repair, run: run}
	observer.Observe(CeremonyProgress{Step: "repair", StepAttempt: 2, StepLimit: 3, Report: map[string]any{"check": map[string]any{"program": "go", "args": []string{"test", "./..."}, "exit_code": 1, "output_tail": "ok  a\n--- FAIL: TestParse (0.00s)\nFAIL\n"}}})
	record, _ := rig.registry.find("j")
	if record.StepAttempt != 2 || record.StepLimit != 3 || record.Check != "go test ./... exited 1: FAIL" {
		t.Fatalf("record: %+v", record)
	}
	observer.Observe(CeremonyProgress{Step: "watch", Report: map[string]any{"watch": map[string]any{"verdict": "red", "failed": []string{"lint: failure https://example.test/1"}}}})
	record, _ = rig.registry.find("j")
	if record.StepAttempt != 0 || record.StepLimit != 0 || record.Check != "pull request checks red: lint: failure https://example.test/1" {
		t.Fatalf("watch: %+v", record)
	}
	for want, report := range map[string]map[string]any{
		"pull request checks green: 12 passed":        {"watch": map[string]any{"verdict": "green", "passed": 12}},
		"pull request checks blocked: closed outside": {"watch": map[string]any{"verdict": "blocked", "reason": "closed outside"}},
		"make check exited 2":                         {"check": map[string]any{"program": "make", "args": []string{"check"}, "exit_code": 2, "output_tail": ""}},
		"":                                            {"memory": "recorded"},
	} {
		if got := checkSummary(report); got != want {
			t.Fatalf("summary %q, want %q", got, want)
		}
	}
}

func TestPullRequestStatusReadsTheForgeOnlyForAJobWithAPullRequest(t *testing.T) {
	rig := newRepairRig(t, &mergeWorkbench{})
	if _, err := rig.repair.PullRequestStatus(context.Background(), "j"); err == nil || !strings.Contains(err.Error(), "forge is not configured") {
		t.Fatalf("no forge: %v", err)
	}
	forge := &fakeForge{statuses: []PullRequestStatus{{State: "OPEN", MergeState: "BEHIND", Passed: 3}}}
	rig.repair.Forge = forge
	rig.registry.records = []domain.RepairRecord{
		{ID: "j", Signature: "s", Repository: "o/r", Parent: "p", Status: domain.RepairAwaitingMerge, PullRequest: 12, Created: time.Now()},
		{ID: "k", Signature: "s2", Repository: "o/r", Parent: "p", Status: domain.RepairRunning},
	}
	status, err := rig.repair.PullRequestStatus(context.Background(), "j")
	if err != nil || status.MergeState != "BEHIND" || status.Passed != 3 {
		t.Fatalf("status %+v %v", status, err)
	}
	if _, err := rig.repair.PullRequestStatus(context.Background(), "k"); err == nil || !strings.Contains(err.Error(), "no pull request yet") {
		t.Fatalf("no pull request: %v", err)
	}
	if _, err := rig.repair.PullRequestStatus(context.Background(), "z"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown: %v", err)
	}
}
