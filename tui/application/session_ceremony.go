package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The session ceremony: every console session is a MADE axlr_session
// instance with one turn step per prompt (decision D4 of the lifecycle
// integration design). The console claims the step when the prompt arrives
// and completes it with the turn's record; the model is never asked.
const (
	SessionDefinition        = "axlr_session"
	SessionDefinitionVersion = "1.0"
	sessionTurnStep          = "turn"
	sessionClosedTrigger     = "closed"
	sessionAbandonedTrigger  = "abandoned"
)

// ErrEntryRecorded is the journal's answer to an entry whose idempotency
// key the store accepted already: nothing to retry.
var ErrEntryRecorded = errors.New("entry recorded already")

// SessionCeremony runs the session's MADE instance. Every failure of the
// engines is logged and leaves the turn to run: the record is the
// console's bookkeeping, not the person's work.
type SessionCeremony struct {
	Engine CeremonyEnginePort
	// Journal writes the KMP turn entry; nil writes none.
	Journal JournalPort
	Labels  SessionLabelsPort
	// Usage gives each turn its share of the session's ledger; nil none.
	Usage SessionUsagePort
	Store SessionStorePort
	// Identity opens the session's KMP context (session_identity.go); nil
	// without KMP.
	Identity *MemoryIdentity
	// Host is how the console names itself in MADE's host reports
	// (host_report.go).
	Host HostIdentity
	// Judge is TypeSafe Jev, which decides what a turn settles
	// (session_settle.go); nil leaves it to the model and the reminder.
	Judge JudgementPort
	Now   func() time.Time
	// Log reports what the console did or could not do; nil says nothing.
	Log func(string)

	mu sync.Mutex
	// refusals are the report errors logged already: MADE 0.10.0 refuses
	// every report the same way, two lines per turn otherwise.
	refusalsMu sync.Mutex
	refusals   map[string]bool
	recalled   map[domain.SessionID]bool
}

// drives reports whether a session is one the console records for: a
// console session the person drives. Workers and the clones of repairs and
// improvements are sessions the console created for ceremonies that are
// MADE instances already.
func (c *SessionCeremony) drives(s domain.Session) bool {
	if c == nil || c.Store == nil {
		return false
	}
	return s.Mode() != domain.ModeTask && !s.Mode().ForgesPullRequest()
}

// records reports whether a session runs its own MADE instance: one the
// console drives, with MADE connected.
func (c *SessionCeremony) records(s domain.Session) bool {
	return c.drives(s) && c.Engine != nil
}

func (c *SessionCeremony) log(format string, args ...any) {
	if c != nil && c.Log != nil {
		c.Log(fmt.Sprintf(format, args...))
	}
}

func (c *SessionCeremony) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// instanceID names the session's instance, or its n-th successor.
func instanceID(id domain.SessionID, successor int) string {
	if successor <= 1 {
		return "axlr-session-" + string(id)
	}
	return fmt.Sprintf("axlr-session-%s-%d", id, successor)
}

// about is the session's memory about: the selected one, else the
// workspace's.
func (c *SessionCeremony) about(ctx context.Context, state domain.SessionState) string {
	about, _, err := memoryAbout(ctx, c.Labels, state)
	if err != nil || about == "" {
		return defaultAbout(state)
	}
	return about
}

// open starts the instance when the session has none, or tries again
// when an earlier start failed. successor > 1 opens the next instance
// after one whose turns ran out.
func (c *SessionCeremony) open(ctx context.Context, s *domain.Session, successor int) (domain.SessionInstance, bool) {
	state := s.Export()
	current, has := s.Instance()
	if has && current.Instance != "" && current.Unavailable == "" && !current.Closed && successor <= 1 {
		return current, true
	}
	if has && current.Instance != "" && current.Closed && successor <= 1 {
		// A closed instance (the session ended, or came from another
		// console with --import) continues in its successor: MADE refuses
		// to start an id again ("already exists", seen 13 October 2026).
		successor = successorNumber(current.Instance) + 1
	}
	if err := c.Engine.Ready(ctx, SessionDefinition, SessionDefinitionVersion); err != nil {
		c.unavailable(s, current, has, err)
		return domain.SessionInstance{}, false
	}
	id := instanceID(state.ID, successor)
	inputs := map[string]string{"workspace": string(state.Workspace), "model": string(state.Model), "about": c.about(ctx, state), "session": string(state.ID)}
	if successor > 1 {
		inputs["predecessor"] = current.Instance
	} else if current.Predecessor != "" {
		inputs["predecessor"] = instanceID(current.Predecessor, 1)
	}
	err := c.Engine.Start(ctx, SessionDefinition, SessionDefinitionVersion, id, inputs)
	for tries := 0; err != nil && successor > 1 && instanceExists(err) && tries < successorTries; tries++ {
		// The successor exists already (the same bundle imported twice on
		// one machine): the next number is tried.
		successor++
		id = instanceID(state.ID, successor)
		err = c.Engine.Start(ctx, SessionDefinition, SessionDefinitionVersion, id, inputs)
	}
	if err != nil {
		c.unavailable(s, current, has, err)
		return domain.SessionInstance{}, false
	}
	// The entries KMP has not taken yet travel to the successor (an
	// imported session's handoff among them); until 13 October 2026 they
	// were lost with the predecessor.
	instance := domain.SessionInstance{Instance: id, MemoryAgent: current.MemoryAgent, MemoryContext: current.MemoryContext, Recalled: current.Recalled, Predecessor: current.Predecessor, RecalledMemories: current.RecalledMemories, Pending: current.Pending}
	if err := s.SetInstance(instance); err != nil {
		c.log("session instance: %v", err)
		return domain.SessionInstance{}, false
	}
	c.log("session instance %s opened", id)
	return instance, true
}

// unavailable records why the instance could not be opened, said once.
func (c *SessionCeremony) unavailable(s *domain.Session, current domain.SessionInstance, has bool, err error) {
	reason := bounded(err.Error(), 300)
	if !has || current.Unavailable != reason {
		c.log("session instance unavailable: %s", reason)
	}
	current.Unavailable = reason
	_ = s.SetInstance(current)
}

// BeginTurn opens the session's KMP context at its first prompt and claims
// the turn step for the prompt the session just began, completing first
// the turn an interrupted console left claimed. It changes the session in
// place; the caller saves it.
func (c *SessionCeremony) BeginTurn(ctx context.Context, s *domain.Session) {
	if !c.drives(*s) {
		return
	}
	c.openContext(ctx, s)
	if c.Engine == nil {
		return
	}
	instance, ok := c.open(ctx, s, 1)
	if !ok {
		return
	}
	if instance.Fence != "" {
		instance = c.completeInterrupted(ctx, s, instance)
	}
	index := len(s.Messages()) - 1
	key := fmt.Sprintf("axlr:turn:%s:%d", s.ID(), index)
	fence, err := c.Engine.Claim(ctx, instance.Instance, sessionTurnStep, key, sessionLease)
	if err != nil && exhausted(err) {
		// The instance's thousand turns ran out: the next one continues it.
		successor := successorNumber(instance.Instance) + 1
		if next, opened := c.open(ctx, s, successor); opened {
			instance = next
			fence, err = c.Engine.Claim(ctx, instance.Instance, sessionTurnStep, key, sessionLease)
		}
	}
	if err != nil {
		c.log("session turn claim: %v", err)
		instance.Fence, instance.TurnIndex = "", index
		_ = s.SetInstance(instance)
		return
	}
	instance.TurnIndex = index
	instance.ClaimRequests, instance.ClaimCost = c.usage(ctx, s.ID())
	c.claimed(ctx, *s, &instance, fence, sessionLease)
	_ = s.SetInstance(instance)
}

// EndTurn completes the turn step with the turn's record and writes its
// results to KMP, then saves the session.
func (c *SessionCeremony) EndTurn(ctx context.Context, s *domain.Session) error {
	if !c.records(*s) {
		return nil
	}
	instance, has := s.Instance()
	if !has || instance.Instance == "" || instance.Closed {
		return nil
	}
	record := c.record(ctx, *s, instance, false)
	instance = c.complete(ctx, s, instance, record)
	if err := s.SetInstance(instance); err != nil {
		return err
	}
	return c.Store.Save(ctx, *s)
}

// record builds the turn's record with its share of the session's cost.
func (c *SessionCeremony) record(ctx context.Context, s domain.Session, instance domain.SessionInstance, interrupted bool) TurnRecord {
	var cost *TurnCost
	if requests, usd := c.usage(ctx, s.ID()); requests > 0 || usd > 0 {
		cost = &TurnCost{Requests: max(0, requests-instance.ClaimRequests), USD: max(0, usd-instance.ClaimCost)}
	}
	record := BuildTurnRecord(s, instance.TurnIndex, instance.Turns+1, c.about(ctx, s.Export()), cost)
	if interrupted {
		record.Interrupted = true
		record.AnswerKind = "paused"
	}
	return record
}

// complete hands the record to MADE, then to KMP. A MADE completion that
// fails keeps the fence, so the next claim completes it as interrupted.
func (c *SessionCeremony) complete(ctx context.Context, s *domain.Session, instance domain.SessionInstance, record TurnRecord) domain.SessionInstance {
	stepRef := fmt.Sprintf("ceremony %s step %s #%d", instance.Instance, sessionTurnStep, record.Turn)
	if instance.Fence != "" {
		// The finished report goes before the completion: MADE takes a
		// report only on an accepted lease, which the completion ends
		// (seen on 13 October 2026: "ceremony agent status step has no
		// accepted lease").
		c.report(ctx, *s, &instance, "finished", "turn")
		if err := c.Engine.Complete(ctx, instance.Instance, sessionTurnStep, instance.Fence, record.Output()); err != nil {
			c.log("session turn %d completion: %v", record.Turn, err)
			stepRef += " (not completed: " + bounded(err.Error(), 200) + ")"
		} else {
			instance.Fence, instance.OperationID = "", ""
		}
	} else {
		stepRef += " (not claimed)"
	}
	instance.Turns++
	instance.LastRecord = bounded(record.JSON(), domain.MaxSessionRecordBytes)
	if c.Journal != nil && record.HasResults() {
		state := s.Export()
		entry := TurnEntry{About: sessionAbout(state.ID), Key: fmt.Sprintf("axlr:turn:%s:%d", state.ID, record.PromptIndex), Summary: record.Summary(), Evidence: record.Evidence(stepRef), OccurredAt: record.FinishedAt,
			Labels: map[string][]string{"project": {record.About}, "ws": {string(state.Workspace)}, "session": {string(state.ID)}, "mode": {record.Mode}, "model": {record.Model}}}
		if entry.About == record.About {
			delete(entry.Labels, "project")
		}
		entry.ContextID = instance.MemoryContext
		instance.Pending = c.journal(ctx, append(instance.Pending, encodeEntry(entry)))
		if !record.Interrupted {
			c.settle(ctx, *s, instance, record, stepRef)
		}
	} else if len(instance.Pending) > 0 {
		instance.Pending = c.journal(ctx, instance.Pending)
	}
	return instance
}

// journal writes the queued entries to KMP, oldest first, and returns what
// KMP did not take: the engine down or a refusal keeps the entry for the
// next turn end or the close (decision E10), bounded by MaxPendingEntries.
func (c *SessionCeremony) journal(ctx context.Context, pending []string) []string {
	if c.Journal == nil {
		return pending
	}
	var kept []string
	for i, encoded := range pending {
		entry, err := decodeEntry(encoded)
		if err != nil {
			c.log("session memory entry dropped: %v", err)
			continue
		}
		if _, err := c.Journal.RecordTurn(ctx, entry); errors.Is(err, ErrEntryRecorded) {
			// The store has it from an earlier attempt (the entry was
			// queued after a write whose receipt was lost): retrying
			// would block every entry behind it.
			c.log("session memory %s: recorded already; dropped", entry.Key)
		} else if err != nil {
			c.log("session memory %s: %v; kept for the next turn", entry.Key, err)
			kept = append(kept, pending[i:]...)
			break
		}
	}
	for len(kept) > domain.MaxPendingEntries {
		c.log("session memory entry dropped: %d pending", len(kept))
		kept = kept[1:]
	}
	return kept
}

func encodeEntry(entry TurnEntry) string {
	data, _ := json.Marshal(entry)
	return string(data)
}

func decodeEntry(encoded string) (TurnEntry, error) {
	var entry TurnEntry
	if err := json.Unmarshal([]byte(encoded), &entry); err != nil {
		return TurnEntry{}, err
	}
	if entry.About == "" || entry.Key == "" {
		return TurnEntry{}, fmt.Errorf("entry without about or key")
	}
	return entry, nil
}

// Leave closes the instance of a session the console switches away from
// (/new, the picker): its turns are over for this console.
func (c *SessionCeremony) Leave(ctx context.Context, s *domain.Session) error {
	return c.Close(ctx, s, "left")
}

// Abandon ends the instance of a session no console holds: an open turn is
// completed as interrupted, a turn carries abandoned=true, and the
// abandoned transition reaches ABANDONED. The turn carries closed=true as
// well: the step's repeat ends on that field, and MADE enables no
// transition out of OPEN while the repeat runs (seen on 13 October 2026:
// "ceremony transition guards are not satisfied").
func (c *SessionCeremony) Abandon(ctx context.Context, s *domain.Session) error {
	return c.end(ctx, s, "abandoned", sessionAbandonedTrigger, map[string]any{"closed": true, "abandoned": true})
}

// SweepAbandoned ends, as abandoned, the open instances of this workspace's
// sessions that no console holds: a session another console has open
// cannot be loaded (its lock is held) and is left alone; the current one
// is skipped. It runs at console start, before heartbeats exist to tell
// liveness by report. It returns how many it ended.
func (c *SessionCeremony) SweepAbandoned(ctx context.Context, store SessionLister, workspace domain.Workspace, current domain.SessionID) (int, error) {
	if c == nil || c.Engine == nil || store == nil {
		return 0, nil
	}
	summaries, err := store.List(ctx)
	if err != nil {
		return 0, err
	}
	ended := 0
	for _, summary := range summaries {
		if summary.Workspace != workspace || summary.ID == current {
			continue
		}
		session, err := store.Load(ctx, summary.ID)
		if err != nil {
			continue // held by a live console, or unreadable: not ours to end
		}
		instance, has := session.Instance()
		if has && instance.Instance != "" && !instance.Closed {
			if err := c.Abandon(ctx, &session); err != nil {
				c.log("session %s abandon: %v", summary.ID, err)
			} else if closed, _ := session.Instance(); closed.Closed {
				ended++
			}
		}
		_ = store.Release(summary.ID)
	}
	return ended, nil
}

// completeInterrupted completes the turn a claim was left on: the console
// stopped, or a completion failed, before the turn's record reached MADE.
func (c *SessionCeremony) completeInterrupted(ctx context.Context, s *domain.Session, instance domain.SessionInstance) domain.SessionInstance {
	c.handoff(ctx, instance)
	record := c.record(ctx, *s, instance, true)
	return c.complete(ctx, s, instance, record)
}

// Close ends the instance: an open turn is completed as interrupted, a
// closing turn carries the summary, and the closed transition reaches DONE.
// Nothing here stops the console from exiting; failures are logged.
func (c *SessionCeremony) Close(ctx context.Context, s *domain.Session, reason string) error {
	return c.end(ctx, s, reason, sessionClosedTrigger, map[string]any{"closed": true})
}

// end completes an open turn, claims one more turn with the ending fields
// and the summary, applies the trigger and saves the session.
func (c *SessionCeremony) end(ctx context.Context, s *domain.Session, reason, trigger string, fields map[string]any) error {
	if !c.records(*s) {
		return nil
	}
	instance, has := s.Instance()
	if !has || instance.Instance == "" || instance.Closed {
		return nil
	}
	if instance.Fence != "" {
		instance = c.completeInterrupted(ctx, s, instance)
	}
	if len(instance.Pending) > 0 {
		instance.Pending = c.journal(ctx, instance.Pending)
	}
	output := map[string]any{"summary": map[string]any{"turns": instance.Turns, "reason": reason, "ended_at": c.now().UTC().Format(time.RFC3339)}}
	for key, value := range fields {
		output[key] = value
	}
	// The key carries the attempt's time: an ending that failed after its
	// claim (the transition refused) leaves a used key behind, and MADE
	// refuses a claim that reuses it (seen on 13 October 2026).
	key := fmt.Sprintf("axlr:%s:%s:%d:%d", trigger, s.ID(), instance.Turns, c.now().UTC().Unix())
	fence, err := c.Engine.Claim(ctx, instance.Instance, sessionTurnStep, key, sessionLease)
	if err == nil {
		err = c.Engine.Complete(ctx, instance.Instance, sessionTurnStep, fence, output)
	}
	if err == nil {
		_, err = c.Engine.Transition(ctx, instance.Instance, trigger)
	}
	if err != nil {
		c.log("session instance %s %s: %v", instance.Instance, trigger, err)
	} else {
		instance.Closed = true
		c.log("session instance %s %s after %s", instance.Instance, trigger, plural(instance.Turns, "turn"))
		if c.Journal != nil {
			// The close leaves one entry under the session's about and
			// nothing in the project's (decision F3).
			state := s.Export()
			// KMP's kinds have no session_closed (refused on 13 October
			// 2026): the entry is an observation labelled event=closed.
			entry := TurnEntry{Kind: "observation", About: sessionAbout(state.ID), Key: fmt.Sprintf("axlr:%s:%s", trigger, instance.Instance), Summary: fmt.Sprintf("Session %s after %s (%s).", trigger, plural(instance.Turns, "turn"), reason),
				Evidence: []string{fmt.Sprintf("ceremony %s %s", instance.Instance, trigger)}, OccurredAt: c.now(), ContextID: instance.MemoryContext,
				Labels: map[string][]string{"ws": {string(state.Workspace)}, "session": {string(state.ID)}, "mode": {string(s.Mode())}, "event": {trigger}}}
			instance.Pending = c.journal(ctx, append(instance.Pending, encodeEntry(entry)))
		}
	}
	if err := s.SetInstance(instance); err != nil {
		return err
	}
	return c.Store.Save(ctx, *s)
}

func (c *SessionCeremony) usage(ctx context.Context, id domain.SessionID) (int, float64) {
	if c == nil || c.Usage == nil {
		return 0, 0
	}
	ledger, err := c.Usage.LoadUsage(ctx, id)
	if err != nil {
		return 0, 0
	}
	return ledger.Requests, ledger.Cost
}

// successorTries bounds how many taken successor numbers open skips.
const successorTries = 20

// instanceExists reports a start MADE refused because the id is taken.
func instanceExists(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}

// exhausted reports a claim MADE refused because the step's repetitions
// ran out.
func exhausted(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "exhaust") || strings.Contains(text, "max_iterations") || strings.Contains(text, "repeat")
}

// successorNumber reads the suffix of an instance id, 1 without one.
func successorNumber(instance string) int {
	parts := strings.Split(instance, "-")
	if len(parts) < 4 {
		return 1
	}
	n := 0
	for _, r := range parts[len(parts)-1] {
		if r < '0' || r > '9' {
			return 1
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		return 1
	}
	return n
}
