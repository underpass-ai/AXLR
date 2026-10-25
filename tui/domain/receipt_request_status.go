package domain

// ReceiptRequestStatus is how a model request of the receipt ended.
type ReceiptRequestStatus string

const (
	ReceiptRequestCompleted ReceiptRequestStatus = "completed"
	ReceiptRequestFailed    ReceiptRequestStatus = "failed"
	ReceiptRequestCancelled ReceiptRequestStatus = "cancelled"
	// ReceiptRequestInterrupted: the request started and never ended, the
	// process died during it; its tokens and cost are unknown.
	ReceiptRequestInterrupted ReceiptRequestStatus = "interrupted"
)
