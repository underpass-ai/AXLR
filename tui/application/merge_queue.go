package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// MergeQueue merges, one at a time and in the order the person queued them
// on /jobs, the green pull requests of the jobs this console drives. On 9
// October 2026 merging several job pull requests under strict branch
// protection, where a branch must be up to date with main, needed a script
// that updated each branch, waited for the required checks and merged in
// order. The queue does the same through the ceremony: it brings a branch
// that fell behind up to date, waits for its checks and only then hands the
// person's queued approval to the job's run, which merges as on a. It never
// approves a job the person did not queue.
type MergeQueue struct {
	Forge ForgePort
	// Next is the head of the queue; false when the queue is empty.
	Next func() (domain.RepairRecord, bool)
	// Approve hands the queued approval to the job's run.
	Approve func(ctx context.Context, id string) error
	// Unqueue takes the job off the queue with the reason for the person;
	// Note records what the queue is doing with it.
	Unqueue func(ctx context.Context, id, reason string)
	Note    func(ctx context.Context, id, note string)
	// Settled reports the job after the approval: its record, whether this
	// console still drives it and whether it waits for a decision again.
	Settled func(ctx context.Context, id string) (record domain.RepairRecord, live, waiting bool)
	// Poll spaces the forge reads (default 30 s); Watch bounds the wait for
	// a job's checks and for its merge (repair.watch_minutes, default 45).
	Poll, Watch time.Duration
	Now         func() time.Time
	Sleep       func(context.Context, time.Duration) error
}

// ErrNotQueued is Approve's answer for a job the person took off the queue
// while the queue read its checks; the queue then leaves it alone.
var ErrNotQueued = errors.New("the person took the job off the merge queue")

const (
	// maxQueueUpdates bounds the branch updates for one job: a base that
	// keeps moving is the person's to look at.
	maxQueueUpdates = 3
	// maxQueueBlocked is how many reads may show green checks on a merge
	// the forge still blocks before the job leaves the queue, as the
	// ceremony's watch allows.
	maxQueueBlocked = 3
	// queueSettle spaces the reads of the job's own record after the
	// approval, which cost no forge call.
	queueSettle = 5 * time.Second
)

func (q *MergeQueue) now() time.Time {
	if q.Now != nil {
		return q.Now()
	}
	return time.Now()
}

func (q *MergeQueue) poll() time.Duration {
	if q.Poll > 0 {
		return q.Poll
	}
	return repairPoll
}

func (q *MergeQueue) watch() time.Duration {
	if q.Watch > 0 {
		return q.Watch
	}
	return repairWatch
}

func (q *MergeQueue) sleep(ctx context.Context, wait time.Duration) error {
	if q.Sleep != nil {
		return q.Sleep(ctx, wait)
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Drain merges the queued jobs, head first, until the queue is empty or the
// console stops.
func (q *MergeQueue) Drain(ctx context.Context) {
	for ctx.Err() == nil {
		record, ok := q.Next()
		if !ok {
			return
		}
		q.merge(ctx, record)
	}
}

// merge takes the job to its merge, or off the queue with the reason; it
// returns once the job no longer waits in the queue.
func (q *MergeQueue) merge(ctx context.Context, record domain.RepairRecord) {
	id := record.ID
	deadline := q.now().Add(q.watch())
	updates, blocked := 0, 0
	// stale is the head before an update: the forge may report it, with its
	// green checks, for a while after the branch moved.
	stale := ""
	for {
		if current, live, waiting := q.Settled(ctx, id); !live || !waiting || current.Queued.IsZero() || current.Status != domain.RepairAwaitingMerge {
			// The person took it off the queue or decided it meanwhile, or
			// the console stops: nothing is left to merge here. The test
			// is Next's, so a job Next returns is never skipped forever.
			return
		}
		status, err := q.Forge.Status(ctx, record.Repository, record.PullRequest)
		if ctx.Err() != nil {
			return
		}
		green := status.Pending == 0 && len(status.Failed) == 0 && status.Passed > 0
		switch {
		case err != nil:
			q.Unqueue(ctx, id, "the pull request could not be read: "+bounded(err.Error(), 300))
			return
		case status.State == "MERGED" || status.State == "CLOSED":
			q.Unqueue(ctx, id, fmt.Sprintf("the pull request was %s outside the console", strings.ToLower(status.State)))
			return
		case stale != "" && status.HeadSHA == stale:
			q.Note(ctx, id, "waiting for the forge to show the updated branch")
		case len(status.Failed) > 0 && status.Pending == 0:
			q.Unqueue(ctx, id, "checks failed: "+bounded(strings.Join(status.Failed, "; "), 400))
			return
		case status.Pending == 0 && status.MergeState == "DIRTY":
			q.Unqueue(ctx, id, "the pull request conflicts with its base")
			return
		case status.Pending == 0 && status.MergeState == "BEHIND":
			if updates == maxQueueUpdates {
				q.Unqueue(ctx, id, fmt.Sprintf("the base kept moving after %d branch updates", updates))
				return
			}
			if err := q.Forge.UpdateBranch(ctx, record.Repository, record.PullRequest); err != nil {
				if ctx.Err() == nil {
					q.Unqueue(ctx, id, "the branch is behind and could not be updated: "+bounded(err.Error(), 300))
				}
				return
			}
			updates, stale, blocked = updates+1, status.HeadSHA, 0
			deadline = q.now().Add(q.watch())
			q.Note(ctx, id, "branch brought up to date with its base; waiting for its checks")
		case green && (status.MergeState == "CLEAN" || status.MergeState == "HAS_HOOKS"):
			q.Note(ctx, id, fmt.Sprintf("checks green (%d passed); merging", status.Passed))
			if err := q.Approve(ctx, id); err != nil {
				if ctx.Err() == nil && !errors.Is(err, ErrNotQueued) {
					q.Unqueue(ctx, id, "the approval was not taken: "+bounded(err.Error(), 300))
				}
				return
			}
			q.settle(ctx, id)
			return
		case green && status.MergeState == "BLOCKED":
			if blocked++; blocked >= maxQueueBlocked {
				q.Unqueue(ctx, id, "checks passed but the forge blocks the merge (review or protection rule)")
				return
			}
		default:
			q.Note(ctx, id, fmt.Sprintf("waiting for checks: %d pending, %d passed (%s)", status.Pending, status.Passed, strings.ToLower(status.MergeState)))
		}
		if !q.now().Before(deadline) {
			q.Unqueue(ctx, id, fmt.Sprintf("checks did not settle within %s", q.watch()))
			return
		}
		if q.sleep(ctx, q.poll()) != nil {
			return
		}
	}
}

// settle waits until the approved job ends: merged, or blocked by a merge
// the forge refused, or interrupted with the console. A job back at its
// merge decision, an approval that did not merge, leaves the queue so it is
// not approved again.
func (q *MergeQueue) settle(ctx context.Context, id string) {
	deadline := q.now().Add(q.watch())
	for {
		if q.sleep(ctx, min(q.poll(), queueSettle)) != nil {
			return
		}
		record, live, waiting := q.Settled(ctx, id)
		switch {
		case record.Status.Terminal() || !live:
			return
		case waiting && record.Status == domain.RepairAwaitingMerge:
			q.Unqueue(ctx, id, "the approval did not merge: "+orNone(record.Error))
			return
		case !q.now().Before(deadline):
			q.Unqueue(ctx, id, fmt.Sprintf("the merge did not end within %s", q.watch()))
			return
		}
	}
}

func orNone(text string) string {
	if text == "" {
		return "no reason was recorded"
	}
	return text
}
