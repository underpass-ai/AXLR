package application

import (
	"context"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// RestoreNotePrefix opens the console note that tells the model which files
// the person restored since its last answer, from the file-changes panel or
// with /undo. A restore does not rewrite the model's history, so without
// the note the model meets files that no longer hold what it wrote: on 14
// October 2026 glm-5.3-flash found calc/calc.go "reverted by someone",
// distrusted its own memory and spent a 382k-token debug ceremony
// investigating.
const RestoreNotePrefix = domain.RestoreNotePrefix

// restoreNote is the note for the restores recorded since the last
// message, or "" when there were none. It is derived from the session's
// console lines, so it rides the next prompt once (or joins a resumed turn,
// deliverRestoreNote) and nothing more is stored.
func restoreNote(state domain.SessionState) string {
	var parts []string
	for _, line := range state.ConsoleLines {
		if line.Restore == nil || line.AfterMessage != len(state.Messages) || len(line.Restore.Files) == 0 {
			continue
		}
		paths := make([]string, 0, len(line.Restore.Files))
		for _, file := range line.Restore.Files {
			paths = append(paths, string(file.Path))
		}
		if line.Restore.Undo {
			parts = append(parts, fmt.Sprintf("%s to what %s before %s (with /undo)", strings.Join(paths, ", "), held(len(paths)), undoneTurn(state, line.Restore.Files[0].To)))
			continue
		}
		file := line.Restore.Files[0]
		parts = append(parts, fmt.Sprintf("%s to what it held before your call %s (from the file-changes panel)", file.Path, file.To))
	}
	if len(parts) == 0 {
		return ""
	}
	return RestoreNotePrefix + " Since your last answer the person restored " + strings.Join(parts, "; and ") + ". That was the person's decision, not a fault: the files on disk now hold the restored content, so local_read a file before relying on what you wrote to it or changing it again."
}

// undoneTurn names the turn an undo put back, by the call it went back
// before. /undo reaches the most recent turn that changed files, which is
// not always the last one: the note said "your last turn" whatever turn it
// was, and the model looked for the restored change in the wrong place.
func undoneTurn(state domain.SessionState, call root.ToolCallID) string {
	turn, last := state.TurnOf(call), state.TurnsBefore(len(state.Messages))
	if turn == 0 || turn == last {
		return "your last turn"
	}
	prompt, _ := state.TurnPrompt(turn)
	return fmt.Sprintf("your turn %d of %d, not your last, the one the person began with %q", turn, last, excerpt(personTyped(string(prompt)), restoreNotePromptBytes))
}

// restoreNotePromptBytes bounds the prompt that names an undone turn.
const restoreNotePromptBytes = 120

func held(files int) string {
	if files == 1 {
		return "it held"
	}
	return "they held"
}

// deliverRestoreNote adds the note for restores made while the turn was
// interrupted to the resumed turn, before its next request: Ctrl+R resumes
// without a prompt, and the note rode only a new prompt, so the model
// continued over files that no longer held what it wrote. It does nothing
// when there is no such restore or the turn cannot take a note.
func deliverRestoreNote(ctx context.Context, session *domain.Session, store SessionStorePort) (bool, error) {
	note := restoreNote(session.Export())
	if note == "" || session.Status() != domain.StatusStreaming || len(session.Pending()) != 0 {
		return false, nil
	}
	next := *session
	if err := next.Note(root.Text(note)); err != nil {
		return false, err
	}
	if err := store.Save(ctx, next); err != nil {
		return false, err
	}
	*session = next
	return true, nil
}
