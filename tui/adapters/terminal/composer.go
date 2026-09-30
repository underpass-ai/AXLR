package terminal

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
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
	a.Prompt = "  "
	a.DynamicHeight = true
	a.MinHeight = 2
	a.MaxHeight = 8
	a.SetHeight(2)
	a.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter"), key.WithHelp("shift+enter", Translate(locale, "composer.insertNewline")))
	a.CharLimit = 0
	if mono {
		a.SetStyles(textarea.Styles{})
	}
	a.Focus()
	return Composer{Input: a, Theme: Theme{Locale: locale, Monochrome: mono}}
}
func (c Composer) Update(msg tea.Msg) (Composer, tea.Cmd) {
	var cmd tea.Cmd
	c.Input, cmd = c.Input.Update(msg)
	return c, cmd
}
func (c Composer) View() string {
	header := c.Theme.T("composer.title")
	if c.Input.Value() != "" {
		header += " · " + c.Theme.T("composer.draft")
	}
	return c.Theme.Panel(c.Theme.Accent("  "+header)+"\n"+c.Input.View(), c.Input.Width())
}

func (c Composer) Intent(msg tea.Msg) ControlIntent {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "enter" {
		return "send"
	}
	return ""
}
func (c Composer) Controls(z *zone.Manager, prefix string) string {
	return z.Mark(prefix+"send", c.Theme.Accent("[ "+c.Theme.T("composer.send")+" ]")) + c.Theme.Muted("  "+c.Theme.T("composer.newline")+"  ") + z.Mark(prefix+"cancel", "[ "+c.Theme.T("common.cancel")+" ]")
}
