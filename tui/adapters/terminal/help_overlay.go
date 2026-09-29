package terminal

import zone "github.com/lrstanley/bubblezone/v2"

type HelpOverlay struct{}

func (HelpOverlay) View(z *zone.Manager, p string) string {
	return "Keyboard help\nCtrl+S Send • Enter Newline\n/model + Enter or Ctrl+S: Choose model\nCtrl+P Actions • Ctrl+F Search\nCtrl+O Sessions • F1 Help • Ctrl+R Continue\nTab Transcript / activity • PgUp/PgDn Scroll\nApproval: A Approve • D Deny • Esc Cancel\nApproval details: ↑↓ / PgUp/PgDn / wheel\nSearch: Enter Next • Shift+Enter Previous\nSessions: ↑↓ Select • Enter Open\nEsc Close overlay / cancel turn • Ctrl+C Quit\n" + z.Mark(p+"close", "[Close Esc]")
}
