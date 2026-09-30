package terminal

import (
	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

type ActionPalette struct{}

func (ActionPalette) Intent(msg tea.Msg) ControlIntent {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "m":
			return "models"
		case "s":
			return "search"
		case "o":
			return "sessions"
		case "h":
			return "help"
		case "i":
			return "info"
		case "c":
			return "cancel"
		case "r":
			return "continue"
		}
	}
	return ""
}
func (ActionPalette) View(z *zone.Manager, p string) string {
	return "Actions\n" + z.Mark(p+"models", "[Models M]") + "\n" + z.Mark(p+"search", "[Search S]") + "\n" + z.Mark(p+"sessions", "[Sessions O]") + "\n" + z.Mark(p+"help", "[Help H]") + "\n" + z.Mark(p+"info", "[Model / workspace I]") + "\n" + z.Mark(p+"continue", "[Continue interrupted turn R]") + "\n" + z.Mark(p+"cancel", "[Cancel turn C]") + "\n" + z.Mark(p+"close", "[Close Esc]")
}
