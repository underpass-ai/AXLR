package terminal

import zone "github.com/lrstanley/bubblezone/v2"

type HelpOverlay struct{}

func (HelpOverlay) View(z *zone.Manager, p string) string {
	return "Keyboard help\nEnter Send • Shift+Enter Newline\n/model Models · /mcp Servers · /plugin Policies\nCtrl+P Actions • Ctrl+F Search\nCtrl+O Sessions • F1 Help • Ctrl+R Continue\nTab Transcript / activity • PgUp/PgDn Scroll\nApproval: A Approve • D Deny • Esc Cancel\nApproval details: ↑↓ / PgUp/PgDn / wheel\nSearch: Enter Next • Shift+Enter Previous\nPlugins: A Autoapprove on/off · R Refresh\nSessions: ↑↓ Select • Enter Open\nEsc Close overlay / cancel turn • Ctrl+C Quit\n" + z.Mark(p+"close", "[Close Esc]")
}
