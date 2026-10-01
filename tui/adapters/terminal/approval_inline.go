package terminal

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// approvalCardChrome is the card's fixed rows: rule, title and actions.
const approvalCardChrome = 3

// approvalDetailRows is how many argument rows the inline card shows: all of
// them when they fit, otherwise a third of the screen (3–10 rows) that the
// arrows and wheel scroll.
func (m AppModel) approvalDetailRows() int {
	limit := min(10, max(3, (m.Layout.Height-approvalCardChrome-2)/3))
	return max(1, min(m.Approval.Details.VisualLineCount(), limit))
}

// approvalCard replaces the composer while a known call waits for a decision,
// so the conversation that led to it stays in view above.
func (m AppModel) approvalCard() string {
	width := max(1, m.Layout.Width)
	warn := func(s string) string {
		if m.Theme.Monochrome {
			return s
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.Theme.palette().Warning)).Render(s)
	}
	rule := warn(strings.Repeat("─", width))
	title := " " + warn(m.Theme.Icon("attention")+" "+m.Theme.T("approval.title")) + "  " + m.Approval.Target
	if m.Approval.Details.VisualLineCount() > m.approvalDetailRows() {
		title += m.Theme.Muted("  " + m.Theme.T("approval.scroll"))
	}
	details := m.Approval.Details
	details.Gutter = (width - details.Viewport.Width()) / 2
	hints := []footerHint{
		{"approve", "a", m.Theme.T("approval.hintApprove")},
		{"always-allow", "l", m.Theme.T("approval.hintAlways")},
		{"deny", "d", m.Theme.T("approval.hintDeny")},
		{"autonomy-on", "f", m.Theme.T("approval.hintAutonomy")},
		{"cancel", "esc", m.Theme.T("footer.cancel")},
	}
	var actions []string
	for _, h := range hints {
		actions = append(actions, m.zones.Mark(m.prefix+h.zone, m.footerKey(h.key)+m.Theme.Muted(" "+h.label)))
	}
	line := func(s string) string {
		return lipgloss.NewStyle().Width(width).Render(ansi.Truncate(s, width, "…"))
	}
	if m.Theme.editorial() {
		return m.approvalSheet(details, hints)
	}
	return strings.Join([]string{rule, line(title), details.View(), line(" " + strings.Join(actions, "   "))}, "\n")
}

// mainTranscript is the conversation sized to the rows the composer or the
// approval card leave free.
func (m AppModel) mainTranscript() string {
	if !m.inlineApproval() {
		return m.Transcript.View()
	}
	t := m.Transcript
	bottom := t.Viewport.AtBottom()
	t.Viewport.SetHeight(max(1, m.Layout.Height-2-approvalCardChrome-m.approvalDetailRows()))
	if bottom {
		t.Viewport.GotoBottom()
	}
	return t.View()
}

func (m AppModel) inlineApproval() bool {
	return m.approvalFocus() && m.knownPending()
}

// approvalSheet is the Editorial card: a raised sheet instead of a warning
// rule, titled with what AXLR wants to do. Same rows, same keys.
func (m AppModel) approvalSheet(details Transcript, hints []footerHint) string {
	width := max(1, m.Layout.Width)
	sheet := lipgloss.NewStyle().Width(width)
	if !m.Theme.Monochrome {
		p := m.Theme.palette()
		sheet = sheet.Background(lipgloss.Color(p.Surface)).Foreground(lipgloss.Color(p.Text))
	}
	row := func(s string) string { return sheet.Render(ansi.Truncate(s, width, "…")) }
	title := "  " + m.Theme.Tf("editorial.approvalTitle", m.Approval.Target)
	if m.Approval.Details.VisualLineCount() > m.approvalDetailRows() {
		title += "  " + m.Theme.T("approval.scroll")
	}
	rows := []string{row(""), sheet.Bold(true).Render(ansi.Truncate(title, width, "…"))}
	for _, l := range strings.Split(details.View(), "\n") {
		rows = append(rows, row(ansi.Strip(l)))
	}
	// Plain text keeps the sheet's background across the row; the zone marks
	// keep each action clickable.
	var actions []string
	for _, h := range hints {
		actions = append(actions, m.zones.Mark(m.prefix+h.zone, h.key+" "+h.label))
	}
	rows = append(rows, row("  "+strings.Join(actions, "   ")))
	return strings.Join(rows, "\n")
}
