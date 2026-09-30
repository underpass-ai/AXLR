package domain

import "errors"

type IconProfile string

const (
	IconsSafe  IconProfile = "safe"
	IconsNerd  IconProfile = "nerd-mono"
	IconsASCII IconProfile = "ascii"
)

func (p IconProfile) Validate() error {
	switch p {
	case IconsSafe, IconsNerd, IconsASCII:
		return nil
	default:
		return errors.New("unknown TUI icon profile")
	}
}
