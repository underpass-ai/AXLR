package terminal

import (
	"fmt"
	"strings"
)

// pasteFoldLines is how many lines a paste may have before the composer
// and the conversation show it folded. On 12 October 2026 a 200-line paste
// filled the composer to its maximum and the transcript with its bubble.
const pasteFoldLines = 20

// pastedText is a long paste the composer shows as its token.
type pastedText struct{ token, text string }

// lineCount counts the lines of a text, a final newline ending the last.
func lineCount(text string) int {
	return strings.Count(strings.TrimSuffix(text, "\n"), "\n") + 1
}

// pasteToken is how a long paste reads folded: "[pasted 200 lines · 6.1 KB]".
func pasteToken(theme Theme, text string) string {
	n := lineCount(text)
	return theme.Tn("paste.token", n, n, formatBytes(len(text)))
}

// paste puts a paste into the composer: as typed when it is short, as its
// token otherwise, kept so the prompt sent carries the whole text.
func (m *AppModel) paste(text string) {
	// tmux and GNOME Terminal send a pasted newline as a carriage return.
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if lineCount(text) <= pasteFoldLines {
		m.Composer.Input.InsertString(text)
		return
	}
	token := pasteToken(m.Theme, text)
	for n := 2; strings.Contains(m.Composer.Input.Value(), token) || m.hasPasteToken(token); n++ {
		token = strings.TrimSuffix(pasteToken(m.Theme, text), "]") + fmt.Sprintf(" #%d]", n)
	}
	m.pastes = append(m.pastes, pastedText{token: token, text: text})
	m.Composer.Input.InsertString(token)
}

func (m AppModel) hasPasteToken(token string) bool {
	for _, p := range m.pastes {
		if p.token == token {
			return true
		}
	}
	return false
}

// expandPastes is the draft as the model receives it: each paste token
// still in it replaced by the text it stands for. A token the person
// edited stays as typed.
func (m *AppModel) expandPastes(draft string) string {
	for _, p := range m.pastes {
		if i := strings.Index(draft, p.token); i >= 0 {
			draft = draft[:i] + p.text + draft[i+len(p.token):]
			m.pasted[p.text] = true
		}
	}
	return draft
}

// foldPastes is a prompt as the conversation shows it: each long paste of
// this console as its token, or, for a long prompt it did not see pasted
// (a session saved earlier), all of it as "[N lines · size]". folded
// reports whether anything was folded, so a click can open it.
func foldPastes(text string, pasted map[string]bool, theme Theme) (shown string, folded bool) {
	shown = text
	for block := range pasted {
		if block != "" && strings.Contains(shown, block) {
			shown = strings.ReplaceAll(shown, block, pasteToken(theme, block))
			folded = true
		}
	}
	if !folded && lineCount(text) > pasteFoldLines {
		n := lineCount(text)
		return theme.Tn("paste.folded", n, n, formatBytes(len(text))), true
	}
	return shown, folded
}
