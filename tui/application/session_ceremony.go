package application

import (
	"context"
	"fmt"
	"strings"
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

// TurnEntry is a turn's results as the console writes them to KMP under
// the session's about: the summary and its evidence, one line per action,
// never the prompt or the answer (decision H8).
type TurnEntry struct {
	About      string
	Labels     map[string][]string
	Key        string
	Summary    string
	Evidence   []string
	OccurredAt time.Time
}

// JournalPort writes a turn's results to KMP and returns the stored ref.
type JournalPort interface {
	RecordTurn(ctx context.Context, entry TurnEntry) (string, error)
}

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
	Now   func() time.Time
	// Log reports what the console did or could not do; nil says nothing.
	Log func(string)
}

// records reports whether a session runs its own instance: a console
// session the person drives. Workers and the clones of repairs and
// improvements are sessions the console created for ceremonies that are
// MADE instances already.
func (c *SessionCeremony) records(s domain.Session) bool {
	if c == nil || c.Engine == nil || c.Store == nil {
		return false
	}
	return s.Mode() != domain.ModeTask && !s.Mode().ForgesPullRequest()
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
	if err := c.Engine.Ready(ctx, SessionDefinition, SessionDefinitionVersion); err != nil {
		c.unavailable(s, current, has, err)
		return domain.SessionInstance{}, false
	}
	id := instanceID(state.ID, successor)
	inputs := map[string]string{"workspace": string(state.Workspace), "model": string(state.Model), "about": c.about(ctx, state), "session": string(state.ID)}
	if successor > 1 {
		inputs["predecessor"] = current.Instance
	}
	if err := c.Engine.Start(ctx, SessionDefinition, SessionDefinitionVersion, id, inputs); err != nil {
		c.unavailable(s, current, has, err)
		return domain.SessionInstance{}, false
	}
	instance := domain.SessionInstance{Instance: id}
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

// BeginTurn claims the turn step for the prompt the session just began,
// completing first the turn an interrupted console left claimed. It
// changes the session in place; the caller saves it.
func (c *SessionCeremony) BeginTurn(ctx context.Context, s *domain.Session) {
	if !c.records(*s) {
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
	fence, err := c.Engine.Claim(ctx, instance.Instance, sessionTurnStep, key, 0)
	if err != nil && exhausted(err) {
		// The instance's thousand turns ran out: the next one continues it.
		successor := successorNumber(instance.Instance) + 1
		if next, opened := c.open(ctx, s, successor); opened {
			instance = next
			fence, err = c.Engine.Claim(ctx, instance.Instance, sessionTurnStep, key, 0)
		}
	}
	if err != nil {
		c.log("session turn claim: %v", err)
		instance.Fence, instance.TurnIndex = "", index
		_ = s.SetInstance(instance)
		return
	}
	instance.Fence, instance.TurnIndex = fence, index
	instance.ClaimRequests, instance.ClaimCost = c.usage(ctx, s.ID())
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
	stepRef := fmt.Sprintf("MADE %s step %s #%d", instance.Instance, sessionTurnStep, record.Turn)
	if instance.Fence != "" {
		if err := c.Engine.Complete(ctx, instance.Instance, sessionTurnStep, instance.Fence, record.Output()); err != nil {
			c.log("session turn %d completion: %v", record.Turn, err)
			stepRef += " (not completed: " + bounded(err.Error(), 200) + ")"
		} else {
			instance.Fence = ""
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
		if _, err := c.Journal.RecordTurn(ctx, entry); err != nil {
			c.log("session turn %d memory: %v", record.Turn, err)
		}
	}
	return instance
}

// completeInterrupted completes the turn a claim was left on: the console
// stopped, or a completion failed, before the turn's record reached MADE.
func (c *SessionCeremony) completeInterrupted(ctx context.Context, s *domain.Session, instance domain.SessionInstance) domain.SessionInstance {
	record := c.record(ctx, *s, instance, true)
	return c.complete(ctx, s, instance, record)
}

// Close ends the instance: an open turn is completed as interrupted, a
// closing turn carries the summary, and the closed transition reaches DONE.
// Nothing here stops the console from exiting; failures are logged.
func (c *SessionCeremony) Close(ctx context.Context, s *domain.Session, reason string) error {
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
	summary := map[string]any{"turns": instance.Turns, "reason": reason, "closed_at": c.now().UTC().Format(time.RFC3339)}
	key := fmt.Sprintf("axlr:close:%s:%d", s.ID(), instance.Turns)
	fence, err := c.Engine.Claim(ctx, instance.Instance, sessionTurnStep, key, 0)
	if err == nil {
		err = c.Engine.Complete(ctx, instance.Instance, sessionTurnStep, fence, map[string]any{"closed": true, "summary": summary})
	}
	if err == nil {
		_, err = c.Engine.Transition(ctx, instance.Instance, sessionClosedTrigger)
	}
	if err != nil {
		c.log("session instance %s close: %v", instance.Instance, err)
	} else {
		instance.Closed = true
		c.log("session instance %s closed after %s", instance.Instance, plural(instance.Turns, "turn"))
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
