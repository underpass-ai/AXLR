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
	return r.Incident != nil && r.Incident.Awaiting != "" || r.Repair != nil && r.Repair.Awaiting != ""
}

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
// resume. The console owns it; MADE and the forge hold the durable record.
type RepairRun struct {
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

// MaxRepairRounds is how many red check rounds go back to repair. MADE's
// max_bounces of 4 is only the backstop.
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
	return r
}
