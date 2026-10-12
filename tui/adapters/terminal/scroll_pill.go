package terminal

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Scrolled up. On 12 October 2026, after PgUp, the view stayed put while a
// reply streamed below and nothing said new content had arrived; Home and
// End belonged to the composer and the only way down was PgDn after PgDn.
// While the conversation is not at its bottom, a row above the composer
// says so, with the lines added since it left the bottom; End with an
// empty composer, Ctrl+End or a click on the row go back down, and
// Ctrl+Home goes to the top.

// markBottom remembers how long the conversation was while it showed its
// last line, so the lines added after it left count as new.
func (m *AppModel) markBottom() {
	if m.Transcript.Viewport.AtBottom() {
		m.bottomLines = m.Transcript.VisualLineCount()
	}
}

// newLines is how many lines were added since the conversation left its
// bottom, and false while it is at the bottom.
func (m AppModel) newLines() (int, bool) {
	if m.Transcript.Viewport.AtBottom() || m.Transcript.VisualLineCount() <= m.Transcript.Viewport.Height() {
		return 0, false
	}
	return max(0, m.Transcript.VisualLineCount()-m.bottomLines), true
}

// jumpBottom shows the conversation's last line.
func (m *AppModel) jumpBottom() {
	m.Transcript.Viewport.GotoBottom()
	m.markBottom()
}

// scrollPill is the row above the composer while scrolled up, or nil.
func (m AppModel) scrollPill() []string {
	count, away := m.newLines()
	if !away {
		return nil
	}
	text := m.Theme.T("scroll.pill")
	if count > 0 {
		text = m.Theme.Tn("scroll.pillNew", count, count)
	}
	width := max(1, m.Layout.Width)
	style := lipgloss.NewStyle().Width(width)
	if !m.Theme.Monochrome {
		p := m.Theme.palette()
		style = style.Background(lipgloss.Color(p.Raised)).Foreground(lipgloss.Color(p.Accent))
	}
	return []string{m.zones.Mark(m.prefix+"jump-bottom", style.Render(ansi.Truncate("  "+text, width, "…")))}
}
