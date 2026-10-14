package application

import (
	"context"
	"fmt"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// Host visibility (decision H5 of the lifecycle integration design): the
// console tells MADE where it stands on every step it claims, as the host
// of that step: running at the claim, finished at the hand-back, blocked
// at a stall, heartbeat while the model works, and renews the claim's
// lease before it runs out. Every failure is logged and never stops a
// turn: the reports are visibility, not the work.

// HostReport is one bounded status report on a claimed step.
type HostReport struct {
	Instance, Step, Fence, OperationID string
	// Attempt is the claimed step record's attempt: MADE refuses a report
	// whose attempt differs from it.
	Attempt int
	// Status is running, blocked or finished; Activity a short label
	// (turn, heartbeat, step, stall).
	Status, Activity string
	// Summary is what the host does, never the prompt or a secret.
	Summary string
	// Execution names the logical execution (the session, or the driven
	// instance), Worker the session; both outlive a console replacement.
	Execution, Worker string
	Role              string
	Model             string
	Sequence          int
	Host              HostIdentity
}

// Host visibility cadence.
const (
	// hostHeartbeat is how often a long turn reports while the model works.
	hostHeartbeat = 5 * time.Minute
	// sessionLease is the turn claim's lease; it is renewed at three
	// quarters.
	sessionLease = time.Hour
)

// report sends one status report for the session's turn and keeps the
// sequence; a failure is logged once per report.
func (c *SessionCeremony) report(ctx context.Context, s domain.Session, instance *domain.SessionInstance, status, activity string) {
	if c == nil || c.Engine == nil || instance.Fence == "" || instance.OperationID == "" {
		return
	}
	state := s.Export()
	instance.Reports++
	report := HostReport{Instance: instance.Instance, Step: sessionTurnStep, Fence: instance.Fence, OperationID: instance.OperationID, Attempt: instance.ClaimAttempt, Status: status, Activity: activity,
		Summary: fmt.Sprintf("turn %d of session %s, mode %s", instance.Turns+1, state.ID, s.Mode()), Execution: string(state.ID), Worker: string(state.ID), Role: sessionRole, Model: string(state.Model), Sequence: instance.Reports, Host: c.Host}
	if err := c.Engine.Report(ctx, report); err != nil {
		if c.firstRefusal(err.Error()) {
			c.log("session %s report %s: %v (an identical refusal is not logged again)", instance.Instance, status, err)
		}
		return
	}
	instance.Reported = c.now()
}

// sessionRole is the console's role in axlr_session.
const sessionRole = "AXLR"

// Heartbeat reports a long turn and renews its lease: called before each
// tool call, it acts when the last report is older than the heartbeat
// interval or the lease is three quarters spent. It changes the session
// in place; the caller saves it.
func (c *SessionCeremony) Heartbeat(ctx context.Context, s *domain.Session) {
	if !c.records(*s) {
		return
	}
	instance, has := s.Instance()
	if !has || instance.Fence == "" || instance.Closed {
		return
	}
	now := c.now()
	changed := false
	if !instance.Reported.IsZero() && now.Sub(instance.Reported) >= hostHeartbeat {
		c.report(ctx, *s, &instance, "running", "heartbeat")
		changed = true
	}
	if c.keepClaim(ctx, *s, &instance) {
		changed = true
	}
	if changed {
		_ = s.SetInstance(instance)
	}
}

// claimed records a fresh claim on the instance: its operation id and
// step attempt (read from MADE, which the claim receipt does not carry),
// its lease and time, then reports running.
func (c *SessionCeremony) claimed(ctx context.Context, s domain.Session, instance *domain.SessionInstance, fence string, lease time.Duration) {
	// Reports count across the instance's claims: MADE orders reports and
	// keys them, so a sequence that restarted at every claim would replay.
	instance.Fence, instance.ClaimedAt, instance.LeaseMS, instance.OperationID, instance.ClaimAttempt, instance.Reported, instance.ClaimIncarnation = fence, c.now(), lease.Milliseconds(), "", 0, time.Time{}, c.Host.incarnation()
	operation, err := c.Engine.Operation(ctx, instance.Instance, sessionTurnStep, fence)
	if err != nil {
		// An id read before the attempt failed is still the claim's.
		instance.OperationID = operation.ID
		c.log("session %s operation id: %v", instance.Instance, err)
		return
	}
	instance.OperationID, instance.ClaimAttempt = operation.ID, operation.Attempt
	c.report(ctx, s, instance, "running", "turn")
}

// handoff declares, before an interrupted claim is completed, that the
// incarnation which made it is gone when it was another console run of
// this install (decision H6): the evidence names that run, the host and
// the time of its last report. A failure is logged; MADE 0.10.0 has no
// grantable action for the declaration yet (13 October 2026).
func (c *SessionCeremony) handoff(ctx context.Context, instance domain.SessionInstance) {
	if c == nil || c.Engine == nil || instance.Fence == "" || instance.ClaimIncarnation == "" || instance.ClaimIncarnation == c.Host.incarnation() {
		return
	}
	last := "never reported"
	if !instance.Reported.IsZero() {
		last = "last reported at " + instance.Reported.UTC().Format(time.RFC3339)
	}
	evidence := fmt.Sprintf("console run %s on host %s claimed at %s, %s; run %s found the claim open", instance.ClaimIncarnation, c.Host.Hostname, instance.ClaimedAt.UTC().Format(time.RFC3339), last, c.Host.incarnation())
	if err := c.Engine.Handoff(ctx, HostHandoff{Instance: instance.Instance, Step: sessionTurnStep, Fence: instance.Fence, Incarnation: instance.ClaimIncarnation, Evidence: evidence}); err != nil {
		c.log("session %s handoff: %v", instance.Instance, err)
		return
	}
	c.log("session %s: handoff from run %s recorded", instance.Instance, instance.ClaimIncarnation)
}

// firstRefusal says whether this report error is new to this console.
func (c *SessionCeremony) firstRefusal(text string) bool {
	c.refusalsMu.Lock()
	defer c.refusalsMu.Unlock()
	if c.refusals[text] {
		return false
	}
	if c.refusals == nil {
		c.refusals = map[string]bool{}
	}
	c.refusals[text] = true
	return true
}

// keepClaim keeps the turn's claim alive: it renews a lease three quarters
// spent and, when the lease ran out (a card answered after an hour, a long
// model stream with no tool), claims the step again so the turn can still
// be completed. Review of 14 October 2026: renewal ran only before a tool
// call, and a completion on an expired fence was refused. It reports
// whether the instance changed.
func (c *SessionCeremony) keepClaim(ctx context.Context, s domain.Session, instance *domain.SessionInstance) bool {
	lease := time.Duration(instance.LeaseMS) * time.Millisecond
	if instance.Fence == "" || lease <= 0 || instance.ClaimedAt.IsZero() {
		return false
	}
	now := c.now()
	spent := now.Sub(instance.ClaimedAt)
	if spent < lease*3/4 {
		return false
	}
	err := c.Engine.Renew(ctx, instance.Instance, sessionTurnStep, instance.Fence, lease)
	if err == nil {
		instance.ClaimedAt = now
		return true
	}
	c.log("session %s lease renewal: %v", instance.Instance, err)
	if spent < lease {
		return false
	}
	key := fmt.Sprintf("axlr:turn:%s:%d:reclaim:%d", s.ID(), instance.TurnIndex, now.Unix())
	fence, err := c.Engine.Claim(ctx, instance.Instance, sessionTurnStep, key, lease)
	if err != nil {
		c.log("session %s claim after an expired lease: %v", instance.Instance, err)
		return false
	}
	c.log("session %s: lease expired after %s, step claimed again", instance.Instance, spent.Round(time.Second))
	c.claimed(ctx, s, instance, fence, lease)
	return true
}
