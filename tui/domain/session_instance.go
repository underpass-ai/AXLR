package domain

import (
	"errors"
	"time"
)

// SessionInstance is the session's own MADE instance, axlr_session: one
// turn step per prompt, claimed when the prompt arrives and completed with
// the turn's record, by the console alone (decision D4 of the lifecycle
// integration design). Like the ceremony run it lives beside the snapshot.
type SessionInstance struct {
	// Instance is the MADE ceremony id, axlr-session-<session id>, with a
	// suffix for each successor opened when a predecessor's turns ran out.
	Instance string
	// Turns counts the turns completed in this instance.
	Turns int
	// Fence is the live claim of the turn in progress, "" between turns. A
	// fence found at the next claim or at close belongs to an interrupted
	// turn, completed as such before anything else.
	Fence string
	// TurnIndex is the transcript index of the person's prompt the live
	// claim covers.
	TurnIndex int
	// ClaimRequests and ClaimCost are the session's usage ledger at the
	// claim, so the turn's share is the difference at completion.
	ClaimRequests int
	ClaimCost     float64
	// Closed is true once the instance reached DONE or ABANDONED.
	Closed bool
	// LastRecord is the JSON of the last turn record the console wrote,
	// for /session.
	LastRecord string
	// Unavailable is why no instance could be opened (MADE not connected,
	// the definition not published, the engine refusing); the console says
	// it once and tries again at the next prompt.
	Unavailable string
	// Pending are KMP turn entries, as JSON, that KMP did not take when the
	// turn ended (the engine down, a refusal); the console retries them at
	// the next turn end and at close, oldest first.
	Pending []string
	// KMPAgent and KMPContext are the console's KMP identity for this
	// session: the agent registered in the session's store and the context
	// opened for the session, which the console passes on every KMP call
	// (decision D3). The model never sees them.
	KMPAgent   string
	KMPContext string
	// Recalled is the intent the console last woke memory for; "" before
	// the first prompt's recall.
	Recalled string
	// OperationID, ClaimedAt and LeaseMS describe the live claim for the
	// host reports and the lease renewal (decision H5); Reported is the
	// time of the last report and Reports their count on this instance.
	OperationID string
	ClaimedAt   time.Time
	LeaseMS     int64
	Reported    time.Time
	Reports     int
	// ClaimIncarnation is the console incarnation that made the live claim
	// (install id and run token); another incarnation finding it declares
	// a handoff (decision H6).
	ClaimIncarnation string
	// RecalledMemories are the project memories the last recall exposed,
	// "ref text" per line, bounded; Jev reads them to say what a settled
	// turn supersedes (decision F5).
	RecalledMemories []string
}

// MaxSessionRecordBytes bounds LastRecord.
const MaxSessionRecordBytes = 16 << 10

// MaxPendingEntries bounds Pending; beyond it the oldest entry is dropped
// and the loss logged.
const MaxPendingEntries = 50

func (i SessionInstance) Validate() error {
	if i.Instance == "" && i.Unavailable == "" && i.KMPContext == "" {
		return errors.New("session instance needs an id, a reason it is unavailable or a KMP context")
	}
	if i.Turns < 0 || i.TurnIndex < 0 || i.ClaimRequests < 0 || i.ClaimCost < 0 || i.LeaseMS < 0 || i.Reports < 0 {
		return errors.New("invalid session instance counters")
	}
	if len(i.LastRecord) > MaxSessionRecordBytes {
		return errors.New("session record is too large")
	}
	if len(i.Pending) > MaxPendingEntries {
		return errors.New("too many pending session entries")
	}
	return nil
}

// Instance is the session's MADE instance, if the console opened one or
// recorded why it could not.
func (s Session) Instance() (SessionInstance, bool) {
	if s.state.Instance == nil {
		return SessionInstance{}, false
	}
	copied := *s.state.Instance
	copied.Pending = append([]string(nil), copied.Pending...)
	copied.RecalledMemories = append([]string(nil), copied.RecalledMemories...)
	return copied, true
}

// SetInstance records the session's MADE instance.
func (s *Session) SetInstance(instance SessionInstance) error {
	if err := instance.Validate(); err != nil {
		return err
	}
	copied := instance
	copied.Pending = append([]string(nil), instance.Pending...)
	s.state.Instance = &copied
	return nil
}
