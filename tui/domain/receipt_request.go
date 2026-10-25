package domain

import (
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ReceiptRequest is one model request of an execution.
type ReceiptRequest struct {
	Session SessionID
	// Index counts the session's requests from 1.
	Index     int
	StartedAt time.Time
	// Duration is zero, unknown, for an interrupted request.
	Duration time.Duration
	Status   ReceiptRequestStatus
	Model    axlr.ModelID
	Usage    UsageSample
}
