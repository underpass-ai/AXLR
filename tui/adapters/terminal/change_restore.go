package terminal

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Undo. On 12 October 2026 Ctrl+D showed exact before and after snapshots
// but was read-only. u in Ctrl+D puts the selected file back to what it
// held before the selected step, and /undo puts back every file the last
// turn changed; each asks first and refuses a file that changed since.

// restoreStep is the step u restores: the selected step, or the only step
// of the selected file. A restore is not a step; a file with several steps
// asks for one.
func (v ChangeViewer) restoreStep() (changeRecord, string) {
	if v.selected >= len(v.rows) {
		return changeRecord{}, "changes.restoreNone"
	}
	row := v.rows[v.selected]
	file := v.files[row.File]
	step := row.Step
	if step < 0 {
		if len(file.Steps) != 1 {
			return changeRecord{}, "changes.restorePick"
		}
		step = 0
	}
	record := v.records[file.Steps[step]]
	if record.Restore {
		return changeRecord{}, "changes.restoreRestore"
	}
	return record, ""
}

// askRestore asks to confirm u on the selection, or says why it cannot.
func (v *ChangeViewer) askRestore() {
	record, refused := v.restoreStep()
	if refused != "" {
		v.notice = v.Theme.T(refused)
		return
	}
	v.confirm = record.ID
}

func (m AppModel) restoreUseCase() application.RestoreFileUseCase {
	return application.RestoreFileUseCase{Tools: m.deps.Start.Tools, Store: m.deps.Store, Workspace: m.deps.Workspace}
}

// restoreChange runs the step u confirmed in Ctrl+D.
func (m AppModel) restoreChange() (AppModel, tea.Cmd) {
	step := m.Changes.confirm
	m.Changes.confirm = ""
	if step == "" {
		return m, nil
	}
	if m.Busy {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	restore := m.restoreUseCase()
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
		_, err := restore.Restore(ctx, s, step)
		return err
	})
	m.restoreOperation = m.operationID
	return m, cmd
}

// askUndo asks to confirm /undo, naming the files the last turn changed.
func (m AppModel) askUndo() (AppModel, tea.Cmd) {
	m.Composer.Input.Reset()
	if m.Busy {
		m.Status.Notice = m.Theme.T("undo.busy")
		return m, nil
	}
	state := m.Header.State
	for _, line := range state.ConsoleLines {
		if line.Restore != nil && line.Restore.Undo && line.AfterMessage == len(state.Messages) {
			// Nothing happened since the last undo.
			m.Status.Notice = m.Theme.T("undo.already")
			return m, nil
		}
	}
	var files []string
	for _, change := range state.LastTurnChanges() {
		path := singleLine(string(change.Change.Path))
		if !strings.Contains(" "+strings.Join(files, " ")+" ", " "+path+" ") {
			files = append(files, path)
		}
	}
	if len(files) == 0 {
		m.Status.Notice = m.Theme.T("undo.nothing")
		return m, nil
	}
	m.undoAsking = true
	m.Status.Error = ""
	m.Status.Notice = m.Theme.Tn("undo.confirm", len(files), changedFiles(files))
	return m, nil
}

// answerUndo takes the key that answers /undo's question: y undoes.
func (m AppModel) answerUndo(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	m.undoAsking = false
	if k.String() != "y" {
		m.Status.Notice = m.Theme.T("undo.kept")
		return m, nil
	}
	if m.Busy {
		m.Status.Notice = m.Theme.T("undo.busy")
		return m, nil
	}
	restore := m.restoreUseCase()
	m.Status.Notice = ""
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
		_, err := restore.UndoLastTurn(ctx, s)
		return err
	})
	m.restoreOperation = m.operationID
	return m, cmd
}

// restoredNotice says what a finished restore put back, from the console
// line it added.
func (m AppModel) restoredNotice(state domain.SessionState) string {
	if len(state.ConsoleLines) == 0 {
		return ""
	}
	restore := state.ConsoleLines[len(state.ConsoleLines)-1].Restore
	if restore == nil {
		return ""
	}
	return restoreLineText(*restore, m.Theme)
}

// restoreLineText reads a restore as the transcript shows it.
func restoreLineText(restore domain.FileRestore, theme Theme) string {
	paths := make([]string, 0, len(restore.Files))
	for _, f := range restore.Files {
		paths = append(paths, singleLine(string(f.Path)))
	}
	if restore.Undo {
		return theme.Tf("transcript.undone", changedFiles(paths))
	}
	return theme.Tf("transcript.restored", changedFiles(paths))
}
