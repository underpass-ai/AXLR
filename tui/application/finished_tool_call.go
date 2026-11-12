package application

import (
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// finishTool tells the tool-call log how the call ended, then emits its
// activity. started is when the decision began; the zero time is a call
// the console refused before it ran.
func finishTool(s *domain.Session, id root.ToolCallID, started time.Time, log ToolCallLogPort, emit func(Event) error) error {
	logFinishedTool(log, s, id, started)
	return emitTool(s, id, emit)
}

// logFinishedTool records the call's outcome as the session holds it; a
// call without one yet records nothing.
func logFinishedTool(log ToolCallLogPort, s *domain.Session, id root.ToolCallID, started time.Time) {
	if log == nil || s == nil {
		return
	}
	state := s.Export()
	for _, activity := range state.Activity {
		if activity.Call.ID != id || activity.Outcome == nil {
			continue
		}
		record := ToolCallRecord{Session: state.ID, Call: id, Tool: activity.Call.Name, PluginTool: CallPluginTool(s.ToolSnapshot(), activity.Call), Decision: activity.Decision, Failed: activity.Outcome.IsError}
		if record.Failed {
			record.Error = ToolCallErrorText(string(activity.Outcome.Content), MaxToolCallErrorBytes)
		}
		if !started.IsZero() {
			record.Duration = time.Since(started)
		}
		log.ToolCallFinished(record)
		return
	}
}
