package domain

import (
	"errors"
	"regexp"
	"time"
)

// PlanStatus is where a plan stands. The console owns the record in the
// plan registry; MADE holds the durable plan, task and sync instances.
type PlanStatus string

const (
	PlanDecomposing PlanStatus = "decomposing"
	// PlanAwaitingApproval means the verified plan waits for the person.
	PlanAwaitingApproval PlanStatus = "awaiting_approval"
	// PlanReady means the person approved it; no task has started yet.
	PlanReady PlanStatus = "ready"
	// PlanRunning means its tasks and syncs are being driven.
	PlanRunning PlanStatus = "running"
	// PlanDone means every task ended DONE and every sync SYNCED.
	PlanDone PlanStatus = "done"
	// PlanPartial means some task or sync ended BLOCKED; the rest is kept.
	PlanPartial PlanStatus = "partial"
	// PlanBlocked means the plan itself never became ready.
	PlanBlocked PlanStatus = "blocked"
	// PlanDeclined means the person declined it.
	PlanDeclined PlanStatus = "declined"
	// PlanInterrupted means the console stopped while it ran.
	PlanInterrupted PlanStatus = "interrupted"
)

// Terminal reports statuses that no longer change on their own.
func (s PlanStatus) Terminal() bool {
	return s == PlanDone || s == PlanPartial || s == PlanBlocked || s == PlanDeclined
}

// TaskStatus is where one task of a plan stands.
type TaskStatus string

const (
	TaskPending TaskStatus = "pending"
	TaskRunning TaskStatus = "running"
	TaskDone    TaskStatus = "done"
	TaskBlocked TaskStatus = "blocked"
	// TaskSkipped means a task it depends on ended blocked.
	TaskSkipped TaskStatus = "skipped"
)

// Citation points a worker at the code it must read: the console verifies
// the quote is on that line and extracts the region into the task's pack.
type Citation struct {
	Path  string
	Line  int
	Quote string
}

// TaskNote is the only channel between tasks: written by a worker's
// hand-back, relayed by the console to the addressed tasks.
type TaskNote struct {
	From, To, Text string
}

// TaskHandback is what a finished task leaves for the others and the person.
type TaskHandback struct {
	Summary, SummaryEN string
	Notes              []TaskNote
	Questions          []string
	Changed            []string
	Revision           string
}

// PlanTask is one atomic task: its contract from the approved plan and,
// once it runs, where it stands.
type PlanTask struct {
	ID, Goal string
	// Scope lists the paths the worker may change; New those of them that
	// did not exist when the plan was verified.
	Scope, New []string
	Context    []Citation
	UnitCheck  CheckCommand
	// Baseline is the unit check's exit code when the plan was verified.
	Baseline  int
	DependsOn []string
	TestFirst bool
	Protect   []string
	// Wave is the task's position in the dependency order, from 1.
	Wave int
	// Pack is the context the worker starts from, built at verification.
	Pack string

	Status   TaskStatus
	Session  SessionID
	Instance string
	Step     string
	Reason   string
	Handback *TaskHandback
}

// SyncRecord is one wave's integration.
type SyncRecord struct {
	Wave     int
	Instance string
	Rounds   int
	Verdict  string
	Output   string
}

// PlanRecord is a plan in the registry.
type PlanRecord struct {
	ID        string
	Session   SessionID
	Brief     string
	Workspace Workspace
	// Planner is the model that decomposed it; Worker the tasks' model.
	Planner, Worker string
	Instance        string
	Status          PlanStatus
	Tasks           []PlanTask
	E2E             CheckCommand
	E2EBaseline     int
	Interfaces      string
	SummaryEN       string
	Waves           int
	Defects         []string
	// Decision is the person's (approve, return, decline or automatic) and
	// Reason the text of a return or decline.
	Decision, Reason string
	Syncs            []SyncRecord
	Error            string
	RunToken         string
	Created, Updated time.Time
}

var planSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// ValidSlug reports whether text can name a plan or a task.
func ValidSlug(text string) bool { return planSlug.MatchString(text) }

func (r PlanRecord) Validate() error {
	if !ValidSlug(r.ID) || r.Session == "" || r.Status == "" {
		return errors.New("plan record needs an id, a session and a status")
	}
	return nil
}

// Task returns the task with id.
func (r *PlanRecord) Task(id string) (*PlanTask, bool) {
	for i := range r.Tasks {
		if r.Tasks[i].ID == id {
			return &r.Tasks[i], true
		}
	}
	return nil, false
}
