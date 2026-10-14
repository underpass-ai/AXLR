package domain

// LedgerEntry is one accepted step: its recorded fields, bounded, and the
// transcript messages it spanned, which axlr_history can still read.
type LedgerEntry struct {
	Step         string
	Iteration    int
	Text         string
	FirstMessage int
	LastMessage  int
}
