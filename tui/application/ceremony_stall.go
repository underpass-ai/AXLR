package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// toolCallMarkers are tool-call templates a server's parser should have
// turned into tool_calls. Text containing one means the call leaked: seen on
// 7 Oct 2026 with Gemma 4 on vLLM (<|tool_call>call:local_write{…}).
var toolCallMarkers = []string{"<|tool_call>", "<tool_call>", "[TOOL_CALLS]", "<|python_tag|>", "<function=", "<|start|>assistant<|channel|>commentary to="}

// leakedToolCall returns the marker found in the session's last answer.
func leakedToolCall(s domain.Session) string {
	messages := s.Messages()
	if len(messages) == 0 {
		return ""
	}
	last := messages[len(messages)-1]
	if last.Role != root.RoleAssistant || len(last.ToolCalls) > 0 {
		return ""
	}
	for _, marker := range toolCallMarkers {
		if strings.Contains(string(last.Content), marker) {
			return marker
		}
	}
	return ""
}

// leakNote is added to the reminder when the answer held a leaked call.
func leakNote(s domain.Session) string {
	if marker := leakedToolCall(s); marker != "" {
		return fmt.Sprintf(" Your last answer wrote a tool call as text (%s); call the tool through the tool interface instead.", marker)
	}
	return ""
}

// StalledReason says why the session's ceremony cannot progress on its own:
// the model ended its turn again after the console's one reminder, so
// nothing would ever run the open step. A step waiting for the person is
// never stalled.
func StalledReason(s domain.Session) string {
	run, live := s.Ceremony()
	if !live || run.AwaitingPerson() || !run.Reminded || s.Status() != domain.StatusComplete {
		return ""
	}
	reason := fmt.Sprintf("the model ended its turn twice with step %s open and did not hand it back", run.Step)
	if marker := leakedToolCall(s); marker != "" {
		reason += fmt.Sprintf("; its answer holds a tool call as text (%s), so the server's tool-call parser is the first suspect", marker)
	}
	return reason
}

// StopCeremony cancels the live ceremony in MADE with the reason, records the
// outcome and returns the session to normal mode. It changes nothing else.
func (d *CeremonyDriver) StopCeremony(ctx context.Context, s *domain.Session, reason string) error {
	run, live := s.Ceremony()
	if !live {
		return errors.New("no ceremony is running in this session")
	}
	if d == nil || d.Engine == nil {
		return errors.New("MADE is not connected; the ceremony cannot be stopped")
	}
	if err := d.Engine.Cancel(ctx, run.Instance, "AXLR console: "+reason); err != nil {
		return fmt.Errorf("cancel %s: %w", run.Instance, err)
	}
	output := map[string]any{"observed": reason}
	switch {
	case run.Plan != nil:
		d.finishPlan(ctx, *s, run, "CANCELLED")
	case run.Task != nil:
		d.finishTask(ctx, *s, run, "CANCELLED", output)
	default:
		d.record(ctx, *s, run, "CANCELLED", output)
	}
	d.observe(run, "CANCELLED", map[string]any{"reason": reason}, true, false)
	s.FinishCeremony()
	return nil
}

// cancelStalled ends a stalled ceremony instead of leaving the session
// waiting forever, and starts one console turn so the reason is visible in
// the transcript. Self-repair and self-improvement keep their own nudges and
// decide themselves.
func cancelStalled(ctx context.Context, s *domain.Session, u ContinueTurnUseCase, emit func(Event) error) (bool, error) {
	reason := StalledReason(*s)
	if reason == "" || s.Mode().ForgesPullRequest() || u.Ceremonies == nil {
		return false, nil
	}
	run, _ := s.Ceremony()
	next := *s
	if err := u.Ceremonies.StopCeremony(ctx, &next, reason); err != nil {
		return false, err
	}
	if u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticCeremonyStalled})
	}
	note := root.Text(fmt.Sprintf("[AXLR] The console cancelled ceremony %s %s at step %s: %s. The session is back in normal mode; nothing was rolled back. Tell the user in their language what was done and what is left.", run.Definition, run.Version, run.Step, reason))
	if s.Mode() == domain.ModeTask {
		// A plan worker is ended by its runner; no further turn.
		if err := u.Store.Save(ctx, next); err != nil {
			return false, err
		}
		*s = next
		return false, emitSession(s, emit)
	}
	if err := next.BeginTurn(note, next.ToolSnapshot()); err != nil {
		return false, err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return false, err
	}
	*s = next
	return true, emitSession(s, emit)
}

// StopCeremony is the person's /stop-ceremony: the live ceremony is
// cancelled in MADE and the session returns to normal mode.
func (u StartTurnUseCase) StopCeremony(ctx context.Context, session *domain.Session, emit func(Event) error) error {
	if session == nil || u.Store == nil || u.Continue.Ceremonies == nil {
		return errors.New("stopping needs a session and a connected ceremony driver")
	}
	next := *session
	if err := u.Continue.Ceremonies.StopCeremony(ctx, &next, "stopped by the person"); err != nil {
		return err
	}
	if u.Continue.Diagnostics != nil {
		_ = u.Continue.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticCeremonyStalled})
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*session = next
	return emitSession(session, emit)
}

// OpenStepIdle reports a ceremony whose step is open while no turn runs: a
// session restored after the console stopped, or one the person paused.
func OpenStepIdle(s domain.Session) (domain.CeremonyRun, bool) {
	run, live := s.Ceremony()
	if !live || run.AwaitingPerson() {
		return domain.CeremonyRun{}, false
	}
	switch s.Status() {
	case domain.StatusComplete, domain.StatusIdle, domain.StatusInterrupted:
		return run, true
	}
	return domain.CeremonyRun{}, false
}
