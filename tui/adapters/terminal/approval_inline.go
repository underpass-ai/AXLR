package terminal

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// approvalCardChrome is the card's fixed rows besides its keys: rule and
// title (the Editorial sheet: a blank row and the title).
const approvalCardChrome = 2

// approvalChrome is the card's rows besides its details: the fixed ones
// and as many rows as its keys wrap to.
func (m AppModel) approvalChrome() int {
	return approvalCardChrome + len(m.approvalKeyRows())
}

// approvalDetailRows is how many argument rows the inline card shows: all of
// them when they fit, otherwise what keeps the whole card within 60 % of
// the screen (at least 3 rows), which the arrows and wheel scroll. On 12
// October 2026 the card stopped at ten rows with thirty free, so a
// 12-line diff needed scrolling.
func (m AppModel) approvalDetailRows() int {
	limit := max(3, m.Layout.Height*3/5-m.approvalChrome())
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
	details := m.Approval.Details
	details.Gutter = (width - details.Viewport.Width()) / 2
	_, note := m.approvalHints()
	if m.denying {
		// The reason takes the keys' row; its keys go on the title's.
		note = m.Theme.T("approval.denyHints")
	}
	if m.Theme.editorial() {
		return m.approvalSheet(details)
	}
	line := func(s string) string {
		return lipgloss.NewStyle().Width(width).Render(ansi.Truncate(s, width, "…"))
	}
	if note != "" {
		// On the title's row: the keys' row is full with S offered.
		title += m.Theme.Muted("  · " + note)
	}
	if m.Approval.Details.VisualLineCount() > m.approvalDetailRows() {
		title += m.Theme.Muted("  " + m.Theme.T("approval.scroll"))
	}
	rows := []string{rule, line(title), details.View()}
	for _, keys := range m.approvalKeyRows() {
		rows = append(rows, line(keys))
	}
	return strings.Join(rows, "\n")
}

// approvalHints are the keys the card offers for its call, and the note
// that says why some are missing.
func (m AppModel) approvalHints() ([]footerHint, string) {
	hints := []footerHint{
		{"approve", "a", m.Theme.T("approval.hintApprove")},
		{"approve-all", "A", m.Theme.Tf("approval.hintApproveAll", m.pendingCalls())},
		{"allow-session", "s", m.Theme.T("approval.hintSession")},
		{"always-allow", "l", m.Theme.T("approval.hintAlways")},
		{"deny", "d", m.Theme.T("approval.hintDeny")},
		{"autonomy-on", "f", m.Theme.T("approval.hintAutonomy")},
		{"cancel", "esc", m.Theme.T("footer.cancel")},
	}
	if m.execCardCommand(m.Approval.Pending) == "" {
		// Only a local command is allowed exactly for the session.
		hints = slices.DeleteFunc(hints, func(h footerHint) bool { return h.zone == "allow-session" })
	}
	if m.pendingCalls() < 2 {
		// Shift+A approves the calls queued behind this one.
		hints = slices.DeleteFunc(hints, func(h footerHint) bool { return h.zone == "approve-all" })
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
	if rule := m.destructiveRule(m.Approval.Pending); rule != "" {
		// The destructive list keeps its card under autonomy (decision K5a).
		hints = slices.DeleteFunc(hints, func(h footerHint) bool { return h.zone == "always-allow" || h.zone == "autonomy-on" })
		note = m.Theme.Tf("approval.destructive", rule)
	}
	return hints, note
}

// approvalKeyRows are the card's key rows: its keys, wrapped onto as many
// rows as the width needs, or the reason being typed for a denial. On 14
// October 2026, at 70 columns, the keys were cut at "l permitir siem…" and
// d (deny) and esc were not shown at all.
func (m AppModel) approvalKeyRows() []string {
	if m.denying {
		lead := " "
		if m.Theme.editorial() {
			lead = "  "
		}
		return []string{lead + m.denyInput.View()}
	}
	hints, note := m.approvalHints()
	var actions []string
	var widths []int
	for _, h := range hints {
		plain := h.key + " " + h.label
		if m.Theme.editorial() {
			actions = append(actions, m.zones.Mark(m.prefix+h.zone, plain))
		} else {
			actions = append(actions, m.zones.Mark(m.prefix+h.zone, m.footerKey(h.key)+m.Theme.Muted(" "+h.label)))
		}
		widths = append(widths, ansi.StringWidth(plain))
	}
	lead := " "
	if m.Theme.editorial() {
		// The sheet's note follows its keys.
		lead = "  "
		if note != "" {
			actions = append(actions, "· "+note)
			widths = append(widths, ansi.StringWidth("· "+note))
		}
	}
	return wrapActions(actions, widths, lead, "   ", max(1, m.Layout.Width))
}

// wrapActions packs actions, whose plain widths are given, into rows of
// width cells: each row starts with lead and separates its actions with
// sep. An action wider than a row has a row of its own.
func wrapActions(actions []string, widths []int, lead, sep string, width int) []string {
	var rows []string
	row, used := "", 0
	for i, action := range actions {
		switch {
		case row == "":
			row, used = lead+action, ansi.StringWidth(lead)+widths[i]
		case used+ansi.StringWidth(sep)+widths[i] <= width:
			row += sep + action
			used += ansi.StringWidth(sep) + widths[i]
		default:
			rows = append(rows, row)
			row, used = lead+action, ansi.StringWidth(lead)+widths[i]
		}
	}
	if row != "" || len(rows) == 0 {
		rows = append(rows, row)
	}
	return rows
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
		height = m.Layout.Height - 2 - m.approvalChrome() - m.approvalDetailRows()
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
func (m AppModel) approvalSheet(details Transcript) string {
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
	for _, keys := range m.approvalKeyRows() {
		if m.denying {
			keys = ansi.Strip(keys)
		}
		rows = append(rows, row(keys))
	}
	return strings.Join(rows, "\n")
}
