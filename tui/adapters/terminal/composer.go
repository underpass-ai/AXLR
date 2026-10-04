package terminal

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type Composer struct {
	Input textarea.Model
	Theme Theme
}

func NewComposer(mono bool, locales ...Locale) Composer {
	locale := English
	if len(locales) > 0 {
		locale = locales[0]
	}
	a := textarea.New()
	a.SetVirtualCursor(false)
	a.ShowLineNumbers = false
	a.Placeholder = Translate(locale, "composer.placeholder")
	a.SetPromptFunc(2, func(textarea.PromptInfo) string { return "  " })
	a.DynamicHeight = true
	a.MinHeight = 2
	a.MaxHeight = 8
	a.SetHeight(2)
	a.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter"), key.WithHelp("shift+enter", Translate(locale, "composer.insertNewline")))
	a.CharLimit = 0
	a.Focus()
	c := Composer{Input: a}
	c.ApplyTheme(Theme{Locale: locale, Monochrome: mono})
	return c
}

// ApplyTheme replaces the textarea defaults, including the dark cursor-line
// background, so the editor and its cursor use the active palette.
func (c *Composer) ApplyTheme(theme Theme) {
	c.Theme = theme
	styles := textarea.Styles{}
	if !theme.Monochrome {
		p := theme.palette()
		state := textarea.StyleState{
			Base:        lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.Background)),
			Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted)),
			Prompt:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent)),
			Selection:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.Selected)),
		}
		styles.Focused, styles.Blurred = state, state
		styles.Cursor = textarea.CursorStyle{Color: lipgloss.Color(p.Accent), Shape: tea.CursorBlock, Blink: true}
	}
	c.Input.SetStyles(styles)
}
func (c Composer) Update(msg tea.Msg) (Composer, tea.Cmd) {
	var cmd tea.Cmd
	c.Input, cmd = c.Input.Update(msg)
	return c, cmd
}

// View draws a rule above the input and marks its first line with the prompt
// glyph; continuation lines align under the text.
func (c Composer) View(width int) string {
	glyph := c.Theme.Icon("prompt")
	if !c.Theme.Monochrome {
		glyph = c.Theme.Accent(glyph)
	}
	c.Input.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return glyph + " "
		}
		return "  "
	})
	return c.Theme.Muted(strings.Repeat("─", max(1, width))) + "\n" + c.Input.View()
}

func (c Composer) Intent(msg tea.Msg) ControlIntent {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "enter" {
		return "send"
	}
	return ""
}
