package terminal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// openSessionPanel shows what the console recorded for this session: its
// MADE instance, its turns and the last turn's record, action by action
// (decision J6 of the lifecycle integration design).
func (m AppModel) openSessionPanel() AppModel {
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(sessionPanelContent(m.Header.State, m.Theme))
	m.Info.Viewport.GotoTop()
	m.overlay = "session"
	return m
}

// sessionPanelContent renders the session's instance and last turn record.
func sessionPanelContent(state domain.SessionState, theme Theme) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", theme.T("session.id"), state.ID)
	fmt.Fprintf(&b, "%s ws:%s\n", theme.T("session.about"), state.ID)
	instance := state.Instance
	if instance != nil && instance.KMPContext != "" {
		fmt.Fprintf(&b, "%s %s\n", theme.T("session.context"), instance.KMPContext)
	}
	switch {
	case instance == nil:
		b.WriteString(theme.T("session.none"))
		return b.String()
	case instance.Instance == "" && instance.Unavailable == "":
		b.WriteString(theme.T("session.none"))
		return b.String()
	case instance.Instance == "":
		fmt.Fprintf(&b, "%s %s", theme.T("session.unavailable"), instance.Unavailable)
		return b.String()
	}
	status := theme.T("session.open")
	if instance.Closed {
		status = theme.T("session.closedState")
	}
	fmt.Fprintf(&b, "%s %s · %s · %s\n", theme.T("session.instance"), instance.Instance, status, theme.Tn("session.turns", instance.Turns, instance.Turns))
	if instance.Fence != "" {
		fmt.Fprintf(&b, "%s\n", theme.T("session.turnOpen"))
	}
	if n := len(instance.Pending); n > 0 {
		fmt.Fprintf(&b, "%s\n", theme.Tn("session.pending", n, n))
	}
	var record application.TurnRecord
	if instance.LastRecord == "" || json.Unmarshal([]byte(instance.LastRecord), &record) != nil {
		b.WriteString("\n" + theme.T("session.noRecord"))
		return b.String()
	}
	fmt.Fprintf(&b, "\n%s\n", theme.Tf("session.lastTurn", record.Turn))
	fmt.Fprintf(&b, "  %s %s\n", theme.T("session.prompt"), singleLine(record.Prompt))
	if record.Answer != "" {
		fmt.Fprintf(&b, "  %s %s (%s)\n", theme.T("session.answer"), singleLine(record.Answer), record.AnswerKind)
	}
	if record.Cost != nil {
		fmt.Fprintf(&b, "  %s %d · %s\n", theme.T("session.cost"), record.Cost.Requests, FormatUSD(record.Cost.USD))
	}
	fmt.Fprintf(&b, "  %s %d\n", theme.T("session.reads"), record.Reads)
	if len(record.Memories) > 0 {
		for _, memory := range record.Memories {
			fmt.Fprintf(&b, "  %s %s (%s)\n", theme.T("session.memory"), memory.About, memory.Key)
		}
	}
	if len(record.Actions) == 0 {
		fmt.Fprintf(&b, "  %s\n", theme.T("session.noActions"))
	}
	for _, action := range record.Actions {
		line := fmt.Sprintf("  %s %s", action.Tool, action.Target)
		if action.ExitCode != nil {
			line += fmt.Sprintf(" → exit %d", *action.ExitCode)
		}
		b.WriteString(singleLine(line) + "\n")
		if action.Why != "" {
			fmt.Fprintf(&b, "    %s\n", singleLine(theme.Tf("approval.why", action.Why)))
		}
		outcome := action.Decision + " · " + action.Outcome
		if action.Reason != "" {
			outcome += " (" + action.Reason + ")"
		}
		fmt.Fprintf(&b, "    %s\n", singleLine(outcome))
	}
	return b.String()
}
