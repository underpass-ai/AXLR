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
	start := -1
	for i := len(s.Messages) - 1; i >= 0; i-- {
		message := s.Messages[i]
		if message.Role == axlr.RoleUser && !strings.HasPrefix(string(message.Content), "[AXLR") && !BudgetNote(message.Content) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	var calls []axlr.ToolCallID
	for _, message := range s.Messages[start+1:] {
		for _, call := range message.ToolCalls {
			calls = append(calls, call.ID)
		}
	}
	var changes []RestorableChange
	for i := len(calls) - 1; i >= 0; i-- {
		for _, a := range s.Activity {
			if a.Call.ID == calls[i] && a.Outcome != nil && a.Outcome.Change != nil && !a.Outcome.IsError && !a.Outcome.Uncertain && a.Decision != DecisionDeny {
				changes = append(changes, RestorableChange{Call: calls[i], Change: *a.Outcome.Change})
				break
			}
		}
	}
	return changes
}
