package terminal

import (
	"charm.land/bubbles/v2/textinput"
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
