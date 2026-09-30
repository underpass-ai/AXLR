package terminal

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

type Composer struct{ Input textarea.Model }

func NewComposer(mono bool) Composer {
	a := textarea.New()
	a.SetVirtualCursor(false)
	a.ShowLineNumbers = false
	a.Placeholder = "Write a message"
	a.Prompt = "> "
	a.SetHeight(3)
	a.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter"), key.WithHelp("shift+enter", "insert newline"))
	a.CharLimit = 0
	a.MaxHeight = 0
	if mono {
		a.SetStyles(textarea.Styles{})
	}
	a.Focus()
	return Composer{Input: a}
}
func (c Composer) Update(msg tea.Msg) (Composer, tea.Cmd) {
	var cmd tea.Cmd
	c.Input, cmd = c.Input.Update(msg)
	return c, cmd
}
func (c Composer) View() string { return c.Input.View() }

func (c Composer) Intent(msg tea.Msg) ControlIntent {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "enter" {
		return "send"
	}
	return ""
}
func (c Composer) Controls(z *zone.Manager, prefix string) string {
	return z.Mark(prefix+"send", "[Send Enter]") + " Shift+Enter newline " + z.Mark(prefix+"cancel", "[Cancel Esc]")
}
