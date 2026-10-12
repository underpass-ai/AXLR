package terminal

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/charmbracelet/x/ansi"
)

// SearchBox is Ctrl+F: its query, every occurrence of it in the
// conversation as drawn, and the one selected. On 12 October 2026 it
// counted and stepped per message ("1/2" with three matches highlighted)
// and landed on the start of the message.
type SearchBox struct {
	Input    textinput.Model
	Hits     []searchHit
	Selected int
	Locale   Locale
	// query is what Hits were found for: a key that leaves it unchanged,
	// such as an arrow, does not search again and reset the selection.
	query string
}

// searchHit is one occurrence: the wrapped line it is on and the cell it
// starts at, as the transcript draws it.
type searchHit struct{ Line, Cell int }

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

// current is the selected hit, if any.
func (b SearchBox) current() (searchHit, bool) {
	if b.Selected < 0 || b.Selected >= len(b.Hits) {
		return searchHit{}, false
	}
	return b.Hits[b.Selected], true
}

// findHits lists every case-insensitive occurrence of query in the
// wrapped lines, as highlightMatches marks them, so the count and the
// highlight agree.
func findHits(content, query string) []searchHit {
	needle := []rune(query)
	if len(needle) == 0 {
		return nil
	}
	var hits []searchHit
	for index, line := range strings.Split(content, "\n") {
		runes := []rune(ansi.Strip(line))
		for i := 0; i+len(needle) <= len(runes); {
			if !strings.EqualFold(string(runes[i:i+len(needle)]), query) {
				i++
				continue
			}
			hits = append(hits, searchHit{Line: index, Cell: ansi.StringWidth(string(runes[:i]))})
			i += len(needle)
		}
	}
	return hits
}
