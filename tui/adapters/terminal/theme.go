package terminal

import "charm.land/lipgloss/v2"

type Theme struct {
	Monochrome bool
	Light      bool
}

func (t Theme) AssistantRow() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Monochrome {
		return style
	}
	background := lipgloss.Color("#262B32")
	if t.Light {
		background = lipgloss.Color("#F3F5F7")
	}
	return style.Background(background)
}

func (t Theme) Heading(s string) string {
	if t.Monochrome {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render(s)
}
