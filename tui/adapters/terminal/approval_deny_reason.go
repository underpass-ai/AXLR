package terminal

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// The denial's reason. On 12 October 2026 D sent a bare denial and the
// model guessed what to change ("should I change the text?"). D now opens
// a one-line reason the denial carries to the model: Enter denies with it,
// Esc denies without one, Ctrl+C goes back to the card undecided.

// askDenyReason opens the reason input in place of the card's keys.
func (m AppModel) askDenyReason() AppModel {
	input := textinput.New()
	input.Prompt = m.Theme.T("approval.denyPrompt")
	input.Placeholder = m.Theme.T("approval.denyPlaceholder")
	input.CharLimit = 2000
	input.SetWidth(max(1, m.Layout.Width-len([]rune(input.Prompt))-3))
	input.Focus()
	m.denyInput, m.denying = input, true
	return m
}

// denyReasonKey handles a key while the reason input is open.
func (m AppModel) denyReasonKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch k.String() {
	case "enter":
		return m.navigation(ControlIntent("deny-send"))
	case "esc":
		m.denyInput.SetValue("")
		return m.navigation(ControlIntent("deny-send"))
	case "ctrl+c":
		m.denying = false
		return m, nil, true
	}
	var cmd tea.Cmd
	m.denyInput, cmd = m.denyInput.Update(k)
	return m, cmd, true
}
