package domain

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
