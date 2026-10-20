package terminal

import root "github.com/underpass-ai/AXLR/domain"

// Prompt history contains only what the person typed, in this session's
// order: console notes, whole messages or appended to a prompt, are left
// out. On 12 October 2026 ↑ recalled "[AXLR · budget]" notes and ceremony
// decisions.
func (m *AppModel) resetPromptHistory() {
	m.promptHistory = nil
	for _, msg := range m.Header.State.Messages {
		if msg.Role != root.RoleUser {
			continue
		}
		if typed, _ := splitConsoleNotes(msg.Content); typed != "" {
			m.promptHistory = append(m.promptHistory, typed)
		}
	}
	m.promptHistory = append(m.promptHistory, m.commandHistory...)
	m.historyIndex = len(m.promptHistory)
	m.historyDraft = ""
}

// rememberCommand keeps a slash command for ↑: on 20 October 2026 ↑ never
// recalled /tools publish, which the person had to type again each time.
// The commands typed in this console stay across its sessions; a command
// repeated at once is kept once.
func (m *AppModel) rememberCommand(command string) {
	if command == "" {
		return
	}
	if n := len(m.commandHistory); n == 0 || m.commandHistory[n-1] != command {
		m.commandHistory = append(m.commandHistory, command)
	}
	if n := len(m.promptHistory); n > 0 && m.promptHistory[n-1] == command {
		m.historyIndex = n
		m.historyDraft = ""
		return
	}
	m.rememberPrompt(command)
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
