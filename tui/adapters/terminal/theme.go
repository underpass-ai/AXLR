package terminal

import "charm.land/lipgloss/v2"

type Theme struct{ Monochrome bool }

func (t Theme) Heading(s string) string {
	if t.Monochrome {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render(s)
}
