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

func (t Theme) UserRow() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Monochrome {
		return style
	}
	background := lipgloss.Color("#1D3033")
	if t.Light {
		background = lipgloss.Color("#EAF2F3")
	}
	return style.Background(background)
}

func (t Theme) Heading(s string) string {
	if t.Monochrome {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render(s)
}

func (t Theme) MemoryRow() lipgloss.Style {
	style := lipgloss.NewStyle()
	if t.Monochrome {
		return style
	}
	background := lipgloss.Color("#30283D")
	if t.Light {
		background = lipgloss.Color("#F1ECF8")
	}
	return style.Background(background)
}
