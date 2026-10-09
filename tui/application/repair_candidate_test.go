package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

type fakeCandidates struct {
	mu        sync.Mutex
	buildErr  error
	builds    []string
	installs  []string
	installTo string
}

func (f *fakeCandidates) Build(_ context.Context, clone, slug string) (RepairCandidate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.builds = append(f.builds, clone)
	if f.buildErr != nil {
		return RepairCandidate{}, f.buildErr
	}
	return RepairCandidate{Path: clone + ".axlr-tui", Version: "repair-" + slug + "-abc123", Revision: "abc123"}, nil
}

func (f *fakeCandidates) Install(_ context.Context, candidate RepairCandidate, slug string) (RepairInstallation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installs = append(f.installs, candidate.Path)
	return RepairInstallation{Path: f.installTo, Backup: f.installTo + ".before-repair-" + slug}, nil
}

func (f *fakeCandidates) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.builds), len(f.installs)
}

// Once the checks are green the merge card names the built candidate and
// how to try it on the parent's workspace; the merged repair installs on
// request, the notice names both paths, and the build that ran the repair
// no longer counts as the one that still carries the defect.
func TestMergedRepairBuildsTriesAndInstallsTheRepairedConsole(t *testing.T) {
	bench := &mergeWorkbench{}
	rig := newRepairRig(t, bench)
	candidates := &fakeCandidates{installTo: "/home/me/.local/bin/axlr-tui"}
	rig.repair.Candidates = candidates
	parent := recurringFailure(t)
	if err := rig.store.Save(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	out, err := requestRepair(t, rig, parent, validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	id := out["repair"].(string)
	record := waitStatus(t, rig.registry, id, domain.RepairAwaitingMerge)
	if record.Candidate != record.Clone+".axlr-tui" || record.CandidateVersion != "repair-"+id+"-abc123" || !strings.Contains(record.Pending, "try repair-"+id+"-abc123 first: "+shellQuote(record.Clone+".axlr-tui")+" --root "+shellQuote(string(parent.Export().Workspace))) {
		t.Fatalf("merge card: %+v", record)
	}
	if err := rig.repair.Install(context.Background(), id); err == nil || !strings.Contains(err.Error(), "only a merged repair is installed") {
		t.Fatalf("installed before the merge: %v", err)
	}
	if err := rig.repair.Decide(context.Background(), id, true, ""); err != nil {
		t.Fatal(err)
	}
	record = waitStatus(t, rig.registry, id, domain.RepairCompleted)
	if !Installable(record) || !strings.Contains(record.Notice, "install it with i on the /repair panel") {
		t.Fatalf("merged: %+v", record)
	}
	if err := rig.repair.Install(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	record, _ = rig.registry.find(id)
	if record.Installed != "/home/me/.local/bin/axlr-tui" || record.Backup != "/home/me/.local/bin/axlr-tui.before-repair-"+id || record.Notified || !strings.Contains(record.Notice, "is installed at /home/me/.local/bin/axlr-tui (the previous one is kept as /home/me/.local/bin/axlr-tui.before-repair-"+id) {
		t.Fatalf("installed: %+v", record)
	}
	if err := rig.repair.Install(context.Background(), id); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("installed twice: %v", err)
	}
	if builds, installs := candidates.counts(); builds != 1 || installs != 1 {
		t.Fatalf("builds=%d installs=%d", builds, installs)
	}
	// After the restart the console runs the candidate's build: the same
	// failure is no longer refused as already repaired by this build.
	rig.repair.Build = record.CandidateVersion
	if reason := rig.repair.admit([]domain.RepairRecord{record}, "dddddddddddddddddddddddddddddddd", record.Signature); strings.Contains(reason, "already repaired") {
		t.Fatalf("refused after the update: %s", reason)
	}
	rig.repair.Build = record.Build
	if reason := rig.repair.admit([]domain.RepairRecord{record}, "dddddddddddddddddddddddddddddddd", record.Signature); !strings.Contains(reason, "already repaired") {
		t.Fatalf("old build not refused: %q", reason)
	}
}

// A repaired console that does not build is declined as a red check: the
// ceremony ends BLOCKED with the build output and nothing waits for a merge.
func TestRepairWhoseConsoleDoesNotBuildIsDeclined(t *testing.T) {
	bench := &mergeWorkbench{}
	rig := newRepairRig(t, bench)
	rig.repair.Candidates = &fakeCandidates{buildErr: errors.New("go build: ./main.go:3: syntax error")}
	out, err := requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	record := waitStatus(t, rig.registry, out["repair"].(string), domain.RepairBlocked)
	decisions := bench.snapshot().decisions
	if len(decisions) != 1 || !strings.Contains(decisions[0], "the repaired axlr-tui does not build: go build: ./main.go:3: syntax error") || !strings.Contains(record.Error, "syntax error") || record.Candidate != "" {
		t.Fatalf("decisions=%q record=%+v", decisions, record)
	}
}

// repair.install installs after the merge without the panel; a clone that
// is not AXLR builds nothing and the repair goes on as before.
func TestAutoInstallAndClonesWithoutAConsole(t *testing.T) {
	bench := &mergeWorkbench{}
	rig := newRepairRig(t, bench)
	candidates := &fakeCandidates{installTo: "/opt/axlr-tui"}
	rig.repair.Candidates, rig.repair.AutoInstall = candidates, true
	out, err := requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	id := out["repair"].(string)
	waitStatus(t, rig.registry, id, domain.RepairAwaitingMerge)
	if err := rig.repair.Decide(context.Background(), id, true, ""); err != nil {
		t.Fatal(err)
	}
	record := waitStatus(t, rig.registry, id, domain.RepairCompleted)
	for deadline := time.Now().Add(5 * time.Second); record.Installed == "" && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		record, _ = rig.registry.find(id)
	}
	if record.Installed != "/opt/axlr-tui" || !strings.Contains(record.Notice, "installed at /opt/axlr-tui") {
		t.Fatalf("auto install: %+v", record)
	}

	plain := &mergeWorkbench{}
	rig = newRepairRig(t, plain)
	rig.repair.Candidates = &fakeCandidates{buildErr: ErrNoCandidate}
	out, err = requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	record = waitStatus(t, rig.registry, out["repair"].(string), domain.RepairAwaitingMerge)
	if record.Candidate != "" || strings.Contains(record.Pending, "try") || record.Error != "" {
		t.Fatalf("no console: %+v", record)
	}
}

func TestShellQuoteLeavesPlainPathsAlone(t *testing.T) {
	for in, want := range map[string]string{"/home/me/axlr-tui": "/home/me/axlr-tui", "/tmp/my repo": "'/tmp/my repo'", "/tmp/it's": `'/tmp/it'\''s'`} {
		if got := shellQuote(in); got != want {
			t.Fatalf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

// A console without the toolchain keeps the merge card, without a
// candidate and with the reason, instead of declining a green repair.
func TestRepairWithoutAToolchainKeepsItsMergeCard(t *testing.T) {
	bench := &mergeWorkbench{}
	rig := newRepairRig(t, bench)
	rig.repair.Candidates = &fakeCandidates{buildErr: errors.Join(ErrNoToolchain, errors.New("go is not on the console's PATH"))}
	out, err := requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	record := waitStatus(t, rig.registry, out["repair"].(string), domain.RepairAwaitingMerge)
	if record.Candidate != "" || !strings.Contains(record.Error, "no repaired console was built") || len(bench.snapshot().decisions) != 0 {
		t.Fatalf("record=%+v decisions=%v", record, bench.snapshot().decisions)
	}
}
