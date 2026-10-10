package terminal

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
)

// HelpOverlay lists the keys and commands; Notice, such as an exec sandbox
// that does not confine commands, leads it.
type HelpOverlay struct{ Notice string }

func (h HelpOverlay) View(theme Theme, z *zone.Manager, prefix string, width, height int) string {
	rows := []string{
		theme.Accent(theme.T("help.write")),
		theme.T("help.sendFull"),
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
	inner, body := OverlayBodySize(width, height)
	var notice []string
	if h.Notice != "" {
		notice = append(strings.Split(ansi.Wrap(h.Notice, max(1, inner), ""), "\n"), "")
	}
	if body < len(notice)+len(rows) {
		// The full list no longer fits once it names every command.
		rows = []string{
			theme.T("help.sendShort"),
			theme.T("help.commandsModelTheme"),
			theme.T("help.changes"),
			theme.T("help.commandsMCPPlugins"),
			theme.T("help.commandsUpdate"),
			theme.T("help.commandsShort"),
			theme.T("help.shortcuts"),
			theme.T("help.views"),
			theme.T("help.copyShort"),
			theme.T("help.approveShort"),
			theme.T("help.quitShort"),
		}
	}
	rows = append(notice, rows...)
	footer := z.Mark(prefix+"close", "["+theme.T("common.close")+"]")
	return theme.Overlay(theme.T("palette.helpTitle"), theme.T("help.subtitle"), strings.Join(rows, "\n"), footer, width, height)
}
