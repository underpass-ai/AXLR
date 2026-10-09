package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// queueForge scripts the forge per pull request: each read takes the next
// status and the last one repeats. The queue never proposes or merges.
type queueForge struct {
	mu       sync.Mutex
	statuses map[int][]PullRequestStatus
	reads    []int
	updates  []int
	err      error
}

func (f *queueForge) Propose(context.Context, RepairProposal) (PullRequest, error) {
	return PullRequest{}, errors.New("the merge queue never proposes")
}
func (f *queueForge) Merge(context.Context, string, int) (string, error) {
	return "", errors.New("the merge queue never merges by itself")
}
func (f *queueForge) Status(_ context.Context, _ string, number int) (PullRequestStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, number)
	if f.err != nil {
		return PullRequestStatus{}, f.err
	}
	scripted := f.statuses[number]
	if len(scripted) == 0 {
		return PullRequestStatus{}, errors.New("no scripted status")
	}
	status := scripted[0]
	if len(scripted) > 1 {
		f.statuses[number] = scripted[1:]
	}
	return status, nil
}
func (f *queueForge) UpdateBranch(_ context.Context, _ string, number int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, number)
	return nil
}

// queueJobs is the coordinator's side of the queue: jobs queued in order, an
// approval that merges unless told otherwise.
type queueJobs struct {
	queued     []domain.RepairRecord
	approved   []string
	unqueued   map[string]string
	notes      []string
	approveErr error
	// backAtDecision makes an approval return the job to its merge
	// decision, as a merge the approver could not record does.
	backAtDecision bool
	// readsAtApproval is how many forge reads preceded each approval.
	readsAtApproval []int
	// takenOffAfter, when set, is the forge read after which the person
	// takes the first job off the queue.
	takenOffAfter int
	forge         *queueForge
}

func (j *queueJobs) next() (domain.RepairRecord, bool) {
	for _, record := range j.queued {
		if !record.Queued.IsZero() {
			return record, true
		}
	}
	return domain.RepairRecord{}, false
}
func (j *queueJobs) approve(_ context.Context, id string) error {
	if j.approveErr != nil {
		return j.approveErr
	}
	j.approved = append(j.approved, id)
	j.readsAtApproval = append(j.readsAtApproval, len(j.forge.reads))
	return nil
}
func (j *queueJobs) unqueue(_ context.Context, id, reason string) {
	if j.unqueued == nil {
		j.unqueued = map[string]string{}
	}
	j.unqueued[id] = reason
	j.drop(id)
}
func (j *queueJobs) note(_ context.Context, id, note string) { j.notes = append(j.notes, id+": "+note) }
func (j *queueJobs) settled(_ context.Context, id string) (domain.RepairRecord, bool, bool) {
	for i, record := range j.queued {
		if record.ID != id || slices.Contains(j.approved, id) {
			continue
		}
		if i == 0 && j.takenOffAfter > 0 && len(j.forge.reads) >= j.takenOffAfter {
			j.queued[i].Queued = time.Time{}
		}
		return j.queued[i], true, true
	}
	if j.backAtDecision {
		return domain.RepairRecord{ID: id, Status: domain.RepairAwaitingMerge, Error: "merge decision: approve as the person: denied"}, true, true
	}
	j.drop(id)
	return domain.RepairRecord{ID: id, Status: domain.RepairCompleted}, false, false
}
func (j *queueJobs) drop(id string) {
	for i, record := range j.queued {
		if record.ID == id {
			j.queued = append(j.queued[:i], j.queued[i+1:]...)
			return
		}
	}
}

// queueRig runs the queue on a clock that sleeps by moving forward.
func queueRig(statuses map[int][]PullRequestStatus, ids ...string) (*MergeQueue, *queueJobs, *queueForge) {
	forge := &queueForge{statuses: statuses}
	jobs := &queueJobs{forge: forge}
	for i, id := range ids {
		jobs.queued = append(jobs.queued, domain.RepairRecord{ID: id, Repository: "o/r", PullRequest: i + 1, Status: domain.RepairAwaitingMerge, Queued: time.Date(2026, 10, 9, 11, 0, i, 0, time.UTC)})
	}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	queue := &MergeQueue{Forge: forge, Next: jobs.next, Approve: jobs.approve, Unqueue: jobs.unqueue, Note: jobs.note, Settled: jobs.settled, Poll: 30 * time.Second, Watch: 10 * time.Minute,
		Now:   func() time.Time { return now },
		Sleep: func(ctx context.Context, wait time.Duration) error { now = now.Add(wait); return ctx.Err() }}
	return queue, jobs, forge
}

func clean(head string) PullRequestStatus {
	return PullRequestStatus{State: "OPEN", MergeState: "CLEAN", HeadSHA: head, Passed: 5}
}

func TestMergeQueueMergesGreenPullRequestsInQueueOrder(t *testing.T) {
	queue, jobs, forge := queueRig(map[int][]PullRequestStatus{1: {clean("a")}, 2: {{State: "OPEN", MergeState: "HAS_HOOKS", HeadSHA: "b", Passed: 2}}}, "first", "second")
	queue.Drain(context.Background())
	if strings.Join(jobs.approved, ",") != "first,second" || len(jobs.unqueued) != 0 || len(forge.updates) != 0 {
		t.Fatalf("approved %v unqueued %v updates %v", jobs.approved, jobs.unqueued, forge.updates)
	}
	if forge.reads[0] != 1 || forge.reads[len(forge.reads)-1] != 2 {
		t.Fatalf("reads %v", forge.reads)
	}
}

// Under strict protection a branch behind main cannot merge: the queue
// updates it and approves only once the new head's checks are green, never
// on the old head's green checks the forge may still report.
func TestMergeQueueUpdatesABranchBehindAndWaitsForItsNewChecks(t *testing.T) {
	queue, jobs, forge := queueRig(map[int][]PullRequestStatus{1: {
		{State: "OPEN", MergeState: "BEHIND", HeadSHA: "old", Passed: 5},
		{State: "OPEN", MergeState: "CLEAN", HeadSHA: "old", Passed: 5},
		{State: "OPEN", MergeState: "BLOCKED", HeadSHA: "new", Pending: 3, Passed: 2},
		clean("new"),
	}}, "behind")
	queue.Drain(context.Background())
	if len(forge.updates) != 1 || strings.Join(jobs.approved, ",") != "behind" || jobs.readsAtApproval[0] != 4 {
		t.Fatalf("updates %v approved %v after %v reads", forge.updates, jobs.approved, jobs.readsAtApproval)
	}
	notes := strings.Join(jobs.notes, "\n")
	for _, want := range []string{"branch brought up to date", "waiting for the forge to show the updated branch", "waiting for checks: 3 pending, 2 passed", "checks green (5 passed); merging"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("notes lack %q:\n%s", want, notes)
		}
	}
}

func TestMergeQueueLetsGoOfWhatItCannotMerge(t *testing.T) {
	red := PullRequestStatus{State: "OPEN", MergeState: "BLOCKED", HeadSHA: "new", Passed: 4, Failed: []string{"lint: failure https://example.test/1"}}
	greenBlocked := PullRequestStatus{State: "OPEN", MergeState: "BLOCKED", HeadSHA: "h", Passed: 4}
	for name, tc := range map[string]struct {
		statuses []PullRequestStatus
		want     string
		updates  int
	}{
		"red after update": {[]PullRequestStatus{{State: "OPEN", MergeState: "BEHIND", HeadSHA: "old", Passed: 5}, {State: "OPEN", MergeState: "BLOCKED", HeadSHA: "new", Pending: 1}, red}, "checks failed: lint: failure", 1},
		"closed":           {[]PullRequestStatus{{State: "CLOSED", MergeState: "CLEAN", Passed: 5}}, "the pull request was closed outside the console", 0},
		"merged":           {[]PullRequestStatus{{State: "MERGED"}}, "the pull request was merged outside the console", 0},
		"conflict":         {[]PullRequestStatus{{State: "OPEN", MergeState: "DIRTY", Passed: 5}}, "conflicts with its base", 0},
		"protection":       {[]PullRequestStatus{greenBlocked}, "forge blocks the merge", 0},
		"stall":            {[]PullRequestStatus{{State: "OPEN", MergeState: "BLOCKED", Pending: 2}}, "checks did not settle within 10m0s", 0},
		"base keeps moving": {[]PullRequestStatus{
			{State: "OPEN", MergeState: "BEHIND", HeadSHA: "h1", Passed: 5}, {State: "OPEN", MergeState: "BEHIND", HeadSHA: "h2", Passed: 5},
			{State: "OPEN", MergeState: "BEHIND", HeadSHA: "h3", Passed: 5}, {State: "OPEN", MergeState: "BEHIND", HeadSHA: "h4", Passed: 5},
		}, "the base kept moving after 3 branch updates", 3},
	} {
		t.Run(name, func(t *testing.T) {
			queue, jobs, forge := queueRig(map[int][]PullRequestStatus{1: tc.statuses, 2: {clean("next")}}, "stuck", "next")
			queue.Drain(context.Background())
			if !strings.Contains(jobs.unqueued["stuck"], tc.want) || len(forge.updates) != tc.updates {
				t.Fatalf("unqueued %q, want %q; updates %v", jobs.unqueued["stuck"], tc.want, forge.updates)
			}
			if strings.Join(jobs.approved, ",") != "next" {
				t.Fatalf("a job the queue let go is never approved, and the next one still merges: %v", jobs.approved)
			}
		})
	}
	queue, jobs, forge := queueRig(map[int][]PullRequestStatus{1: {clean("a")}}, "unreadable")
	forge.err = errors.New("gh: not logged in")
	queue.Drain(context.Background())
	if !strings.Contains(jobs.unqueued["unreadable"], "could not be read: gh: not logged in") {
		t.Fatalf("unreadable: %v", jobs.unqueued)
	}
	queue, jobs, _ = queueRig(map[int][]PullRequestStatus{1: {clean("a")}}, "refused")
	jobs.approveErr = errors.New("repair refused is not waiting for a decision")
	queue.Drain(context.Background())
	if !strings.Contains(jobs.unqueued["refused"], "approval was not taken") {
		t.Fatalf("refused approval: %v", jobs.unqueued)
	}
	queue, jobs, _ = queueRig(map[int][]PullRequestStatus{1: {clean("a")}}, "back")
	jobs.backAtDecision = true
	queue.Drain(context.Background())
	if len(jobs.approved) != 1 || !strings.Contains(jobs.unqueued["back"], "the approval did not merge: merge decision: approve as the person: denied") {
		t.Fatalf("an approval that did not merge is not repeated: %v %v", jobs.approved, jobs.unqueued)
	}
}

func TestMergeQueueNeverApprovesAJobThePersonTookOff(t *testing.T) {
	queue, jobs, forge := queueRig(map[int][]PullRequestStatus{1: {{State: "OPEN", MergeState: "BLOCKED", Pending: 2}, clean("a")}, 2: {clean("b")}}, "taken off", "kept")
	jobs.takenOffAfter = 1
	queue.Drain(context.Background())
	if strings.Join(jobs.approved, ",") != "kept" || len(forge.reads) != 2 {
		t.Fatalf("approved %v after reads %v", jobs.approved, forge.reads)
	}
}

func TestMergeQueueStopsWithTheConsole(t *testing.T) {
	queue, jobs, _ := queueRig(map[int][]PullRequestStatus{1: {{State: "OPEN", MergeState: "BLOCKED", Pending: 1}}}, "pending")
	ctx, cancel := context.WithCancel(context.Background())
	sleep := queue.Sleep
	queue.Sleep = func(ctx context.Context, wait time.Duration) error {
		cancel()
		return sleep(ctx, wait)
	}
	queue.Drain(ctx)
	if len(jobs.unqueued) != 0 || len(jobs.approved) != 0 {
		t.Fatalf("a stopping console keeps the queue as it is: %v %v", jobs.unqueued, jobs.approved)
	}
}
