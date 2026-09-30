package terminal

import (
	"charm.land/lipgloss/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Theme struct {
	ID         domain.ThemeID
	Icons      domain.IconProfile
	Locale     Locale
	Monochrome bool
	Light      bool
}

func (t Theme) palette() ThemePalette {
	if t.Light && (t.ID == "" || t.ID == domain.ThemeAuto) {
		return themePalettes[domain.ThemePaper]
	}
	if p, ok := themePalettes[t.ID]; ok {
		return p
	}
	return themePalettes[domain.ThemeInk]
}

func (t Theme) AssistantRow() lipgloss.Style {
	if t.Monochrome {
		return lipgloss.NewStyle()
	}
	p := t.palette()
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.Surface))
}

func (t Theme) UserRow() lipgloss.Style {
	if t.Monochrome {
		return lipgloss.NewStyle()
	}
	p := t.palette()
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.User))
}

func (t Theme) MemoryRow() lipgloss.Style {
	if t.Monochrome {
		return lipgloss.NewStyle()
	}
	p := t.palette()
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.Memory))
}

func (t Theme) ToolRow() lipgloss.Style {
	if t.Monochrome {
		return lipgloss.NewStyle()
	}
	p := t.palette()
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted)).Background(lipgloss.Color(p.Raised))
}

func (t Theme) Heading(s string) string {
	if t.Monochrome {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.palette().Accent)).Render(s)
}

func (t Theme) Muted(s string) string {
	if t.Monochrome {
		return s
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(t.palette().Muted)).Render(s)
}

func (t Theme) Accent(s string) string {
	if t.Monochrome {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.palette().Accent)).Render(s)
}

func (t Theme) Selected(s string) string {
	if t.Monochrome {
		return "> " + s
	}
	p := t.palette()
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.Selected)).Bold(true).Render(s)
}

func (t Theme) Panel(s string, width int) string {
	if t.Monochrome {
		return s
	}
	p := t.palette()
	return lipgloss.NewStyle().Width(max(1, width)).Background(lipgloss.Color(p.Surface)).Foreground(lipgloss.Color(p.Text)).Render(s)
}

func (t Theme) Icon(kind string) string {
	if t.Icons == domain.IconsASCII || t.Monochrome {
		switch kind {
		case "memory":
			return "[MEM]"
		case "connected":
			return "[OK]"
		case "waiting":
			return "[WAIT]"
		case "error":
			return "[ERR]"
		}
	}
	if t.Icons == domain.IconsNerd {
		switch kind {
		case "memory":
			return "󰈙"
		case "connected":
			return "󰄬"
		case "waiting":
			return "󰏤"
		case "error":
			return "󰅙"
		}
	}
	switch kind {
	case "memory":
		return "◇"
	case "connected":
		return "●"
	case "waiting":
		return "◌"
	case "error":
		return "!"
	}
	return "•"
}
