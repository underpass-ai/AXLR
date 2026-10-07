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

// footerView is the main view's last row: clickable key hints on the left
// and the session state on the right. An error takes the whole row so its
// text is never truncated behind the hints.
func (m AppModel) footerView() string {
	width := max(1, m.Layout.Width)
	if m.Status.Error != "" {
		return m.errorRow()
	}
	// Hints are in priority order; the narrowest terminals keep the first.
	hints := []footerHint{{"send", "enter", m.Theme.T("footer.send")}}
	if m.Busy {
		hints = append(hints, footerHint{"cancel", "esc", m.Theme.T("footer.cancel")})
	}
	if m.Header.State.Status == domain.StatusInterrupted || m.Header.State.Status == domain.StatusStreaming {
		hints = append(hints, footerHint{"continue", "ctrl+r", m.Theme.T("footer.continue")})
	}
	if count := len(m.Changes.records); count > 0 {
		hints = append(hints, footerHint{"changes", "ctrl+d", m.Theme.Tf("footer.changes", count)})
	}
	hints = append(hints, footerHint{"palette", "ctrl+p", m.Theme.T("footer.actions")}, footerHint{"help", "f1", m.Theme.T("footer.help")})
	if m.inlineApproval() {
		// The approval card lists the only keys that work until it is decided.
		hints = nil
	}
	right := m.footerStatus()
	room := width - 2 - ansi.StringWidth(right) - 2
	var left []string
	used := 0
	for _, h := range hints {
		plain := h.key + " " + h.label
		if used+ansi.StringWidth(plain)+2 > room && len(left) > 0 {
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

// footerStatus is the right side: activity first, then the session state and
// tokens used.
func (m AppModel) footerStatus() string {
	status := m.Status
	status.Waiting, status.Executing, status.ToolName = m.providerWaiting, m.toolExecuting, m.toolName
	parts := []string{}
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
			badge = m.Theme.Tf("ceremony.badge", badge, run.Step, run.Iteration)
			if run.AwaitingPerson() {
				badge = m.Theme.Tf("incident.badgeAwaiting", m.Theme.T("mode."+string(mode)))
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
	if m.tokens > 0 {
		parts = append(parts, m.Theme.Tf("footer.tokens", formatTokens(m.tokens)))
	}
	text := strings.Join(parts, " · ")
	if m.Theme.Monochrome {
		return text
	}
	return m.Theme.Muted(text)
}

func (m AppModel) statusActivity(status StatusBar) string {
	activity := m.activityLabel(status)
	if activity != "" && m.steerPrompt != "" {
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
		}
		return indicator + " " + label + " · " + fmt.Sprintf("%ds", int(time.Since(m.providerWaitStarted).Seconds()))
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

// errorRow gives an error the whole row, in the footer and under overlays.
func (m AppModel) errorRow() string {
	width := max(1, m.Layout.Width)
	text := " " + m.Theme.Icon("error") + " " + singleLine(m.Status.Error)
	if !m.Theme.Monochrome {
		text = lipgloss.NewStyle().Foreground(lipgloss.Color(m.Theme.palette().Warning)).Render(text)
	}
	return lipgloss.NewStyle().Width(width).Render(ansi.Truncate(text, width, "…"))
}
