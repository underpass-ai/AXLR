package terminal

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// approvalCardChrome is the card's fixed rows: rule, title and actions.
const approvalCardChrome = 3

// approvalDetailRows is how many argument rows the inline card shows: all of
// them when they fit, otherwise what keeps the whole card within 60 % of
// the screen (at least 3 rows), which the arrows and wheel scroll. On 12
// October 2026 the card stopped at ten rows with thirty free, so a
// 12-line diff needed scrolling.
func (m AppModel) approvalDetailRows() int {
	limit := max(3, m.Layout.Height*3/5-approvalCardChrome)
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
	if !savesAlwaysAllow(m.Header.State.ToolSnapshot, m.Approval.Pending) {
		// A host tool has no saved rule of its own.
		hints = slices.DeleteFunc(hints, func(h footerHint) bool { return h.zone == "always-allow" })
	}
	note := ""
	if m.modeAsksForEachCall(m.Approval.Pending) {
		// The mode decides; neither a saved rule nor autonomy would apply.
		hints = slices.DeleteFunc(hints, func(h footerHint) bool { return h.zone == "always-allow" || h.zone == "autonomy-on" })
		// On 10 October 2026, in review mode under full autonomy, git
		// status and git diff each raised a card without l or f, and
		// nothing said why: the card now says the mode asks.
		note = m.Theme.Tf("approval.modeAsks", m.Theme.T("mode."+string(m.Header.State.Mode)))
	}
	var actions []string
	for _, h := range hints {
		actions = append(actions, m.zones.Mark(m.prefix+h.zone, m.footerKey(h.key)+m.Theme.Muted(" "+h.label)))
	}
	line := func(s string) string {
		return lipgloss.NewStyle().Width(width).Render(ansi.Truncate(s, width, "…"))
	}
	if m.Theme.editorial() {
		return m.approvalSheet(details, hints, note)
	}
	if note != "" {
		actions = append(actions, m.Theme.Muted("· "+note))
	}
	return strings.Join([]string{rule, line(title), details.View(), line(" " + strings.Join(actions, "   "))}, "\n")
}

// mainTranscript is the conversation sized to the rows the composer or the
// approval card leave free.
func (m AppModel) mainTranscript() string {
	t := m.shownTranscript()
	if m.overlay == "search" {
		t.Highlight = m.SearchBox.query
		if hit, ok := m.SearchBox.current(); ok {
			t.CurrentHit = &hit
		}
	}
	return t.View()
}

// shownTranscript is the conversation as the main view draws it: the
// approval card and a long error take rows from it, and it keeps showing
// its latest line when it was at the bottom. Mouse selection maps cells
// through the same geometry.
func (m AppModel) shownTranscript() Transcript {
	t := m.Transcript
	height := t.Viewport.Height()
	if m.inlineApproval() {
		height = m.Layout.Height - 2 - approvalCardChrome - m.approvalDetailRows()
	}
	height = max(1, height-(m.footerRows()-1))
	if height != t.Viewport.Height() {
		bottom := t.Viewport.AtBottom()
		t.Viewport.SetHeight(height)
		if bottom {
			t.Viewport.GotoBottom()
		}
	}
	return t
}

func (m AppModel) inlineApproval() bool {
	return m.approvalFocus() && m.knownPending()
}

// approvalSheet is the Editorial card: a raised sheet instead of a warning
// rule, titled with what AXLR wants to do. Same rows, same keys.
func (m AppModel) approvalSheet(details Transcript, hints []footerHint, note string) string {
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
	if note != "" {
		actions = append(actions, "· "+note)
	}
	rows = append(rows, row("  "+strings.Join(actions, "   ")))
	return strings.Join(rows, "\n")
}
