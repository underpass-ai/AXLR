package domain

import (
	"errors"
	"time"
)

// RepairStatus is where a self-repair stands. The console owns the record;
// MADE and the forge hold the durable evidence it points at.
type RepairStatus string

const (
	// RepairPreparing is set while the clone is being made; no session yet.
	RepairPreparing RepairStatus = "preparing"
	// RepairRunning means the repair session works without the person.
	RepairRunning RepairStatus = "running"
	// RepairAwaitingApproval means the repair session's next tool call waits
	// for the person: the reproduction command, or a tool the policy keeps
	// under approval.
	RepairAwaitingApproval RepairStatus = "awaiting_approval"
	// RepairAwaitingMerge means the pull request is green and the merge waits
	// for the person's decision.
	RepairAwaitingMerge RepairStatus = "awaiting_merge"
	// RepairCompleted means the pull request merged.
	RepairCompleted RepairStatus = "completed"
	// RepairBlocked means the ceremony ended BLOCKED; the reason is kept.
	RepairBlocked RepairStatus = "blocked"
	// RepairFailed means the console could not carry the repair: a clone that
	// failed, a session that could not continue. The evidence is kept.
	RepairFailed RepairStatus = "failed"
	// RepairInterrupted means the console stopped while the repair ran; the
	// record can be recovered without repeating effects blindly.
	RepairInterrupted RepairStatus = "interrupted"
)

// Terminal reports statuses that no longer change on their own.
func (s RepairStatus) Terminal() bool {
	return s == RepairCompleted || s == RepairBlocked || s == RepairFailed
}

// Awaiting reports statuses that wait for the person's decision.
func (s RepairStatus) Awaiting() bool {
	return s == RepairAwaitingApproval || s == RepairAwaitingMerge
}

// RepairRecord links a session that detected a defect of AXLR with the
// separate session that repairs it, and keeps what both need to show:
// progress, the pull request, the outcome and the integration reports. An
// improvement the agent requested is recorded the same way.
type RepairRecord struct {
	ID string
	// Improvement is true when the separate session improves AXLR through
	// axlr_improve instead of repairing a defect.
	Improvement bool
	// Signature identifies the failure (repository, tool and failure class),
	// so the same defect is not repaired twice at once and a merged repair is
	// not requested again by the build that still carries the defect.
	Signature  string
	Repository string
	// Parent is the session that requested the repair; Session the repair
	// session, empty until the clone exists.
	Parent, Session SessionID
	Clone           string
	Brief           string
	// Build is the console build that detected the defect.
	Build  string
	Status RepairStatus
	// Step and State mirror the ceremony while it runs; Instance names the
	// MADE instance once started.
	Step, State, Instance string
	// StepAttempt and StepLimit are the current step's attempt and the
	// bound the console keeps on it; StepLimit is zero for a step the
	// console does not repeat.
	StepAttempt, StepLimit int
	// Check is the last check result the ceremony reported, bounded: the
	// check command's exit with its last output line, or the verdict on
	// the pull request's checks.
	Check string
	// Queued is when the person queued the merge on /jobs, zero when it is
	// not queued; QueueNote is the merge queue's last word about it.
	Queued        time.Time
	QueueNote     string
	PullRequest   int
	URL, MergeSHA string
	// Pending describes the decision the person owes while Awaiting.
	Pending string
	// Memory is the last KMP report the ceremony gave; Error the last failure.
	Memory, Error string
	// Attempt counts repair sessions started for this signature.
	Attempt int
	// RunToken names the console launch that owns a running record; a record
	// found running under another token was interrupted.
	RunToken string
	// Notice is the message for the parent session once the repair ends;
	// Notified is true once a turn carried it.
	Notice   string
	Notified bool
	Created  time.Time
	Updated  time.Time
}

func (r RepairRecord) Validate() error {
	if r.ID == "" || r.Signature == "" || r.Repository == "" || r.Parent == "" || r.Status == "" {
		return errors.New("repair record needs id, signature, repository, parent and status")
	}
	switch r.Status {
	case RepairPreparing, RepairRunning, RepairAwaitingApproval, RepairAwaitingMerge, RepairCompleted, RepairBlocked, RepairFailed, RepairInterrupted:
		return nil
	}
	return errors.New("unknown repair status")
}

// Kind names the record's work for people: "repair" or "improvement".
func (r RepairRecord) Kind() string {
	if r.Improvement {
		return "improvement"
	}
	return "repair"
}

// Active reports a record that still occupies the repair slot.
func (r RepairRecord) Active() bool {
	return !r.Status.Terminal() && r.Status != RepairInterrupted
}
