package application

// PullRequestStatus is what the forge reports about an open pull request.
type PullRequestStatus struct {
	// State is OPEN, MERGED or CLOSED.
	State string
	// MergeState is the forge's merge state: CLEAN, BEHIND, BLOCKED, DIRTY,
	// UNSTABLE, HAS_HOOKS or UNKNOWN.
	MergeState string
	HeadSHA    string
	// Pending, Passed and Failed count the head commit's checks; Failed names
	// each failing check with its conclusion.
	Pending, Passed int
	Failed          []string
}
