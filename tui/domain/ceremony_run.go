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
}

// AwaitingPerson is true while the console waits for the person's decision:
// the model has no step to hand back.
func (r CeremonyRun) AwaitingPerson() bool {
	return r.Incident != nil && r.Incident.Awaiting != ""
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
	return r
}
