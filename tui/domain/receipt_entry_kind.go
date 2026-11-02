package domain

// ReceiptEntryKind names a line of a session's receipt journal: a model
// request or a tool call starting or ending, or the exact cost of an
// unmetered request settled after it ended.
type ReceiptEntryKind string

const (
	ReceiptRequestStarted ReceiptEntryKind = "request.started"
	ReceiptRequestEnded   ReceiptEntryKind = "request.ended"
	ReceiptCallStarted    ReceiptEntryKind = "call.started"
	ReceiptCallEnded      ReceiptEntryKind = "call.ended"
	ReceiptRequestSettled ReceiptEntryKind = "request.settled"
)

// Validate refuses a kind the journal does not write.
func (k ReceiptEntryKind) Validate() error {
	switch k {
	case ReceiptRequestStarted, ReceiptRequestEnded, ReceiptCallStarted, ReceiptCallEnded, ReceiptRequestSettled:
		return nil
	}
	return errReceiptEntryKind
}
