package domain

import (
	"errors"
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

var errReceiptEntryKind = errors.New("unknown receipt entry kind")

// ReceiptEntry is one line of a session's receipt journal, written as a
// model request or a tool call starts and ends, so a receipt read after a
// restart still lists what ran before it.
type ReceiptEntry struct {
	Kind ReceiptEntryKind
	At   time.Time
	// Model is the model a request asked for (request.started), and
	// Route why it went there when the execution routes its requests:
	// ranked first, or the fallback of a failed request.
	Model axlr.ModelID
	Route RouteReason
	// Usage and Status are what an ended request consumed, as its ledger
	// sample says, and how it ended (request.ended).
	Usage  UsageSample
	Status ReceiptRequestStatus
	// Call names the tool call (call.started, call.ended).
	Call axlr.ToolCallID
}

// Validate refuses an entry the journal could not have written.
func (e ReceiptEntry) Validate() error {
	if err := e.Kind.Validate(); err != nil {
		return err
	}
	if e.At.IsZero() {
		return errors.New("a receipt entry needs its time")
	}
	switch e.Kind {
	case ReceiptRequestStarted:
		if _, err := axlr.NewModelID(string(e.Model)); err != nil {
			return err
		}
		switch e.Route {
		case "", RouteRanked, RouteFallback:
		default:
			return errors.New("a request's route is ranked or fallback")
		}
	case ReceiptRequestEnded:
		switch e.Status {
		case ReceiptRequestCompleted, ReceiptRequestFailed, ReceiptRequestCancelled:
		default:
			return errors.New("an ended request needs how it ended")
		}
	default:
		if e.Call == "" {
			return errors.New("a call entry needs its call")
		}
	}
	return nil
}
