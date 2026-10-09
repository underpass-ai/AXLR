package domain

import (
	"errors"
	"slices"
)

// CheckCommand is a program and its arguments, run without a shell so the
// console repeats exactly what the user approved.
type CheckCommand struct {
	Program string
	Args    []string
	// MaxOutput raises the output cap for a console command whose output the
	// console parses (git status, gh pr view); the runner bounds it by the
	// runtime's hard limit. Zero keeps the cap of the model's checks. It is
	// never set on an approved command, so it is not persisted or compared.
	MaxOutput int
}

func (c CheckCommand) Equal(other CheckCommand) bool {
	return c.Program == other.Program && slices.Equal(c.Args, other.Args)
}

func (c CheckCommand) IsZero() bool { return c.Program == "" }

// CeremonyRun is what a session knows about its live MADE instance. The
// console drives it; the model only does each step's work.
type CeremonyRun struct {
	Definition string
	Version    string
	Instance   string
	Step       string
	Iteration  int
	Fence      string
	// Check is the command the user approved for this instance; zero until a
	// step proposes one.
	Check CheckCommand
	// About is the exact scope used for initial recall and original MADE inputs.
	About string
	// Memory is the bounded wake text captured when the ceremony began.
	Memory string
	// BudgetBase is the turn's call count when the last step was accepted;
	// the per-turn call limit counts from it, so each step gets a full budget.
	BudgetBase int
	// Reminded is true once the console reminded the model that this step
	// is still open; it reminds once per claimed step.
	Reminded bool
	// Incident is the incident ceremony's state; nil for other ceremonies.
	Incident *IncidentRun
	// Repair is the repair ceremony's state; nil for other ceremonies.
	Repair *RepairRun
	// Plan is the plan ceremony's state; the proposal itself lives in the
	// plan registry. Nil for other ceremonies.
	Plan *PlanRun
	// Task is a plan task's state; nil for other ceremonies.
	Task *TaskRun
	// Model, when set, is the model the console asks while this ceremony is
	// live instead of the session's: the planner for a plan.
	Model string
	// Compact is the small-model profile, decided when the ceremony began
	// and kept for its whole life: fewer calls per step, a smaller context,
	// one step's fields at a time and a projection that starts each step
	// from the ledger.
	Compact bool
	// Ledger holds what each accepted step recorded, in order; under the
	// compact profile a step starts from it instead of the earlier steps'
	// tool chatter.
	Ledger []LedgerEntry
	// StepCall is the axlr_step_done call that opened the current step; the
	// compact projection cuts the transcript after its results.
	StepCall string
}

// LedgerEntry is one accepted step: its recorded fields, bounded, and the
// transcript messages it spanned, which axlr_history can still read.
type LedgerEntry struct {
	Step         string
	Iteration    int
	Text         string
	FirstMessage int
	LastMessage  int
}

// CompactStepCalls is the compact profile's tool-call budget per step.
const CompactStepCalls = 16

// StepCallLimit is the tool-call budget of one step of this run.
func (r CeremonyRun) StepCallLimit() int {
	if r.Compact {
		return CompactStepCalls
	}
	return MaxTurnToolCalls
}

// AwaitingPerson is true while the console waits for the person's decision:
// the model has no step to hand back.
func (r CeremonyRun) AwaitingPerson() bool {
	return r.Incident != nil && r.Incident.Awaiting != "" || r.Repair != nil && r.Repair.Awaiting != "" || r.Plan != nil && r.Plan.Awaiting != ""
}

// PlanRun is what the plan ceremony carries between steps and across a
// resume. The console owns it; the registry holds the proposal and MADE the
// durable record.
type PlanRun struct {
	// ID is the plan's slug, the key of its registry record.
	ID string
	// Awaiting is AwaitingApproval while the plan is with the person.
	Awaiting string
	// Decided is the present decision MADE recorded; Granted is true once
	// the approver granted the guard.
	Decided string
	Granted bool
	// Returns counts the person's returns; ReturnReason is the last one,
	// which reaches the next decompose instruction.
	Returns      int
	ReturnReason string
	// Defects are the last unverified proposal's, for the next round.
	Defects []string
}

// TaskRun is what a task ceremony carries: the task's contract and the
// digests the console compares against. The console owns it.
type TaskRun struct {
	Plan, Task string
	Scope      []string
	Protect    []string
	Check      CheckCommand
	TestFirst  bool
	// Start holds the SHA-256 of every scope, protected and already dirty
	// file when the task started; "" for a file that did not exist.
	Start map[string]string
	// Frozen holds the digests of the test files red named; green must not
	// change them.
	Frozen map[string]string
	// Git is false when the workspace is not a repository: only the digests
	// are enforced and the hand-back says so.
	Git bool
}

// MaxPlanReturns is how often the person can send a plan back. MADE's
// max_bounces of 3 is only the backstop.
const MaxPlanReturns = 2

// Awaiting values: the console waits for the person, not the model.
const AwaitingApproval = "approval"

// IncidentRun is what the incident ceremony carries between steps and
// across a resume. The console owns it; MADE holds the durable record.
type IncidentRun struct {
	Slug, Service, Severity string
	// DraftPath and DraftDigest name the draft the reviewer judged; the
	// person approves exactly those bytes.
	DraftPath, DraftDigest string
	// Findings are the last review's, ReturnReason the person's; both reach
	// the next revise instruction.
	Findings     []string
	ReturnReason string
	Returns      int
	// ReviewFailures counts consecutive reviewer failures.
	ReviewFailures int
	// Awaiting is AwaitingApproval while the draft is with the person.
	Awaiting string
	// Decided is the present decision MADE has recorded ("approve" or
	// "return"); Granted is true once the approver granted the guard. They
	// let a retried keypress skip what already landed.
	Decided string
	Granted bool
	// Published is the approved file's workspace path.
	Published string
}

// RepairRun is what the repair ceremony carries between steps and across a
// resume, and the improve ceremony too: both end in a pull request the
// console opens, watches and merges. The console owns it; MADE and the forge
// hold the durable record.
type RepairRun struct {
	// Improvement is true for axlr_improve: the change is the brief's, not a
	// diagnosed fix, and the merge always waits for the person.
	Improvement bool
	// Repository is owner/name; Base its default branch; Branch the repair
	// branch the console pushes; Slug names both the branch and the memory.
	Repository, Base, Branch, Slug string
	// Title is the pull request title, from the brief's first line.
	Title string
	// PullRequest is zero until propose opened one; URL and HeadSHA follow.
	PullRequest  int
	URL, HeadSHA string
	// Rounds counts the check rounds that came back red.
	Rounds int
	// Feedback is the last red round's failing checks, for the next repair.
	Feedback string
	// Cause is the accepted diagnosis, kept for the pull request body.
	Cause, Fix, Summary string
	// Criteria and Scope are an improvement's accepted brief, kept for the
	// pull request body and the merge card.
	Criteria, Scope string
	// WakeRefs are the memory refs the recall exposed; connect_to may only
	// name these.
	WakeRefs []string
	// Awaiting is AwaitingApproval while the merge waits for the person.
	Awaiting string
	// Decided is the decide output MADE recorded ("approve", "decline" or
	// "automatic"); Granted is true once the approver granted the guard.
	Decided string
	Granted bool
	// MergeSHA is the merge commit once merged.
	MergeSHA string
	// CauseRecorded is true once the diagnosis reached memory; CauseRef is
	// the ref KMP gave it, which the outcome links to.
	CauseRecorded bool
	CauseRef      string
}

// Kind names the ceremony's work for people and memory: "repair" or
// "improvement".
func (r RepairRun) Kind() string {
	if r.Improvement {
		return "improvement"
	}
	return "repair"
}

// MaxRepairRounds is how many red check rounds go back to repair, or to build
// for an improvement. MADE's max_bounces of 4 is only the backstop.
const MaxRepairRounds = 2

// MaxIncidentReturns is how often the person can send a draft back. MADE's
// max_bounces of 3 is only the backstop.
const MaxIncidentReturns = 2

func (r CeremonyRun) Validate() error {
	if r.Definition == "" || r.Version == "" || r.Instance == "" || r.Step == "" || r.Iteration < 1 || r.BudgetBase < 0 {
		return errors.New("ceremony run needs definition, version, instance, step and iteration")
	}
	return nil
}

func (r CeremonyRun) clone() CeremonyRun {
	r.Check.Args = append([]string(nil), r.Check.Args...)
	if r.Incident != nil {
		incident := *r.Incident
		incident.Findings = append([]string(nil), incident.Findings...)
		r.Incident = &incident
	}
	if r.Repair != nil {
		repair := *r.Repair
		repair.WakeRefs = append([]string(nil), repair.WakeRefs...)
		r.Repair = &repair
	}
	r.Ledger = append([]LedgerEntry(nil), r.Ledger...)
	if r.Plan != nil {
		plan := *r.Plan
		plan.Defects = append([]string(nil), plan.Defects...)
		r.Plan = &plan
	}
	if r.Task != nil {
		task := r.Task.Clone()
		r.Task = &task
	}
	return r
}

// Clone copies the task run, maps included.
func (t TaskRun) Clone() TaskRun {
	t.Scope = append([]string(nil), t.Scope...)
	t.Protect = append([]string(nil), t.Protect...)
	t.Check.Args = append([]string(nil), t.Check.Args...)
	t.Start = cloneDigests(t.Start)
	t.Frozen = cloneDigests(t.Frozen)
	return t
}

func cloneDigests(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
