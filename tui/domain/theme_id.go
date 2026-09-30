package domain

import "errors"

type ThemeID string

const (
	ThemeAuto     ThemeID = "auto"
	ThemeInk      ThemeID = "ink"
	ThemeAurora   ThemeID = "aurora"
	ThemePaper    ThemeID = "paper"
	ThemePhosphor ThemeID = "phosphor"
)

func (id ThemeID) Validate() error {
	switch id {
	case ThemeAuto, ThemeInk, ThemeAurora, ThemePaper, ThemePhosphor:
		return nil
	default:
		return errors.New("unknown TUI theme")
	}
}
