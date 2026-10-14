package terminal

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
)

// HelpOverlay lists the keys and commands; Notice, such as an exec sandbox
// that does not confine commands, leads it. Top is the first row shown when
// the list is taller than the overlay: on 12 October 2026 F1 at 15 rows lost
// its last rows (approve, quit, copy) and could not scroll to them.
type HelpOverlay struct {
	Notice string
	Top    int
}

// rows is the whole help, notice first, wrapped to the overlay's width.
func (h HelpOverlay) rows(theme Theme, inner int) []string {
	rows := []string{
		theme.Accent(theme.T("help.write")),
		theme.T("help.sendFull"),
		theme.T("help.commandsSession"),
		theme.T("help.commandsMemory"),
		theme.T("help.commandsModelTheme"),
		theme.T("help.changes"),
		theme.T("help.commandsMCPPlugins"),
		theme.T("help.commandsUpdate"),
		theme.T("help.commandsModes"),
		theme.T("help.commandsCeremonies"),
		theme.T("help.commandsJobs"),
		"",
		theme.Accent(theme.T("help.navigate")),
		theme.T("help.shortcuts"),
		theme.T("help.views"),
		theme.T("help.copy"),
		theme.T("help.quitFull"),
		"",
		theme.Accent(theme.T("help.approvals")),
		theme.T("help.approveFull"),
		theme.T("help.pluginReview"),
		theme.T("help.modelSelect"),
		theme.T("help.themeOptions"),
	}
	if h.Notice != "" {
		rows = append([]string{h.Notice, ""}, rows...)
	}
	// Every row wraps: on 14 October 2026 the longer ones ended in "…" even
	// at 160 columns.
	var wrapped []string
	for _, row := range rows {
		wrapped = append(wrapped, strings.Split(ansi.Wrap(row, max(1, inner), ""), "\n")...)
	}
	return wrapped
}

// window is the first row shown and how many rows fit with the marker row
// that counts what is above and below.
func (h HelpOverlay) window(total, body int) (top, shown int) {
	if total <= body {
		return 0, total
	}
	shown = max(1, body-1)
	return max(0, min(h.Top, total-shown)), shown
}

// Scroll moves the list by delta rows within what the overlay can show.
func (h *HelpOverlay) Scroll(theme Theme, width, height, delta int) {
	inner, body := OverlayBodySize(width, height)
	total := len(h.rows(theme, inner))
	top, shown := h.window(total, body)
	h.Top = max(0, min(top+delta, total-shown))
}

func (h HelpOverlay) View(theme Theme, z *zone.Manager, prefix string, width, height int) string {
	inner, body := OverlayBodySize(width, height)
	rows := h.rows(theme, inner)
	if top, shown := h.window(len(rows), body); shown < len(rows) {
		above, below := top, len(rows)-top-shown
		rows = rows[top : top+shown]
		marker := theme.Tf("slash.more", below)
		switch {
		case above > 0 && below > 0:
			marker = theme.Tf("slash.aboveBelow", above, below)
		case above > 0:
			marker = theme.Tf("slash.above", above)
		}
		rows = append(rows, theme.Muted(marker))
	}
	footer := z.Mark(prefix+"close", "["+theme.T("common.close")+"]")
	return theme.Overlay(theme.T("palette.helpTitle"), theme.T("help.subtitle"), strings.Join(rows, "\n"), footer, width, height)
}
