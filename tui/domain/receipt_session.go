package domain

// ReceiptSession is what a receipt reads of one session: the session, its
// journal and its ledger.
type ReceiptSession struct {
	Session Session
	Entries []ReceiptEntry
	Ledger  UsageLedger
}
