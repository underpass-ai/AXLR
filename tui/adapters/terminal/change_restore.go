package terminal

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
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
	if record.Foreign {
		return changeRecord{}, "changes.restoreForeign"
	}
	return record, ""
}

// askRestore asks to confirm u on the selection, or says why it cannot.
func (v *ChangeViewer) askRestore() {
	record, refused := v.restoreStep()
	if refused == "changes.restoreForeign" {
		v.notice = v.Theme.Tf(refused, shortenHome(singleLine(string(v.importedFrom))))
		return
	}
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

// askUndo asks to confirm /undo, naming the files the last turn changed;
// after a turn that changed none, the most recent turn that did. On 14
// October 2026 /undo after a turn that only read said "The last turn
// changed no file" and nothing more, though the turn before had.
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
	m.undoTurn = 0
	changes := state.LastTurnChanges()
	if len(changes) == 0 {
		if turn := state.LatestChangingTurn(); turn > 0 {
			m.undoTurn, changes = turn, state.TurnChanges(turn)
		}
	}
	if m.Changes.foreignCalls(changes) {
		m.Status.Notice = m.Theme.Tf("changes.restoreForeign", shortenHome(singleLine(string(m.Changes.importedFrom))))
		return m, nil
	}
	var files []string
	for _, change := range changes {
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
	if m.undoTurn > 0 {
		m.Status.Notice = m.Theme.Tn("undo.confirmTurn", len(files), m.undoTurn, changedFiles(files))
	}
	turn := m.undoTurn
	if turn == 0 {
		turn = state.TurnsBefore(len(state.Messages))
	}
	m.undoRefs = turnMemoryRefs(state, turn)
	if len(m.undoRefs) > 0 && m.deps.MemoryForget != nil {
		m.Status.Notice += m.Theme.counted("undo.forgetToo", len(m.undoRefs))
	}
	return m, nil
}

// undoMemories is what an undo did with the memories its turn recorded:
// closed them (f), or left them current (y), named so /memory forget can
// close them later.
type undoMemories struct {
	Closed int
	Kept   []string
	Err    error
}

// turnMemoryRefs are the refs of the memories axlr_remember recorded in a
// turn. On 20 October 2026 /undo put calc.py back while the turn's
// "Renamed div to divide" stayed current in KMP, and the next session
// recalled a rename that had been undone.
func turnMemoryRefs(state domain.SessionState, turn int) []string {
	if turn <= 0 {
		return nil
	}
	calls := map[root.ToolCallID]bool{}
	var refs []string
	for _, message := range state.Messages {
		for _, call := range message.ToolCalls {
			if call.Name == application.HostRememberName && state.TurnOf(call.ID) == turn {
				calls[call.ID] = true
			}
		}
		if message.Role != root.RoleTool || !calls[message.ToolCallID] {
			continue
		}
		var result struct {
			Accepted bool   `json:"accepted"`
			Ref      string `json:"ref"`
		}
		if json.Unmarshal([]byte(message.Content), &result) == nil && result.Accepted && result.Ref != "" && !slices.Contains(refs, result.Ref) {
			refs = append(refs, result.Ref)
		}
	}
	return refs
}

// undoMemoriesNotice adds to the undo's notice what became of the turn's
// memories.
func (m AppModel) undoMemoriesNotice(result undoMemories) string {
	switch {
	case result.Err != nil:
		return m.Theme.Tf("undo.forgetFailed", result.Err.Error())
	case result.Closed > 0:
		return m.Theme.counted("undo.forgot", result.Closed)
	case len(result.Kept) == 1:
		return m.Theme.Tf("undo.memoryKeptOne", result.Kept[0])
	case len(result.Kept) > 1:
		return m.Theme.Tf("undo.memoryKept", len(result.Kept), strings.Join(result.Kept, " "))
	}
	return ""
}

// counted is label for n, with its One form (which names no number) for 1.
func (t Theme) counted(label string, n int) string {
	if n == 1 {
		return t.T(label + "One")
	}
	return t.Tf(label, n)
}

// answerUndo takes the key that answers /undo's question: y undoes.
func (m AppModel) answerUndo(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	m.undoAsking = false
	refs := m.undoRefs
	m.undoRefs = nil
	forget := k.String() == "f" && len(refs) > 0 && m.deps.MemoryForget != nil
	if k.String() != "y" && !forget {
		m.Status.Notice = m.Theme.T("undo.kept")
		return m, nil
	}
	if m.Busy {
		m.Status.Notice = m.Theme.T("undo.busy")
		return m, nil
	}
	restore := m.restoreUseCase()
	m.Status.Notice = ""
	turn := m.undoTurn
	memories := undoMemories{Kept: refs}
	var forgetter application.MemoryForgetPort
	var requests []application.ForgetRequest
	if forget {
		forgetter, memories.Kept = m.deps.MemoryForget, nil
		state := m.Header.State
		for _, ref := range refs {
			requests = append(requests, application.ForgetRequest{Ref: ref, Why: "undone with /undo: the turn that recorded it was put back", Command: "/undo", At: time.Now(), ContextID: m.sessionContext(),
				Labels: map[string][]string{"ws": {string(state.Workspace)}, "session": {string(state.ID)}, "event": {"forget"}}})
		}
	}
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
		var err error
		if turn > 0 {
			_, err = restore.UndoTurn(ctx, s, turn)
		} else {
			_, err = restore.UndoLastTurn(ctx, s)
		}
		if err != nil {
			return err
		}
		// The files are back; a memory that cannot be closed is said, not
		// taken for a failed undo.
		for _, request := range requests {
			if _, err := forgetter.Forget(ctx, request); err != nil {
				memories.Err = err
				break
			}
			memories.Closed++
		}
		return nil
	})
	m.restoreOperation = m.operationID
	if len(refs) == 0 {
		return m, cmd
	}
	return m, func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok && done.Err == nil {
			done.UndoMemories = &memories
			return done
		}
		return msg
	}
}

// restoredNotice says what a finished restore put back, from the console
// line it added.
func (m AppModel) restoredNotice(state domain.SessionState) string {
	if len(state.ConsoleLines) == 0 {
		return ""
	}
	line := state.ConsoleLines[len(state.ConsoleLines)-1]
	if line.Restore == nil {
		return ""
	}
	return restoreLineTextIn(state, line, m.Theme)
}

// restoreLineTextIn reads a restore as the transcript shows it, naming the
// turn an undo put back when it was not the last one before it.
func restoreLineTextIn(state domain.SessionState, line domain.ConsoleLine, theme Theme) string {
	restore := *line.Restore
	if restore.Undo && len(restore.Files) > 0 {
		if turn := state.TurnOf(restore.Files[0].From); turn > 0 && turn != state.TurnsBefore(line.AfterMessage) {
			paths := make([]string, 0, len(restore.Files))
			for _, f := range restore.Files {
				paths = append(paths, singleLine(string(f.Path)))
			}
			return theme.Tf("transcript.undoneTurn", turn, changedFiles(paths))
		}
	}
	return restoreLineText(restore, theme)
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
