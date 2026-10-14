package domain

import (
	"errors"
	"slices"
	"strings"
	"time"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// FileRestore is the files the person put back to an earlier snapshot of
// the session's recorded changes, from Ctrl+D or by undoing the last turn.
// It is a console line: the model's history does not change, and the
// snapshots it refers to stay in the steps' activity, so the line holds no
// file content.
type FileRestore struct {
	// Undo is true when the person undid the last turn's writes.
	Undo  bool
	Files []RestoredFile
}

// RestoredFile is one file put back: it held what step From wrote, and now
// holds what it held before step To. For one step, From and To are the
// same call; undoing a turn that changed a file twice restores it to before
// the first of the two.
type RestoredFile struct {
	Path     axlr.RelativePath
	From, To axlr.ToolCallID
}

// RestorableChange is a recorded change that can be restored: a write or
// edit that completed, was not denied, and kept both snapshots.
type RestorableChange struct {
	Call   axlr.ToolCallID
	Change FileChange
}

func (r FileRestore) validate() error {
	if len(r.Files) == 0 {
		return errors.New("a restore needs a file")
	}
	for _, f := range r.Files {
		if _, err := axlr.NewRelativePath(string(f.Path)); err != nil {
			return err
		}
		if f.From == "" || f.To == "" {
			return errors.New("a restored file needs the steps it went back over")
		}
	}
	return nil
}

func (r FileRestore) clone() FileRestore {
	r.Files = slices.Clone(r.Files)
	return r
}

// AddRestore records a restore as a console line after the session's last
// message, between turns.
func (s *Session) AddRestore(restore FileRestore) error {
	if err := s.requireBetweenTurns("console lines"); err != nil {
		return err
	}
	if err := restore.validate(); err != nil {
		return err
	}
	line := ConsoleLine{AfterMessage: len(s.state.Messages), At: now().UTC().Truncate(time.Second), Restore: &restore}
	s.state.ConsoleLines = append(s.state.ConsoleLines, line.clone())
	return nil
}

// RecordedChange is the change a step recorded, when it can be restored:
// the call completed without error or uncertainty, was not denied, and its
// change kept both snapshots. The reason says why not otherwise.
func (s SessionState) RecordedChange(call axlr.ToolCallID) (FileChange, string) {
	for _, a := range s.Activity {
		if a.Call.ID != call {
			continue
		}
		switch {
		case a.Outcome == nil || a.Outcome.Change == nil:
			return FileChange{}, "the step changed no file"
		case a.Outcome.IsError || a.Outcome.Uncertain || a.Decision == DecisionDeny:
			return FileChange{}, "the step did not complete"
		case a.Outcome.Change.Unavailable != "":
			return FileChange{}, "the step's snapshot was not kept (" + a.Outcome.Change.Unavailable + ")"
		}
		return *a.Outcome.Change, ""
	}
	return FileChange{}, "the step is not in this session"
}

// RestoreChange is the file change a restored file made: from what step From
// wrote back to what the file held before step To.
func (s SessionState) RestoreChange(f RestoredFile) (FileChange, bool) {
	from, fromReason := s.RecordedChange(f.From)
	to, toReason := s.RecordedChange(f.To)
	if fromReason != "" || toReason != "" {
		return FileChange{Path: f.Path, Unavailable: "unavailable"}, false
	}
	return FileChange{Path: f.Path, Before: from.After, After: to.Before}, true
}

// LastTurnChanges is the restorable changes of the last turn, newest first:
// the calls after the person's last prompt. A console note or a budget note
// is not a prompt; a message the person steered into a running turn is.
func (s SessionState) LastTurnChanges() []RestorableChange {
	return s.TurnChanges(len(s.promptIndexes()))
}

// TurnChanges is the restorable changes of the turn the person's prompt
// number turn (from 1) began, newest first; none for a turn that is not
// there. A change the person already restored from the file-changes panel
// is left out: its file is back to what it held before that step, so an
// undo of the turn undoes what remains. On 14 October 2026 /undo offered a
// turn whose calc_test.go the person had restored from Ctrl+D, found the
// file "changed after the session wrote it" and refused the whole undo, so
// calc.go was not undone either.
func (s SessionState) TurnChanges(turn int) []RestorableChange {
	prompts := s.promptIndexes()
	if turn < 1 || turn > len(prompts) {
		return nil
	}
	end := len(s.Messages)
	if turn < len(prompts) {
		end = prompts[turn]
	}
	var calls []axlr.ToolCallID
	for _, message := range s.Messages[prompts[turn-1]+1 : end] {
		for _, call := range message.ToolCalls {
			calls = append(calls, call.ID)
		}
	}
	restored := s.panelRestored()
	var changes []RestorableChange
	for i := len(calls) - 1; i >= 0; i-- {
		if restored[calls[i]] {
			continue
		}
		for _, a := range s.Activity {
			if a.Call.ID == calls[i] && a.Outcome != nil && a.Outcome.Change != nil && !a.Outcome.IsError && !a.Outcome.Uncertain && a.Decision != DecisionDeny {
				changes = append(changes, RestorableChange{Call: calls[i], Change: *a.Outcome.Change})
				break
			}
		}
	}
	return changes
}

// panelRestored is the steps whose change the person restored one at a
// time, from the file-changes panel.
func (s SessionState) panelRestored() map[axlr.ToolCallID]bool {
	restored := map[axlr.ToolCallID]bool{}
	for _, line := range s.ConsoleLines {
		if line.Restore != nil && !line.Restore.Undo {
			for _, f := range line.Restore.Files {
				restored[f.From] = true
			}
		}
	}
	return restored
}

// LatestChangingTurn is the most recent turn whose writes and edits can be
// restored and were not undone yet, numbered from 1, or 0: /undo after a
// turn that changed nothing offers it. A turn whose every change the
// person restored from the panel has nothing left to undo.
func (s SessionState) LatestChangingTurn() int {
	undone := map[axlr.ToolCallID]bool{}
	for _, line := range s.ConsoleLines {
		if line.Restore != nil && line.Restore.Undo {
			for _, f := range line.Restore.Files {
				undone[f.From], undone[f.To] = true, true
			}
		}
	}
	for turn := len(s.promptIndexes()); turn > 0; turn-- {
		changes := s.TurnChanges(turn)
		if len(changes) > 0 && !slices.ContainsFunc(changes, func(c RestorableChange) bool { return undone[c.Call] }) {
			return turn
		}
	}
	return 0
}

// promptIndexes are the indexes of the person's prompts in the messages.
func (s SessionState) promptIndexes() []int {
	var prompts []int
	for i, message := range s.Messages {
		if message.Role == axlr.RoleUser && !strings.HasPrefix(string(message.Content), "[AXLR") && !BudgetNote(message.Content) {
			prompts = append(prompts, i)
		}
	}
	return prompts
}

// TurnPrompt is the message that began the person's turn number turn
// (from 1), console notes appended to it included; false for a turn that
// is not there.
func (s SessionState) TurnPrompt(turn int) (axlr.Text, bool) {
	prompts := s.promptIndexes()
	if turn < 1 || turn > len(prompts) {
		return "", false
	}
	return s.Messages[prompts[turn-1]].Content, true
}

// TurnOf is the turn (from 1) whose calls include call, or 0.
func (s SessionState) TurnOf(call axlr.ToolCallID) int {
	prompts := s.promptIndexes()
	for i, message := range s.Messages {
		for _, c := range message.ToolCalls {
			if c.ID == call {
				turn := 0
				for turn < len(prompts) && prompts[turn] < i {
					turn++
				}
				return turn
			}
		}
	}
	return 0
}

// TurnsBefore is how many turns began before message index end.
func (s SessionState) TurnsBefore(end int) int {
	count := 0
	for _, i := range s.promptIndexes() {
		if i < end {
			count++
		}
	}
	return count
}
