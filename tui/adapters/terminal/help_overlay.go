package terminal

import (
	"strings"

	zone "github.com/lrstanley/bubblezone/v2"
)

type HelpOverlay struct{}

func (HelpOverlay) View(theme Theme, z *zone.Manager, prefix string, width, height int) string {
	rows := []string{
		theme.Accent(theme.T("help.write")),
		theme.T("help.sendFull"),
		theme.T("help.commandsModelTheme"),
		theme.T("help.commandsMCPPlugins"),
		"",
		theme.Accent(theme.T("help.navigate")),
		theme.T("help.shortcuts"),
		theme.T("help.views"),
		theme.T("help.quitFull"),
		"",
		theme.Accent(theme.T("help.approvals")),
		theme.T("help.approveFull"),
		theme.T("help.pluginReview"),
		theme.T("help.modelSelect"),
		theme.T("help.themeOptions"),
	}
	if height < 20 {
		rows = []string{
			theme.T("help.sendShort"),
			theme.T("help.commandsModelTheme"),
			theme.T("help.commandsMCPPlugins"),
			theme.T("help.shortcuts"),
			theme.T("help.views"),
			theme.T("help.approveShort"),
			theme.T("help.quitShort"),
		}
	}
	footer := z.Mark(prefix+"close", "["+theme.T("common.close")+"]")
	return theme.Overlay(theme.T("palette.helpTitle"), theme.T("help.subtitle"), strings.Join(rows, "\n"), footer, width, height)
}
