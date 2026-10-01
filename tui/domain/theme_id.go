package domain

import "errors"

type ThemeID string

const (
	ThemeAuto     ThemeID = "auto"
	ThemeInk      ThemeID = "ink"
	ThemeAurora   ThemeID = "aurora"
	ThemePaper    ThemeID = "paper"
	ThemePhosphor ThemeID = "phosphor"
	// ThemeEditorial is a light theme that also changes the transcript's
	// layout: speaker labels, tool calls summarised per run, a bottom sheet.
	ThemeEditorial ThemeID = "editorial"
)

func (id ThemeID) Validate() error {
	switch id {
	case ThemeAuto, ThemeInk, ThemeAurora, ThemePaper, ThemePhosphor, ThemeEditorial:
		return nil
	default:
		return errors.New("unknown TUI theme")
	}
}
