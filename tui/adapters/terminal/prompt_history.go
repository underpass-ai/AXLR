package terminal

import root "github.com/underpass-ai/AXLR/domain"

// Prompt history contains only user messages, in this session's order.
func (m *AppModel) resetPromptHistory() {
	m.promptHistory = nil
	for _, msg := range m.Header.State.Messages {
		if msg.Role == root.RoleUser && !consoleMemoryReminder(msg.Content) {
			m.promptHistory = append(m.promptHistory, string(msg.Content))
		}
	}
	m.historyIndex = len(m.promptHistory)
	m.historyDraft = ""
}

func (m *AppModel) rememberPrompt(prompt string) {
	if prompt == "" {
		return
	}
	m.promptHistory = append(m.promptHistory, prompt)
	m.historyIndex = len(m.promptHistory)
	m.historyDraft = ""
}

func (m *AppModel) previousPrompt() bool {
	if m.historyIndex <= 0 {
		return false
	}
	if m.historyIndex == len(m.promptHistory) {
		m.historyDraft = m.Composer.Input.Value()
	}
	m.historyIndex--
	m.Composer.Input.SetValue(m.promptHistory[m.historyIndex])
	m.resizeComposer()
	return true
}

func (m *AppModel) nextPrompt() bool {
	if m.historyIndex >= len(m.promptHistory) {
		return false
	}
	m.historyIndex++
	if m.historyIndex == len(m.promptHistory) {
		m.Composer.Input.SetValue(m.historyDraft)
		m.historyDraft = ""
	} else {
		m.Composer.Input.SetValue(m.promptHistory[m.historyIndex])
	}
	m.resizeComposer()
	return true
}
