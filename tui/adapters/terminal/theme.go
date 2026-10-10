package terminal

import (
	"strings"

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

// rowText is the foreground of a transcript row. Rows carry no background:
// whitespace and a leading glyph separate turns.
func (t Theme) rowText(kind transcriptRowKind) lipgloss.Style {
	if t.Monochrome {
		return lipgloss.NewStyle()
	}
	p := t.palette()
	switch kind {
	case transcriptRowUser:
		if t.editorial() {
			return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text))
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Bold(true)
	case transcriptRowSpeaker:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Bold(true)
	case transcriptRowAssistant, transcriptRowGap:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text))
	case transcriptRowDiffRemoved:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.DiffRemoved))
	case transcriptRowDiffAdded:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Good))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted))
	}
}

// rowLabel colours a row's leading glyph and restores the row's foreground
// before the trailing space, so a hanging-indent cut never splits the codes.
func (t Theme) rowLabel(label string, tone rowTone, kind transcriptRowKind) string {
	if t.Monochrome || tone == toneNone || label == "" {
		return label
	}
	p := t.palette()
	colour := map[rowTone]string{toneAccent: p.Accent, toneGood: p.Good, toneWarning: p.Warning, toneError: p.DiffRemoved}[tone]
	restore := p.Muted
	if kind == transcriptRowUser || kind == transcriptRowAssistant || kind == transcriptRowSpeaker {
		restore = p.Text
	}
	glyph := strings.TrimRight(label, " ")
	return sgrForeground(colour) + glyph + sgrForeground(restore) + label[len(glyph):]
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
		case "user", "prompt":
			return ">"
		case "brand":
			return ">>"
		case "attention":
			return "!"
		case "search":
			return "/"
		case "favorite":
			return "*"
		case "done":
			return "+"
		case "failed":
			return "x"
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
		case "user", "prompt":
			return ""
		case "done":
			return "󰄬"
		case "failed":
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
	case "user":
		return "›"
	case "prompt":
		return "❯"
	case "brand":
		return "››"
	case "attention":
		return "▲"
	case "search":
		return "⌕"
	case "favorite":
		return "★"
	case "done":
		return "✓"
	case "failed":
		return "✗"
	}
	return "•"
}
