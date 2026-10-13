package application

// RecallRequest is one read of memory under an about: a wake (with an
// intent the store may focus on), a question, or a span of history (Since
// onwards, else the newest Entries).
type RecallRequest struct {
	Mode, About, Intent, Since string
	Entries                    int
	ContextID                  string
}
