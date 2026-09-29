package domain

import (
	"errors"
	"strings"
	"unicode"

	root "github.com/underpass-ai/AXLR/domain"
)

type AvailableModel struct {
	ID             root.ModelID
	Name           root.Text
	Context        ContextWindow
	PromptRate     ModelRate
	CompletionRate ModelRate
	SupportsTools  bool
	TextOutput     bool
}

func (m AvailableModel) Validate() error {
	if _, err := root.NewModelID(string(m.ID)); err != nil {
		return err
	}
	if _, err := root.NewText(string(m.Name)); err != nil {
		return err
	}
	if strings.TrimSpace(string(m.Name)) == "" {
		return errors.New("model name is empty")
	}
	for _, r := range string(m.Name) {
		if unicode.IsControl(r) {
			return errors.New("model name contains control characters")
		}
	}
	if m.Context < 0 {
		return errors.New("context window cannot be negative")
	}
	return nil
}
