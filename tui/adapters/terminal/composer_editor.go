package terminal

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// editorDone carries the draft back from the external editor Ctrl+G
// opened, or why it could not be read.
type editorDone struct {
	Text string
	Err  error
}

// editorCommand is the editor Ctrl+G runs on path: $VISUAL, then $EDITOR,
// split on spaces so "code --wait" works, then vi (notepad on Windows).
func editorCommand(getenv func(string) string, path string) *exec.Cmd {
	editor := strings.TrimSpace(getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
		if runtime.GOOS == "windows" {
			editor = "notepad"
		}
	}
	fields := strings.Fields(editor)
	return exec.Command(fields[0], append(fields[1:], path)...)
}

// editDraft opens the composer's draft in the person's editor and puts
// what they saved back in the composer. On 12 October 2026 a long prompt
// could only be written in the composer, where Shift+Enter is the only
// newline some terminals cannot send.
func (m AppModel) editDraft() (tea.Model, tea.Cmd) {
	file, err := os.CreateTemp("", "axlr-draft-*.md")
	if err != nil {
		m.Status.Error = m.Theme.Tf("editor.failed", err.Error())
		return m, nil
	}
	path := file.Name()
	_, err = file.WriteString(m.expandPastes(m.Composer.Input.Value()))
	err = errors.Join(err, file.Close())
	if err != nil {
		_ = os.Remove(path)
		m.Status.Error = m.Theme.Tf("editor.failed", err.Error())
		return m, nil
	}
	return m, tea.ExecProcess(editorCommand(os.Getenv, path), func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return editorDone{Err: err}
		}
		data, err := os.ReadFile(path)
		return editorDone{Text: string(data), Err: err}
	})
}

// editorResult puts the edited draft in the composer; a failed editor
// leaves the draft as it was.
func (m AppModel) editorResult(done editorDone) AppModel {
	if done.Err != nil {
		m.Status.Error = m.Theme.Tf("editor.failed", done.Err.Error())
		return m
	}
	m.Status.Error = ""
	m.pastes = nil
	m.Composer.Input.SetValue(strings.TrimRight(done.Text, "\r\n"))
	m.slashSelected = 0
	m.resizeComposer()
	return m
}
