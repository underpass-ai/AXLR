package terminal

import (
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ThemePicker struct {
	List     list.Model
	Original domain.UIPreferences
	Preview  domain.UIPreferences
}

func NewThemePicker(p domain.UIPreferences, locales ...Locale) ThemePicker {
	locale := English
	if len(locales) > 0 {
		locale = locales[0]
	}
	items := []list.Item{
		themeChoice{domain.ThemeAuto, Translate(locale, "theme.name.auto"), Translate(locale, "theme.autoDescription")},
		themeChoice{domain.ThemeInk, Translate(locale, "theme.name.ink"), Translate(locale, "theme.inkDescription")},
		themeChoice{domain.ThemeAurora, Translate(locale, "theme.name.aurora"), Translate(locale, "theme.auroraDescription")},
		themeChoice{domain.ThemePaper, Translate(locale, "theme.name.paper"), Translate(locale, "theme.paperDescription")},
		themeChoice{domain.ThemePhosphor, Translate(locale, "theme.name.phosphor"), Translate(locale, "theme.phosphorDescription")},
	}
	l := list.New(items, list.NewDefaultDelegate(), 36, 16)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.DisableQuitKeybindings()
	l.FilterInput.Prompt = Translate(locale, "common.searchPrompt")
	l.FilterInput.SetVirtualCursor(false)
	for i, item := range items {
		if item.(themeChoice).id == p.Theme {
			l.Select(i)
			break
		}
	}
	return ThemePicker{List: l, Original: p, Preview: p}
}

func (p ThemePicker) Update(msg tea.Msg) (ThemePicker, ControlIntent, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			if !p.List.SettingFilter() {
				return p, "theme-cancel", nil
			}
		case "enter":
			if !p.List.SettingFilter() {
				return p, "theme-save", nil
			}
		case "i":
			if !p.List.SettingFilter() {
				switch p.Preview.Icons {
				case domain.IconsSafe:
					p.Preview.Icons = domain.IconsNerd
				case domain.IconsNerd:
					p.Preview.Icons = domain.IconsASCII
				default:
					p.Preview.Icons = domain.IconsSafe
				}
				return p, "theme-preview", nil
			}
		case "a":
			if !p.List.SettingFilter() {
				p.Preview.ReduceMotion = !p.Preview.ReduceMotion
				return p, "theme-preview", nil
			}
		}
	}
	var cmd tea.Cmd
	p.List, cmd = p.List.Update(msg)
	if choice, ok := p.List.SelectedItem().(themeChoice); ok && choice.id != p.Preview.Theme {
		p.Preview.Theme = choice.id
		return p, "theme-preview", cmd
	}
	return p, "", cmd
}

func (p *ThemePicker) View(theme Theme, width, height int) string {
	width, height = max(1, width), max(1, height)
	leftWidth := width
	if width >= 80 {
		leftWidth = max(26, width*2/5)
	}
	p.List.SetSize(max(1, leftWidth-2), max(1, height-7))
	left := p.List.View()
	if p.List.FilterState() == list.FilterApplied && len(p.List.VisibleItems()) == 0 {
		left = strings.Replace(left, "No items.", theme.T("theme.noMatches"), 1)
	}
	if width >= 80 {
		left = lipgloss.NewStyle().Width(leftWidth - 2).Height(max(1, height-7)).Render(left)
	}
	icon := string(p.Preview.Icons)
	if icon == "" {
		icon = string(domain.IconsSafe)
	}
	motion := theme.T("theme.motionOn")
	if p.Preview.ReduceMotion {
		motion = theme.T("theme.motionReduced")
	}
	header := theme.Heading(theme.T("palette.themesTitle")) + "  " + theme.Muted(theme.T("theme.hint"))
	preview := []string{
		theme.Heading(theme.Tf("theme.preview", theme.T("theme.name."+string(p.Preview.Theme)))),
		"",
		theme.UserRow().Render(" " + theme.T("theme.sampleUser") + " "),
		"",
		theme.AssistantRow().Render(" " + theme.T("theme.sampleAssistant") + " "),
		"",
		theme.MemoryRow().Render(" " + theme.Icon("memory") + " " + theme.T("theme.sampleMemory") + " "),
		"",
		theme.Muted(theme.Tf("theme.iconsMotion", icon, motion)),
	}
	var body string
	if width >= 80 {
		rightWidth := max(1, width-leftWidth-2)
		for i, line := range preview {
			preview[i] = ansi.Truncate(line, rightWidth, "…")
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", theme.Panel(strings.Join(preview, "\n"), rightWidth))
	} else {
		body = left + "\n" + strings.Join(preview, "\n")
	}
	footer := theme.T("theme.footer")
	lines := strings.Split(header+"\n"+body+"\n"+theme.Muted(footer), "\n")
	if len(lines) > height {
		lines = append(lines[:height-1], lines[len(lines)-1])
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}
