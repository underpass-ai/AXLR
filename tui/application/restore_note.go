package application

import (
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// RestoreNotePrefix opens the console note that tells the model which files
// the person restored since its last answer, from the file-changes panel or
// with /undo. A restore does not rewrite the model's history, so without
// the note the model meets files that no longer hold what it wrote: on 14
// October 2026 glm-5.3-flash found calc/calc.go "reverted by someone",
// distrusted its own memory and spent a 382k-token debug ceremony
// investigating.
const RestoreNotePrefix = "[AXLR · restore]"

// restoreNote is the note for the restores recorded since the last
// message, or "" when there were none. It is derived from the session's
// console lines, so it rides the next prompt once and nothing more is
// stored.
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
			parts = append(parts, fmt.Sprintf("%s to what %s before your last turn (with /undo)", strings.Join(paths, ", "), held(len(paths))))
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

func held(files int) string {
	if files == 1 {
		return "it held"
	}
	return "they held"
}
