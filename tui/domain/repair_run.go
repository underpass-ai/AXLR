package domain

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
