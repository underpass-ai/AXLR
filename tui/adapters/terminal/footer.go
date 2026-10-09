package terminal

import (
	"fmt"
	"github.com/underpass-ai/AXLR/tui/application"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type footerHint struct{ zone, key, label string }

// maxErrorRows bounds the rows a long error takes from the conversation.
const maxErrorRows = 3

// footerView is the main view's last row: clickable key hints on the left
// and the session state on the right. An error takes the whole row, and up
// to maxErrorRows rows when it is longer, so its text is not truncated
// behind the hints. While an operation runs the error sits above the hint
// row instead, so the activity and its cancel hint stay in view.
func (m AppModel) footerView() string {
	if m.Status.Error != "" {
		if m.Busy {
			return strings.Join(append(m.errorRows(maxErrorRows-1), m.hintRow()), "\n")
		}
		return strings.Join(m.errorRows(maxErrorRows), "\n")
	}
	return m.hintRow()
}

// hintRow is the footer's row of key hints and session state.
func (m AppModel) hintRow() string {
	width := max(1, m.Layout.Width)
	// Hints are in priority order; the narrowest terminals keep the first.
	hints := []footerHint{{"send", "enter", m.Theme.T("footer.send")}}
	if m.Busy {
		hints = append(hints, footerHint{"cancel", "esc", m.Theme.T("footer.cancel")})
	}
	if !m.Busy && (m.Header.State.Status == domain.StatusInterrupted || m.Header.State.Status == domain.StatusStreaming) {
		// Ctrl+R is refused while an operation runs.
		hints = append(hints, footerHint{"continue", "ctrl+r", m.Theme.T("footer.continue")})
	}
	if count := len(m.Changes.files); count > 0 {
		// Files, as /changes lists them: on 10 October 2026 the hint read
		// "4 changes" for four edits to two files.
		label := m.Theme.Tf("changes.files", count)
		if count == 1 {
			label = m.Theme.T("changes.oneFile")
		}
		hints = append(hints, footerHint{"changes", "ctrl+d", label})
	}
	hints = append(hints, footerHint{"palette", "ctrl+p", m.Theme.T("footer.actions")}, footerHint{"help", "f1", m.Theme.T("footer.help")})
	if m.inlineApproval() {
		// The approval card lists the only keys that work until it is decided.
		hints = nil
	}
	// On 10 October 2026, at 80 columns, the row kept "enter send" and cut
	// the session cost ("… $0.0057 turn · $…"). The state and the session
	// cost come first: hints take only the room left, and the status drops
	// its token and cache details, then the turn's cost, before it is cut.
	right := m.footerStatus(statusFull)
	for detail := statusFull + 1; detail <= statusLeast && ansi.StringWidth(right)+3 > width; detail++ {
		right = m.footerStatus(detail)
	}
	room := width - 2 - ansi.StringWidth(right) - 2
	var left []string
	used := 0
	for _, h := range hints {
		plain := h.key + " " + h.label
		if used+ansi.StringWidth(plain)+2 > room {
			break
		}
		left = append(left, m.zones.Mark(m.prefix+h.zone, m.footerKey(h.key)+m.Theme.Muted(" "+h.label)))
		used += ansi.StringWidth(plain) + 2
	}
	line := " " + strings.Join(left, "  ")
	if len(left) == 0 {
		line = ""
	}
	gap := max(1, width-ansi.StringWidth(line)-ansi.StringWidth(right)-1)
	return lipgloss.NewStyle().Width(width).Render(ansi.Truncate(line+strings.Repeat(" ", gap)+right, width, "…"))
}

func (m AppModel) footerKey(key string) string {
	if m.Theme.Monochrome {
		return key
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(m.Theme.palette().Text)).Render(key)
}

// Footer status detail levels, from everything to the state and the
// session's cost.
const (
	statusFull = iota
	statusNoTokens
	statusLeast
)

// footerStatus is the right side: activity first, then the session state and
// tokens used. A higher detail level leaves out what matters least.
func (m AppModel) footerStatus(detail int) string {
	status := m.Status
	status.Waiting, status.Executing, status.ToolName = m.providerWaiting, m.toolExecuting, m.toolName
	status.PreparingTool, status.PreparingBytes = m.toolCallName, m.toolCallBytes
	parts := []string{}
	if status.Notice != "" {
		parts = append(parts, status.Notice)
	}
	if activity := m.statusActivity(status); activity != "" {
		parts = append(parts, activity)
	} else {
		state := status.State
		if state == "" {
			state = domain.StatusIdle
		}
		parts = append(parts, m.Theme.T("status."+string(state)))
	}
	if mode := m.Header.State.Mode; mode != "" && mode != domain.ModeNormal {
		badge := m.Theme.T("mode." + string(mode))
		if run := m.Header.State.Ceremony; run != nil {
			if limit := application.AttemptLimit(*run); limit > 0 {
				badge = m.Theme.Tf("ceremony.badgeBounded", badge, run.Step, run.Iteration, limit)
			} else {
				badge = m.Theme.Tf("ceremony.badge", badge, run.Step, run.Iteration)
			}
			if run.AwaitingPerson() {
				badge = m.Theme.Tf("incident.badgeAwaiting", m.Theme.T("mode."+string(mode)))
			} else if m.deps.Session != nil && !m.Busy {
				if open, idle := application.OpenStepIdle(*m.deps.Session); idle {
					badge = m.Theme.Tf("ceremony.badgeOpen", badge, open.Step)
				}
			}
		}
		parts = append(parts, badge)
	}
	if badge := m.planBadge(); badge != "" {
		parts = append(parts, badge)
	}
	if badge := m.repairBadge(); badge != "" {
		parts = append(parts, badge)
	}
	if status.Autonomous {
		parts = append(parts, m.Theme.T("status.autonomous"))
	}
	if m.tokens > 0 && detail < statusNoTokens {
		parts = append(parts, m.Theme.Tf("footer.tokens", formatTokens(m.tokens)))
		if m.cached > 0 {
			parts = append(parts, m.Theme.Tf("footer.cache", m.cached))
		}
	}
	costs := m.costParts()
	if detail >= statusLeast && len(costs) > 1 {
		// The turn's cost goes; the session's stays.
		costs = costs[1:]
	}
	parts = append(parts, costs...)
	text := strings.Join(parts, " · ")
	if m.Theme.Monochrome {
		return text
	}
	return m.Theme.Muted(text)
}

func (m AppModel) statusActivity(status StatusBar) string {
	activity := m.activityLabel(status)
	if activity != "" && m.steer.Peek() != "" {
		activity += " · " + m.Theme.T("status.queued")
	}
	return activity
}

func (m AppModel) activityLabel(status StatusBar) string {
	indicator := m.Theme.Icon("waiting")
	if !m.UIPreferences.ReduceMotion {
		indicator = m.activitySpinner.View()
	}
	switch {
	case m.memoryActive:
		return indicator + " " + m.Theme.T("app.memoryRunning")
	case status.Executing:
		name := status.ToolName
		if name == "" {
			name = m.Theme.T("common.tool")
		}
		if run := m.Header.State.Ceremony; run != nil && run.Step == "revise" && name == string(application.HostStepDoneName) {
			return indicator + " " + m.Theme.Tf("incident.reviewing", int(time.Since(m.toolStarted).Seconds()))
		}
		return indicator + " " + m.Theme.Tf("status.executing", name, int(time.Since(m.toolStarted).Seconds()))
	case status.Waiting:
		label := m.Theme.T("status.waiting")
		switch status.Phase {
		case domain.ProviderReasoning:
			label = m.Theme.T("status.reasoning")
		case domain.ProviderToolCall:
			label = m.Theme.T("status.preparingTools")
			if status.PreparingTool != "" {
				label = m.Theme.Tf("status.preparingTool", status.PreparingTool, formatBytes(status.PreparingBytes))
			}
		}
		return indicator + " " + label + " · " + fmt.Sprintf("%ds", int(time.Since(m.providerWaitStarted).Seconds()))
	case m.Busy && !m.turnRunning():
		// An update, MADE preparation or a list runs: say so instead of
		// "idle". No clock ticks for it, so the indicator is the still one.
		return m.Theme.Icon("waiting") + " " + m.Theme.T("status.working")
	}
	return ""
}

// formatTokens shortens a token count: 980, 12.4k, 1.2M.
func formatTokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}

// footerRows is the main view's footer height: one row, or the rows a long
// error wraps to. The conversation gives up the rows past the first.
func (m AppModel) footerRows() int {
	if m.Status.Error == "" || m.overlay == "search" && !m.inlineApproval() {
		return 1 // search draws its own one-row footer
	}
	return lipgloss.Height(m.footerView())
}

// errorRow gives an error the whole row under overlays.
func (m AppModel) errorRow() string {
	return m.errorRows(1)[0]
}

// errorRows wraps an error over at most limit full-width rows, aligned
// after its icon; the last row is cut with "…" when even those are short.
func (m AppModel) errorRows(limit int) []string {
	width := max(1, m.Layout.Width)
	lead := " " + m.Theme.Icon("error") + " "
	indent := ansi.StringWidth(lead)
	room := max(1, width-indent)
	lines := strings.Split(ansi.Wrap(singleLine(m.Status.Error), room, ""), "\n")
	if limit = max(1, limit); len(lines) > limit {
		lines = append(lines[:limit-1], ansi.Truncate(strings.Join(lines[limit-1:], " "), room-1, "")+"…")
	}
	rows := make([]string, len(lines))
	for i, line := range lines {
		text := lead + strings.TrimRight(line, " ")
		if i > 0 {
			text = strings.Repeat(" ", indent) + strings.TrimRight(line, " ")
		}
		if !m.Theme.Monochrome {
			text = lipgloss.NewStyle().Foreground(lipgloss.Color(m.Theme.palette().Warning)).Render(text)
		}
		rows[i] = lipgloss.NewStyle().Width(width).Render(ansi.Truncate(text, width, "…"))
	}
	return rows
}
