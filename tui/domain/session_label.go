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
	Title    axlr.Text
	Archived bool
	// About is the exact KMP scope selected for continuity across sessions.
	About string
}

func (l SessionLabel) Validate() error {
	if utf8.RuneCountInString(string(l.Title)) > MaxSessionTitleRunes {
		return errors.New("session title is too long")
	}
	if utf8.RuneCountInString(l.About) > 256 || strings.TrimSpace(l.About) != l.About || strings.IndexFunc(l.About, unicode.IsControl) >= 0 {
		return errors.New("invalid session memory about")
	}
	_, err := axlr.NewText(string(l.Title))
	return err
}
