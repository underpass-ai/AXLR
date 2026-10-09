package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// QueueMerge records the person's approval of a job's merge as queued: the
// merge queue merges it in turn, once its checks are green on a branch up to
// date with its base. For an improvement this is the person's merge
// decision, as a is. Only a job this console drives and that waits for its
// merge decision is queued; repair.auto_merge is not involved.
func (r *SelfRepair) QueueMerge(ctx context.Context, id string) error {
	if r == nil || r.Registry == nil || r.Forge == nil {
		return errors.New("the merge queue needs the forge, which is not configured in this console")
	}
	r.mu.Lock()
	run, live := r.runs[id]
	var record domain.RepairRecord
	if live {
		record = run.record
	}
	r.mu.Unlock()
	if !live {
		stored, found, err := r.stored(ctx, id)
		switch {
		case err != nil:
			return err
		case !found:
			return fmt.Errorf("job %s is unknown", id)
		case stored.Status == domain.RepairInterrupted:
			return fmt.Errorf("%s %s is interrupted: recover it first with r, then queue its merge", stored.Kind(), id)
		case stored.Active():
			return fmt.Errorf("%s %s is owned by another console; queue its merge there", stored.Kind(), id)
		}
		record = stored
	}
	if record.Status != domain.RepairAwaitingMerge || record.PullRequest == 0 {
		return fmt.Errorf("%s %s is %s; only a pull request waiting for its merge decision is queued", record.Kind(), id, record.Status)
	}
	if !record.Queued.IsZero() {
		return nil
	}
	r.update(ctx, run, func(record *domain.RepairRecord) {
		record.Queued, record.QueueNote, record.Error = r.now(), "queued", ""
	})
	r.kickQueue()
	return nil
}

// UnqueueMerge takes the job off the merge queue; its merge waits for the
// person again.
func (r *SelfRepair) UnqueueMerge(ctx context.Context, id string) error {
	if r == nil || r.Registry == nil {
		return errors.New("self-repair is not configured in this console")
	}
	r.unqueue(ctx, id, "")
	return nil
}

func (r *SelfRepair) stored(ctx context.Context, id string) (domain.RepairRecord, bool, error) {
	records, err := r.Registry.Load(ctx)
	if err != nil {
		return domain.RepairRecord{}, false, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, true, nil
		}
	}
	return domain.RepairRecord{}, false, nil
}

// kickQueue starts the merge queue's goroutine unless it runs. It runs while
// a queued job waits for its merge, so an idle console keeps none.
func (r *SelfRepair) kickQueue() {
	r.mu.Lock()
	if r.draining || r.closed || r.Forge == nil {
		r.mu.Unlock()
		return
	}
	r.draining = true
	if r.queueCtx == nil {
		r.queueCtx, r.queueCancel = context.WithCancel(r.lifetime())
	}
	ctx := r.queueCtx
	r.wg.Add(1)
	r.mu.Unlock()
	queue := &MergeQueue{Forge: r.Forge, Next: r.nextQueued, Approve: r.approveQueued, Unqueue: r.unqueue, Note: r.noteQueue, Settled: r.settled, Watch: r.Settings.Watch, Now: r.Now, Sleep: r.Sleep}
	go func() {
		defer r.wg.Done()
		queue.Drain(ctx)
	}()
}

// nextQueued is the head of the queue: the earliest queued job of this
// console that waits for its merge decision. When there is none the queue's
// goroutine ends, under the same lock a new queued job takes to start it.
func (r *SelfRepair) nextQueued() (domain.RepairRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var head *repairRun
	for _, run := range r.runs {
		record := run.record
		if record.Queued.IsZero() || record.Status != domain.RepairAwaitingMerge || !run.waiting {
			continue
		}
		if head == nil || record.Queued.Before(head.record.Queued) || record.Queued.Equal(head.record.Queued) && record.ID < head.record.ID {
			head = run
		}
	}
	if head == nil {
		r.draining = false
		return domain.RepairRecord{}, false
	}
	return head.record, true
}

// approveQueued hands the queued approval to the job's run, unless the
// person took it off the queue meanwhile.
func (r *SelfRepair) approveQueued(ctx context.Context, id string) error {
	r.mu.Lock()
	run, live := r.runs[id]
	queued := live && !run.record.Queued.IsZero()
	r.mu.Unlock()
	if !queued {
		return ErrNotQueued
	}
	return r.Decide(ctx, id, true, "merge queue")
}

// unqueue clears the job's place in the queue; a reason tells the person why
// the queue let it go.
func (r *SelfRepair) unqueue(ctx context.Context, id, reason string) {
	change := func(record *domain.RepairRecord) {
		record.Queued, record.QueueNote = time.Time{}, ""
		if reason != "" {
			record.Error = bounded("merge queue: "+reason, 600)
		}
	}
	r.mu.Lock()
	run, live := r.runs[id]
	r.mu.Unlock()
	if live {
		r.update(ctx, run, change)
		return
	}
	if record, found, err := r.stored(ctx, id); err == nil && found && !record.Queued.IsZero() {
		change(&record)
		_ = r.save(ctx, &record) // a refusal is kept for Records to report
	}
}

// noteQueue records what the queue does with the job, once per change.
func (r *SelfRepair) noteQueue(ctx context.Context, id, note string) {
	r.mu.Lock()
	run, live := r.runs[id]
	same := live && run.record.QueueNote == note
	r.mu.Unlock()
	if live && !same {
		r.update(ctx, run, func(record *domain.RepairRecord) { record.QueueNote = note })
	}
}

// settled is the job as the queue sees it after the approval.
func (r *SelfRepair) settled(ctx context.Context, id string) (domain.RepairRecord, bool, bool) {
	r.mu.Lock()
	run, live := r.runs[id]
	if live {
		record, waiting := run.record, run.waiting
		r.mu.Unlock()
		return record, true, waiting
	}
	r.mu.Unlock()
	record, _, _ := r.stored(ctx, id)
	return record, false, false
}
