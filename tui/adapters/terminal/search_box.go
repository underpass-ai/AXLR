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
	Locale   Locale
}

func NewSearchBox(locales ...Locale) SearchBox {
	locale := English
	if len(locales) > 0 {
		locale = locales[0]
	}
	i := textinput.New()
	i.Prompt = Translate(locale, "common.searchPrompt")
	i.SetVirtualCursor(false)
	i.Focus()
	return SearchBox{Input: i, Locale: locale}
}
func (s SearchBox) View(z *zone.Manager, p string) string {
	n := 0
	if len(s.Hits) > 0 {
		n = s.Selected + 1
	}
	return s.Input.View() + fmt.Sprintf(" %d/%d", n, len(s.Hits)) + "\n" + z.Mark(p+"previous", "["+Translate(s.Locale, "search.previous")+"]") + " " + z.Mark(p+"next", "["+Translate(s.Locale, "search.next")+"]") + " " + z.Mark(p+"close", "["+Translate(s.Locale, "common.close")+"]")
}
