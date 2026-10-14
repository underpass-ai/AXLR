package domain

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

// MaxPlanReturns is how often the person can send a plan back. MADE's
// max_bounces of 3 is only the backstop.
const MaxPlanReturns = 2
