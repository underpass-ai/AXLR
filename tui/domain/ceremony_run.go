package domain

import (
	"errors"
	"slices"
)

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
	// Approved holds every command the person approved on a step's card in
	// this instance, accepted or refused, so the same command is not asked
	// again: run.Check is kept only when a step is accepted.
	Approved []CheckCommand
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
	// LimitStep names the step and attempt ("step#iteration") whose budget
	// the console restarted at the call limit, LimitResumes how many times.
	LimitStep    string
	LimitResumes int
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
	// OperationID is the live claim's operation id and Reports the count
	// of host reports on this instance (decision H5).
	OperationID string
	Reports     int
	// Generic is the state of a definition driven through /ceremony's
	// generic loop; nil for the console's own definitions.
	Generic *GenericRun
}

// CompactStepCalls is the compact profile's tool-call budget per step.
const CompactStepCalls = 16

// StepCallLimit is the tool-call budget of one step of this run, given the
// turn's: a compact step keeps its smaller one.
func (r CeremonyRun) StepCallLimit(turnLimit int) int {
	if r.Compact {
		return min(CompactStepCalls, turnLimit)
	}
	return turnLimit
}

// MaxApprovedChecks bounds Approved; the oldest approvals go first.
const MaxApprovedChecks = 32

// Approves is true when the person already approved command in this
// instance: it is the run's check or one they approved on a card.
func (r CeremonyRun) Approves(command CheckCommand) bool {
	return command.Equal(r.Check) || slices.ContainsFunc(r.Approved, command.Equal)
}

// Approve records commands the person approved on a step's card.
func (r *CeremonyRun) Approve(commands ...CheckCommand) {
	for _, command := range commands {
		if command.IsZero() || r.Approves(command) {
			continue
		}
		r.Approved = append(r.Approved, CheckCommand{Program: command.Program, Args: append([]string(nil), command.Args...)})
	}
	if len(r.Approved) > MaxApprovedChecks {
		r.Approved = append([]CheckCommand(nil), r.Approved[len(r.Approved)-MaxApprovedChecks:]...)
	}
}

// AwaitingPerson is true while the console waits for the person's decision:
// the model has no step to hand back.
func (r CeremonyRun) AwaitingPerson() bool {
	return r.Incident != nil && r.Incident.Awaiting != "" || r.Repair != nil && r.Repair.Awaiting != "" || r.Plan != nil && r.Plan.Awaiting != ""
}

// Awaiting values: the console waits for the person, not the model.
const AwaitingApproval = "approval"

func (r CeremonyRun) Validate() error {
	if r.Definition == "" || r.Version == "" || r.Instance == "" || r.Step == "" || r.Iteration < 1 || r.BudgetBase < 0 {
		return errors.New("ceremony run needs definition, version, instance, step and iteration")
	}
	return nil
}

func (r CeremonyRun) clone() CeremonyRun {
	r.Check.Args = append([]string(nil), r.Check.Args...)
	if r.Approved != nil {
		approved := make([]CheckCommand, len(r.Approved))
		for i, command := range r.Approved {
			approved[i] = CheckCommand{Program: command.Program, Args: append([]string(nil), command.Args...)}
		}
		r.Approved = approved
	}
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
	if r.Generic != nil {
		generic := GenericRun{Prompts: make(map[string]string, len(r.Generic.Prompts)), Terminal: append([]string(nil), r.Generic.Terminal...)}
		for step, prompt := range r.Generic.Prompts {
			generic.Prompts[step] = prompt
		}
		r.Generic = &generic
	}
	if r.Task != nil {
		task := r.Task.Clone()
		r.Task = &task
	}
	return r
}
