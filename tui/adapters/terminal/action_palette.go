package terminal

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
)

type actionItem struct {
	intent      ControlIntent
	title       string
	description string
	shortcut    string
}

func (a actionItem) FilterValue() string { return a.title + " " + a.description + " " + a.shortcut }

var actionItems = []list.Item{
	actionItem{"mcp", "palette.mcpTitle", "palette.mcpDescription", "e"},
	actionItem{"plugins", "palette.pluginsTitle", "palette.pluginsDescription", "p"},
	actionItem{"models", "palette.modelsTitle", "palette.modelsDescription", "m"},
	actionItem{"theme", "palette.themesTitle", "palette.themesDescription", "t"},
	actionItem{"search", "palette.searchTitle", "palette.searchDescription", "s"},
	actionItem{"sessions", "palette.sessionsTitle", "palette.sessionsDescription", "o"},
	actionItem{"help", "palette.helpTitle", "palette.helpDescription", "h"},
	actionItem{"info", "palette.infoTitle", "palette.infoDescription", "i"},
	actionItem{"continue", "palette.continueTitle", "palette.continueDescription", "r"},
	actionItem{"cancel", "palette.cancelTitle", "palette.cancelDescription", "c"},
	actionItem{"updates", "palette.updatesTitle", "palette.updatesDescription", "u"},
	actionItem{"changes", "palette.changesTitle", "palette.changesDescription", "d"},
	actionItem{"jobs", "palette.jobsTitle", "palette.jobsDescription", "j"},
	actionItem{"copy", "palette.copyTitle", "palette.copyDescription", "y"},
}

type actionDelegate struct {
	theme  Theme
	zones  *zone.Manager
	prefix string
}

func (actionDelegate) Height() int                         { return 1 }
func (actionDelegate) Spacing() int                        { return 0 }
func (actionDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d actionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	a, ok := item.(actionItem)
	if !ok {
		return
	}
	width := max(1, m.Width())
	label := ansi.Truncate("  "+a.title, max(1, width-5), "…")
	shortcut := strings.ToUpper(a.shortcut)
	gap := max(1, width-ansi.StringWidth(label)-ansi.StringWidth(shortcut)-2)
	line := label + strings.Repeat(" ", gap) + shortcut
	if index == m.Index() && m.FilterState() != list.Filtering {
		line = d.theme.Selected(line)
	} else {
		line = d.theme.Panel(line, width)
	}
	if d.zones != nil {
		line = d.zones.Mark(d.prefix+string(a.intent), line)
	}
	fmt.Fprint(w, line)
}

type ActionPalette struct{ List list.Model }

func NewActionPalette(locales ...Locale) ActionPalette {
	locale := English
	if len(locales) > 0 {
		locale = locales[0]
	}
	items := make([]list.Item, len(actionItems))
	for i, item := range actionItems {
		a := item.(actionItem)
		a.title = Translate(locale, a.title)
		a.description = Translate(locale, a.description)
		items[i] = a
	}
	l := list.New(items, actionDelegate{}, 40, 12)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.FilterInput.Prompt = Translate(locale, "common.searchPrompt")
	l.FilterInput.SetVirtualCursor(false)
	return ActionPalette{List: l}
}

func (p ActionPalette) Update(msg tea.Msg) (ActionPalette, ControlIntent, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if !p.List.SettingFilter() {
			switch key.String() {
			case "esc":
				return p, "close", nil
			case "enter":
				if selected, ok := p.List.SelectedItem().(actionItem); ok {
					return p, selected.intent, nil
				}
				return p, "", nil
			default:
				// Shortcuts are drawn uppercase: Shift+letter is the same key.
				for _, item := range actionItems {
					a := item.(actionItem)
					if strings.ToLower(key.String()) == a.shortcut {
						return p, a.intent, nil
					}
				}
			}
		}
	}
	var cmd tea.Cmd
	p.List, cmd = p.List.Update(msg)
	return p, "", cmd
}

func (p *ActionPalette) View(theme Theme, zones *zone.Manager, prefix string, width, height int) string {
	innerWidth, bodyHeight := OverlayBodySize(width, height)
	p.List.SetDelegate(actionDelegate{theme: theme, zones: zones, prefix: prefix})
	p.List.SetSize(innerWidth, max(1, bodyHeight-2))
	styles := p.List.FilterInput.Styles()
	if !theme.Monochrome {
		styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.palette().Accent))
		styles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.palette().Text))
		styles.Cursor.Color = lipgloss.Color(theme.palette().Accent)
	}
	p.List.FilterInput.SetStyles(styles)
	hint := theme.T("palette.hint")
	footer := zones.Mark(prefix+"close", "["+theme.T("common.close")+"]")
	detail := ""
	if selected, ok := p.List.SelectedItem().(actionItem); ok {
		detail = theme.Muted(ansi.Truncate(selected.description, innerWidth, "…"))
	}
	listView := p.List.View()
	if p.List.FilterState() == list.FilterApplied && len(p.List.VisibleItems()) == 0 {
		listView = strings.Replace(listView, "No items.", theme.T("palette.noMatches"), 1)
	}
	body := listView + "\n" + detail
	return theme.Overlay(theme.T("palette.title"), hint, body, footer, width, height)
}
