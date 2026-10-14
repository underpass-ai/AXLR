package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// MaxSessionTitleRunes bounds a manual or agent-defined session title.
const MaxSessionTitleRunes = 120

// SessionLabel keeps the display title, archive flag and selected memory scope
// apart from the session transcript. Manual titles take precedence.
type SessionLabel struct {
	Title axlr.Text
	// TitleDerived marks a title the console derived from the first two
	// prompts: a title from the person or the model replaces it, while a
	// derived title never replaces theirs. On 14 October 2026 axlr_session
	// returned the derived title unchanged and glm-5.3-flash told the
	// person the console kept its own.
	TitleDerived bool
	Archived     bool
	// About is the exact KMP scope selected for continuity across sessions.
	About string
	// ImportedFrom is the workspace a session imported with --import was
	// exported from, when it differs from the one it was imported into;
	// ImportedAfter is how many messages it had then. The file changes of
	// those messages were made in ImportedFrom. On 14 October 2026 an
	// imported session listed the other workspace's changes in Ctrl+D as
	// its own and offered to restore them here.
	ImportedFrom  Workspace
	ImportedAfter int
}

func (l SessionLabel) Validate() error {
	if utf8.RuneCountInString(string(l.Title)) > MaxSessionTitleRunes {
		return errors.New("session title is too long")
	}
	if utf8.RuneCountInString(l.About) > 256 || strings.TrimSpace(l.About) != l.About || strings.IndexFunc(l.About, unicode.IsControl) >= 0 {
		return errors.New("invalid session memory about")
	}
	if l.ImportedFrom != "" {
		if _, err := NewWorkspace(string(l.ImportedFrom)); err != nil {
			return errors.New("invalid imported-from workspace")
		}
	}
	if l.ImportedAfter < 0 || l.ImportedFrom == "" && l.ImportedAfter != 0 {
		return errors.New("invalid imported-after message count")
	}
	_, err := axlr.NewText(string(l.Title))
	return err
}
