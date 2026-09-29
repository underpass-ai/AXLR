package terminal

import (
	"charm.land/bubbles/v2/textinput"
	"fmt"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type SearchBox struct {
	Input    textinput.Model
	Hits     []domain.SearchHit
	Selected int
}

func NewSearchBox() SearchBox {
	i := textinput.New()
	i.Prompt = "Search: "
	i.SetVirtualCursor(false)
	i.Focus()
	return SearchBox{Input: i}
}
func (s SearchBox) View(z *zone.Manager, p string) string {
	n := 0
	if len(s.Hits) > 0 {
		n = s.Selected + 1
	}
	return s.Input.View() + fmt.Sprintf(" %d/%d", n, len(s.Hits)) + "\n" + z.Mark(p+"previous", "[Previous Shift+Enter]") + " " + z.Mark(p+"next", "[Next Enter]") + " " + z.Mark(p+"close", "[Close Esc]")
}
