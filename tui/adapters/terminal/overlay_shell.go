package terminal

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// OverlayBodySize leaves a quiet header, a gutter, and fixed controls while
// giving scrollable content the rest of the terminal.
func OverlayBodySize(width, height int) (int, int) {
	return max(1, width-4), max(1, height-6)
}

func (t Theme) overlayLine(content string, width int, raised bool) string {
	content = "  " + ansi.Truncate(content, max(1, width-4), "…")
	style := lipgloss.NewStyle().Width(width)
	if !t.Monochrome {
		background := t.palette().Surface
		if raised {
			background = t.palette().Raised
		}
		style = style.Background(lipgloss.Color(background))
	}
	return style.Render(content)
}

// Overlay renders one full-height surface; callers own scroll and focus.
func (t Theme) Overlay(title, hint, body, footer string, width, height int) string {
	width, height = max(1, width), max(1, height)
	_, bodyHeight := OverlayBodySize(width, height)
	bodyLines := strings.Split(body, "\n")
	if len(bodyLines) > bodyHeight {
		bodyLines = bodyLines[:bodyHeight]
	}
	for len(bodyLines) < bodyHeight {
		bodyLines = append(bodyLines, "")
	}
	lines := []string{
		t.overlayLine("", width, false),
		t.overlayLine(t.Heading(title), width, true),
		t.overlayLine(t.Muted(hint), width, false),
		t.overlayLine("", width, false),
	}
	for _, line := range bodyLines {
		lines = append(lines, t.overlayLine(line, width, false))
	}
	lines = append(lines, t.overlayLine("", width, false), t.overlayLine(t.Muted(footer), width, true))
	return strings.Join(lines, "\n")
}

// jumpViewport moves a scrolling panel to its top on Home or Ctrl+Home and
// to its end on End or Ctrl+End, and reports whether the key was one of
// them. On 14 October 2026 the info panel (Ctrl+P → I) scrolled with PgUp
// and PgDn and ignored Home and End, as /usage, /session and the /mcp
// detail did.
func jumpViewport(v *viewport.Model, key string) bool {
	switch key {
	case "home", "ctrl+home":
		v.GotoTop()
	case "end", "ctrl+end":
		v.GotoBottom()
	default:
		return false
	}
	return true
}
