package terminal

import "github.com/underpass-ai/AXLR/tui/domain"

type themeChoice struct {
	id          domain.ThemeID
	title       string
	description string
}

func (c themeChoice) Title() string       { return c.title }
func (c themeChoice) Description() string { return c.description }
func (c themeChoice) FilterValue() string { return c.title + " " + c.description }
