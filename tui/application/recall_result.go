package application

// RecallResult is what the read gave: prose and refs for a wake, a compact
// answer for a question or a history. Empty says nothing is recorded under
// the about.
type RecallResult struct {
	Mode, About string
	Text        string
	Refs        []string
	Answer      map[string]any
	Empty       bool
}
