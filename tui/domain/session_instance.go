package domain

import "errors"

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
}

// MaxSessionRecordBytes bounds LastRecord.
const MaxSessionRecordBytes = 16 << 10

func (i SessionInstance) Validate() error {
	if i.Instance == "" && i.Unavailable == "" {
		return errors.New("session instance needs an id or a reason it is unavailable")
	}
	if i.Turns < 0 || i.TurnIndex < 0 || i.ClaimRequests < 0 || i.ClaimCost < 0 {
		return errors.New("invalid session instance counters")
	}
	if len(i.LastRecord) > MaxSessionRecordBytes {
		return errors.New("session record is too large")
	}
	return nil
}

// Instance is the session's MADE instance, if the console opened one or
// recorded why it could not.
func (s Session) Instance() (SessionInstance, bool) {
	if s.state.Instance == nil {
		return SessionInstance{}, false
	}
	return *s.state.Instance, true
}

// SetInstance records the session's MADE instance.
func (s *Session) SetInstance(instance SessionInstance) error {
	if err := instance.Validate(); err != nil {
		return err
	}
	copied := instance
	s.state.Instance = &copied
	return nil
}
