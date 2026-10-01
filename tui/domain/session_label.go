package domain

import (
	"errors"
	"unicode/utf8"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// MaxSessionTitleRunes bounds a user-given session title.
const MaxSessionTitleRunes = 120

// SessionLabel is what the user says about a session, kept apart from its
// content: a title replacing the first prompt, and whether it is archived.
type SessionLabel struct {
	Title    axlr.Text
	Archived bool
}

func (l SessionLabel) Validate() error {
	if utf8.RuneCountInString(string(l.Title)) > MaxSessionTitleRunes {
		return errors.New("session title is too long")
	}
	_, err := axlr.NewText(string(l.Title))
	return err
}
