package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// fastSleep lets the queue's goroutine poll without waiting half a minute.
func fastSleep(ctx context.Context, _ time.Duration) error {
	time.Sleep(time.Millisecond)
	return ctx.Err()
}

func TestQueuedMergeIsTheApprovalTheCeremonyMergesWith(t *testing.T) {
	bench := &mergeWorkbench{}
	rig := newRepairRig(t, bench)
	out, err := requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	id := out["repair"].(string)
	waitStatus(t, rig.registry, id, domain.RepairAwaitingMerge)
	if err := rig.repair.QueueMerge(context.Background(), id); err == nil || !strings.Contains(err.Error(), "needs the forge") {
		t.Fatalf("no forge: %v", err)
	}
	forge := &queueForge{statuses: map[int][]PullRequestStatus{9: {{State: "OPEN", MergeState: "BEHIND", HeadSHA: "old", Passed: 3}, clean("new")}}}
	rig.repair.Forge, rig.repair.Sleep = forge, fastSleep
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	others := []domain.RepairRecord{
		{ID: "elsewhere", Signature: "e", Repository: "o/r", Parent: "q", Session: "s9", Status: domain.RepairAwaitingMerge, PullRequest: 3, RunToken: "other", Created: now},
		{ID: "stopped", Signature: "f", Repository: "o/r", Parent: "q", Session: "s8", Status: domain.RepairInterrupted, PullRequest: 4, Created: now},
		{ID: "done", Signature: "g", Repository: "o/r", Parent: "q", Session: "s7", Status: domain.RepairCompleted, PullRequest: 5, Created: now},
	}
	for _, record := range others {
		_ = rig.registry.Save(context.Background(), record)
	}
	for wanted, want := range map[string]string{"elsewhere": "owned by another console", "stopped": "recover it first with r", "done": "only a pull request waiting for its merge decision", "nobody": "unknown"} {
		if err := rig.repair.QueueMerge(context.Background(), wanted); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("queue %s: %v, want %q", wanted, err, want)
		}
	}
	if err := rig.repair.QueueMerge(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	record := waitStatus(t, rig.registry, id, domain.RepairCompleted)
	if record.MergeSHA != "def456" || record.Queued.IsZero() || bench.snapshot().decisions[0] != "merge queue" {
		t.Fatalf("merged record %+v, decisions %v", record, bench.snapshot().decisions)
	}
	forge.mu.Lock()
	updates := len(forge.updates)
	forge.mu.Unlock()
	if updates != 1 {
		t.Fatalf("the branch behind its base is updated once: %d", updates)
	}
	for _, other := range []string{"elsewhere", "stopped", "done"} {
		if stored, _ := rig.registry.find(other); !stored.Queued.IsZero() {
			t.Fatalf("%s was queued", other)
		}
	}
}

func TestMergeQueueTakesOnlyQueuedJobsWaitingForTheirMerge(t *testing.T) {
	r := &SelfRepair{}
	at := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	r.runs = map[string]*repairRun{
		"later":      {waiting: true, record: domain.RepairRecord{ID: "later", Status: domain.RepairAwaitingMerge, Queued: at.Add(time.Minute)}},
		"first":      {waiting: true, record: domain.RepairRecord{ID: "first", Status: domain.RepairAwaitingMerge, Queued: at}},
		"not queued": {waiting: true, record: domain.RepairRecord{ID: "not queued", Status: domain.RepairAwaitingMerge}},
		"running":    {waiting: false, record: domain.RepairRecord{ID: "running", Status: domain.RepairRunning, Queued: at.Add(-time.Hour)}},
	}
	r.draining = true
	if head, ok := r.nextQueued(); !ok || head.ID != "first" {
		t.Fatalf("head %s %v", head.ID, ok)
	}
	delete(r.runs, "first")
	if head, ok := r.nextQueued(); !ok || head.ID != "later" {
		t.Fatalf("head %s %v", head.ID, ok)
	}
	delete(r.runs, "later")
	if _, ok := r.nextQueued(); ok || r.draining {
		t.Fatal("a job the person did not queue, or that does not wait for its merge, is never taken")
	}
}

func TestRecoveringAJobAsksForItsMergeToBeQueuedAgain(t *testing.T) {
	bench := &blockingWorkbench{started: make(chan struct{})}
	rig := newRepairRig(t, bench)
	out, err := requestRepair(t, rig, recurringFailure(t), validRepairRequest)
	if err != nil {
		t.Fatal(err)
	}
	id := out["repair"].(string)
	<-bench.started
	rig.repair.Close()
	record := waitStatus(t, rig.registry, id, domain.RepairInterrupted)
	record.Queued, record.QueueNote = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC), "queued"
	_ = rig.registry.Save(context.Background(), record)
	rig.repair.closed = false
	if err := rig.repair.Recover(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	record = waitStatus(t, rig.registry, id, domain.RepairCompleted)
	if !record.Queued.IsZero() || record.QueueNote != "" {
		t.Fatalf("a recovered job keeps its old place in the queue: %+v", record)
	}
	if err := rig.repair.UnqueueMerge(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}
